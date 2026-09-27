package participant

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testID = "12345678-1234-4123-8123-123456789abc"

func TestCanonicalParticipantID(t *testing.T) {
	for _, id := range []string{"", strings.ToUpper(testID), "12345678-1234-1123-8123-123456789abc", "12345678-1234-4123-7123-123456789abc", testID + " "} {
		if ValidID(id) {
			t.Errorf("accepted invalid ID %q", id)
		}
	}
	if !ValidID(testID) {
		t.Fatal("canonical v4 ID rejected")
	}
	seen := map[string]bool{}
	for range 100 {
		id := NewID()
		if !ValidID(id) || seen[id] {
			t.Fatalf("invalid or repeated generated ID %q", id)
		}
		seen[id] = true
	}
}

type fakeRepository struct {
	input    Registration
	calls    int
	err      error
	deadline bool
}

func (f *fakeRepository) Register(ctx context.Context, input Registration) (Profile, error) {
	f.input = input
	f.calls++
	_, f.deadline = ctx.Deadline()
	return Profile{ID: input.ID, DisplayName: input.DisplayName, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}, f.err
}
func (*fakeRepository) Exists(context.Context, string) (bool, error) { return true, nil }

func TestRegistrationHandlerValidatesBeforePersistence(t *testing.T) {
	for _, tc := range []struct {
		name, method, media, body string
		status                    int
	}{
		{"valid", "POST", "application/json", `{"id":"` + testID + `","display_name":"Ada"}`, 200},
		{"missing id", "POST", "application/json", `{}`, 400},
		{"null", "POST", "application/json", `null`, 400},
		{"unknown field", "POST", "application/json", `{"id":"` + testID + `","password":"x"}`, 400},
		{"extra JSON", "POST", "application/json", `{"id":"` + testID + `"}{}`, 400},
		{"large", "POST", "application/json", `{"id":"` + testID + `","display_name":"` + strings.Repeat("x", 4096) + `"}`, 413},
		{"long name", "POST", "application/json", `{"id":"` + testID + `","display_name":"` + strings.Repeat("x", 101) + `"}`, 400},
		{"control name", "POST", "application/json", `{"id":"` + testID + `","display_name":"a\nb"}`, 400},
		{"media", "POST", "text/plain", `{}`, 415},
		{"list", "GET", "application/json", `{}`, 405},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{}
			req := httptest.NewRequest(tc.method, "/participants", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.media)
			w := httptest.NewRecorder()
			NewHandler(repo, time.Second)(w, req)
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", w.Code, tc.status, w.Body.String())
			}
			if tc.status != 200 {
				if repo.calls != 0 {
					t.Fatal("invalid request persisted")
				}
				return
			}
			var profile Profile
			if err := json.Unmarshal(w.Body.Bytes(), &profile); err != nil {
				t.Fatal(err)
			}
			if profile.ID != testID || profile.DisplayName != "Ada" || !repo.deadline {
				t.Fatalf("profile/deadline mismatch: %+v", profile)
			}
		})
	}
}

func TestRegistrationErrorsAreSafe(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{errors.New("mongodb://secret-password"), 500, "registration_failed"},
		{context.DeadlineExceeded, 504, "registration_timeout"},
		{ErrMigrationRequired, 503, "migration_required"},
	} {
		req := httptest.NewRequest(http.MethodPost, "/participants", strings.NewReader(`{"id":"`+testID+`"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		NewHandler(&fakeRepository{err: tc.err}, time.Second)(w, req)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) || strings.Contains(w.Body.String(), "secret-password") {
			t.Fatalf("unsafe/wrong error: %d %s", w.Code, w.Body.String())
		}
	}
}
