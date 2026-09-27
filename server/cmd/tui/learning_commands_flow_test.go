package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Behavior inventory — /search-learning and /send-learning (seam: handleLearningCommand + HTTP):
//
// S1. Unauthenticated search/send demand /login (no HTTP).
// S2. Authenticated search GETs /api/v1/knowledge with bearer, query, org, k=5; opens picker.
// S3. Zero hits → "No knowledge matches".
// S4. Search HTTP/API failure → visible "Knowledge search failed".
// S5. Authenticated send POSTs content+topics with bearer → "Posted learning".
// S6. Send HTTP/API failure → "Send learning failed".
// S7. Send aliases (_ / case) and content that contains extra | after the first split.
// S8. Draft Enter runs send the same as handleLearningCommand.
// S9. Send typo hint for near-miss command names.

func TestSendLearningRequiresLogin(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	t.Setenv("CORTISOL_CREDENTIALS", filepath.Join(t.TempDir(), "missing"))
	handled, cmd := m.handleLearningCommand("/send-learning payments | Use backoff")
	if !handled || cmd != nil {
		t.Fatalf("handled=%v cmd=%v", handled, cmd != nil)
	}
	if !strings.Contains(m.clipboardNotice, "/login") {
		t.Fatalf("notice=%q", m.clipboardNotice)
	}
}

func TestSearchLearningEmptyHitsNotice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[]}`)
	}))
	defer srv.Close()
	t.Setenv("CORTISOL_SERVER_URL", srv.URL)

	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.auth = &authIdentity{Token: "tok", OrgID: "org-1"}

	_, cmd := m.handleLearningCommand("/search-learning nothing-here")
	m.Update(cmd())
	if m.clipboardNotice != "No knowledge matches" {
		t.Fatalf("notice=%q", m.clipboardNotice)
	}
	if len(m.knowledgeHits) != 0 {
		t.Fatalf("hits=%d", len(m.knowledgeHits))
	}
}

func TestSearchLearningHTTPFailureNotice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"code": "unauthorized", "message": "bearer required"},
		})
	}))
	defer srv.Close()
	t.Setenv("CORTISOL_SERVER_URL", srv.URL)

	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.auth = &authIdentity{Token: "bad", OrgID: "org-1"}
	_, cmd := m.handleLearningCommand("/search-learning x")
	m.Update(cmd())
	if !strings.Contains(m.clipboardNotice, "Knowledge search failed") {
		t.Fatalf("notice=%q", m.clipboardNotice)
	}
}

func TestSendLearningHTTPFailureNotice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"code": "forbidden", "message": "denied"},
		})
	}))
	defer srv.Close()
	t.Setenv("CORTISOL_SERVER_URL", srv.URL)

	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.auth = &authIdentity{Token: "tok"}
	_, cmd := m.handleLearningCommand("/send-learning payments | body")
	m.Update(cmd())
	if !strings.Contains(m.clipboardNotice, "Send learning failed") {
		t.Fatalf("notice=%q", m.clipboardNotice)
	}
}

func TestSendLearningAliasesAndPipeInContent(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		bodies = append(bodies, body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"1"}`))
	}))
	defer srv.Close()
	t.Setenv("CORTISOL_SERVER_URL", srv.URL)

	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.auth = &authIdentity{Token: "tok"}

	for _, line := range []string{
		"/send_learning a,b | first",
		"/SEND-LEARNING a | second | still content",
	} {
		handled, cmd := m.handleLearningCommand(line)
		if !handled || cmd == nil {
			t.Fatalf("%q: handled=%v cmd=%v notice=%q", line, handled, cmd != nil, m.clipboardNotice)
		}
		m.Update(cmd())
		if m.clipboardNotice != "Posted learning" {
			t.Fatalf("%q notice=%q", line, m.clipboardNotice)
		}
	}
	if len(bodies) != 2 {
		t.Fatalf("posts=%d", len(bodies))
	}
	if bodies[1]["content"] != "second | still content" {
		t.Fatalf("pipe content=%v", bodies[1]["content"])
	}
	topics, _ := bodies[0]["topics"].([]any)
	if len(topics) != 2 || topics[0] != "a" || topics[1] != "b" {
		t.Fatalf("topics=%v", bodies[0]["topics"])
	}
}

func TestSendLearningEnterFromDraft(t *testing.T) {
	var posted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/knowledge" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		posted = true
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"1"}`))
	}))
	defer srv.Close()
	t.Setenv("CORTISOL_SERVER_URL", srv.URL)
	t.Setenv("CORTISOL_CREDENTIALS", filepath.Join(t.TempDir(), "missing"))

	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.focus = -1
	m.draft.Focus()
	m.auth = &authIdentity{Token: "tok", OrgID: "org-1"}
	m.draft.SetValue("/send-learning retries | Prefer idempotent POSTs")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected post cmd, notice=%q", m.clipboardNotice)
	}
	m.Update(cmd())
	if !posted {
		t.Fatal("POST not issued")
	}
	if m.clipboardNotice != "Posted learning" {
		t.Fatalf("notice=%q", m.clipboardNotice)
	}
}

func TestSendLearningTypoHint(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	handled, cmd := m.handleLearningCommand("/send-learnign x | y")
	if !handled || cmd != nil {
		t.Fatalf("handled=%v cmd=%v", handled, cmd != nil)
	}
	if !strings.Contains(m.clipboardNotice, "Usage: /send-learning") {
		t.Fatalf("notice=%q", m.clipboardNotice)
	}
}
