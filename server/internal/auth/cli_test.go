package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func startTestHandoff(t *testing.T, s *Service) (string, string) {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleCLIStart(w, httptest.NewRequest("POST", "/api/v1/auth/cli/start", nil))
	if w.Code != 200 {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	var body struct{ ID, Secret string }
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.ID) != 64 || len(body.Secret) != 64 || body.ID == body.Secret {
		t.Fatal("handoff needs separate random approval and polling secrets")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("secret response is cacheable")
	}
	return body.ID, body.Secret
}

func approveTestHandoff(s *Service, id, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/v1/auth/cli/approve", strings.NewReader(`{"id":"`+id+`"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.handleCLIApprove(w, r)
	return w
}

func TestCLIHandoffRequiresSecretAndIsSingleUse(t *testing.T) {
	s := &Service{}
	id, secret := startTestHandoff(t, s)
	if _, status := s.claimCLI(id, secret); status != 202 {
		t.Fatalf("pending: %d", status)
	}
	if w := approveTestHandoff(s, id, "dashboard-session"); w.Code != 204 {
		t.Fatal(w.Body.String())
	}
	for _, wrong := range []string{"", id, "wrong-secret"} {
		if token, status := s.claimCLI(id, wrong); status != 401 || token != "" {
			t.Fatal("browser approval ID must not claim credentials")
		}
	}
	var claims atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, status := s.claimCLI(id, secret)
			if status == 200 {
				if token != "dashboard-session" {
					t.Error("terminal must share the dashboard session")
				}
				claims.Add(1)
			} else if status != 410 {
				t.Errorf("unexpected replay status %d", status)
			}
		}()
	}
	wg.Wait()
	if claims.Load() != 1 {
		t.Fatalf("claimed %d times", claims.Load())
	}
}

func TestCLIHandoffExpiryAndApprovalProtection(t *testing.T) {
	s := &Service{}
	mux := http.NewServeMux()
	s.Register(mux)
	r := httptest.NewRequest("POST", "/api/v1/auth/cli/approve", strings.NewReader(`{"id":"anything"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("approval must require dashboard authentication")
	}
	id, secret := startTestHandoff(t, s)
	approveTestHandoff(s, id, "first-session")
	if w := approveTestHandoff(s, id, "different-session"); w.Code != 409 {
		t.Fatal("approved identity can be replaced")
	}
	s.handoffs[id].expires = time.Now().Add(-time.Second)
	if w := approveTestHandoff(s, id, "first-session"); w.Code != 410 {
		t.Fatal("expired approval accepted")
	}
	if _, status := s.claimCLI(id, secret); status != 410 {
		t.Fatal("expired handoff claimed")
	}
}
