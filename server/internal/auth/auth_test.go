package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type mockGitHub struct {
	start DeviceStart
	token string
	user  GitHubUser
	err   error
	pollN int
}

func (m *mockGitHub) StartDevice(context.Context, string) (DeviceStart, error) {
	if m.err != nil {
		return DeviceStart{}, m.err
	}
	return m.start, nil
}

func (m *mockGitHub) PollToken(context.Context, string, string, string) (string, error) {
	m.pollN++
	if m.pollN < 2 {
		return "", ErrDevicePending
	}
	if m.err != nil {
		return "", m.err
	}
	return m.token, nil
}

func (m *mockGitHub) FetchUser(context.Context, string) (GitHubUser, error) {
	return m.user, m.err
}

func TestBearerTokenParse(t *testing.T) {
	if bearerToken("Bearer abc") != "abc" {
		t.Fatal(bearerToken("Bearer abc"))
	}
	if bearerToken("bearer abc") != "abc" {
		t.Fatal("case")
	}
	if bearerToken("Token abc") != "" {
		t.Fatal("reject")
	}
}

func TestHashTokenStable(t *testing.T) {
	a := hashToken("secret")
	b := hashToken("secret")
	if a != b || len(a) != 64 {
		t.Fatalf("%s %s", a, b)
	}
}

func TestDeviceStartHandler(t *testing.T) {
	svc := &Service{
		cfg: Config{GitHubClientID: "cid"},
		client: &mockGitHub{start: DeviceStart{
			DeviceCode: "dev", UserCode: "ABCD-EFGH", VerificationURI: "https://github.com/login/device",
			ExpiresIn: 900, Interval: 5,
		}},
	}
	mux := http.NewServeMux()
	svc.Register(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/github/device", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var got DeviceStart
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.UserCode != "ABCD-EFGH" || got.DeviceCode != "dev" {
		t.Fatalf("%+v", got)
	}
}

func TestDevicePollPending(t *testing.T) {
	svc := &Service{
		cfg:    Config{GitHubClientID: "cid"},
		client: &mockGitHub{},
	}
	mux := http.NewServeMux()
	svc.Register(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/github/poll", strings.NewReader(`{"deviceCode":"dev"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "authorization_pending" {
		t.Fatalf("code=%q", body.Error.Code)
	}
}

func TestDevicePollErrorContracts(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"slow_down", ErrDeviceSlowDown, http.StatusAccepted, "slow_down"},
		{"expired", ErrDeviceExpired, http.StatusGone, "expired_token"},
		{"denied", ErrDeviceDenied, http.StatusForbidden, "access_denied"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{
				cfg: Config{GitHubClientID: "cid"},
				client: &mockGitHub{
					pollN: 1, // skip pending branch
					err:   tc.err,
				},
			}
			mux := http.NewServeMux()
			svc.Register(mux)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/github/poll", strings.NewReader(`{"deviceCode":"dev"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status %d body %s", w.Code, w.Body.String())
			}
			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != tc.code {
				t.Fatalf("code=%q want %q", body.Error.Code, tc.code)
			}
		})
	}
}

func TestDevicePollRejectsEmptyDeviceCode(t *testing.T) {
	svc := &Service{cfg: Config{GitHubClientID: "cid"}, client: &mockGitHub{}}
	mux := http.NewServeMux()
	svc.Register(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/github/poll", strings.NewReader(`{"deviceCode":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d", w.Code)
	}
}

func TestRequireUnauthorized(t *testing.T) {
	svc := &Service{}
	called := false
	h := svc.Require(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized || called {
		t.Fatalf("status %d called %v", w.Code, called)
	}
}

func TestDefaultOrgHex(t *testing.T) {
	id, err := bson.ObjectIDFromHex("660100000000000004000000")
	if err != nil || id.IsZero() {
		t.Fatal(err)
	}
}

func TestPrincipalContextRoundTrip(t *testing.T) {
	p := Principal{UserID: bson.NewObjectID(), Name: "n", OrganizationID: bson.NewObjectID()}
	got, ok := PrincipalFromContext(WithPrincipal(context.Background(), p))
	if !ok || got.UserID != p.UserID {
		t.Fatalf("%v %+v", ok, got)
	}
}
