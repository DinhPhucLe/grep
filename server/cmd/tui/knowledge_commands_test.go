package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestParseSearchLearning(t *testing.T) {
	q, ok := parseSearchLearning("/search-learning payment retries")
	if !ok || q != "payment retries" {
		t.Fatalf("got %q ok=%v", q, ok)
	}
	if _, ok := parseSearchLearning("/search-learning"); ok {
		t.Fatal("empty query should fail")
	}
	if _, ok := parseSearchLearning("/search-learning   "); ok {
		t.Fatal("whitespace query should fail")
	}
	if _, ok := parseSearchLearning("/send-learning a | b"); ok {
		t.Fatal("wrong prefix")
	}
}

func TestParseSendLearning(t *testing.T) {
	topics, content, ok := parseSendLearning("/send-learning payments,retries | Use backoff on 429")
	if !ok {
		t.Fatal("expected ok")
	}
	if len(topics) != 2 || topics[0] != "payments" || topics[1] != "retries" {
		t.Fatalf("topics=%v", topics)
	}
	if content != "Use backoff on 429" {
		t.Fatalf("content=%q", content)
	}

	cases := []string{
		"/send-learning",
		"/send-learning only topics",
		"/send-learning | body only",
		"/send-learning , , | body",
		"/send-learning payments |",
		"/search-learning foo",
	}
	for _, line := range cases {
		if _, _, ok := parseSendLearning(line); ok {
			t.Fatalf("expected fail for %q", line)
		}
	}
}

func TestHandleLearningCommandRequiresLogin(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	t.Setenv("CORTISOL_CREDENTIALS", filepath.Join(t.TempDir(), "missing"))
	handled, cmd := m.handleLearningCommand("/search-learning foo")
	if !handled || cmd != nil {
		t.Fatalf("handled=%v cmd=%v", handled, cmd != nil)
	}
	if !strings.Contains(m.clipboardNotice, "/login") {
		t.Fatalf("notice=%q", m.clipboardNotice)
	}
}

func TestSearchLearningTypoHint(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	handled, cmd := m.handleLearningCommand("/search-learnign payments")
	if !handled || cmd != nil {
		t.Fatalf("handled=%v cmd=%v", handled, cmd != nil)
	}
	if !strings.Contains(m.clipboardNotice, "Usage: /search-learning") {
		t.Fatalf("notice=%q", m.clipboardNotice)
	}
}

func TestSearchLearningNoticeBeatsLoginBanner(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m.authDeviceCode = "dev"
	m.authUserCode = "CODE"
	m.authVerificationURI = "https://github.com/login/device"
	m.clipboardNotice = "Knowledge search failed: boom"
	view := m.View()
	if !strings.Contains(view, "Knowledge search failed") {
		t.Fatalf("error hidden behind login banner: %s", view)
	}
	if strings.Contains(view, "enter CODE") {
		t.Fatalf("login banner should not win over search error: %s", view)
	}
}

func TestHandleLearningCommandHydratesFromCredentials(t *testing.T) {
	credPath := filepath.Join(t.TempDir(), "credentials")
	t.Setenv("CORTISOL_CREDENTIALS", credPath)
	if err := saveCredentials(sessionCredentials{
		Token:       "disk-token",
		GitHubLogin: "alice",
		OrgID:       "org-1",
		OrgName:     "Org",
		ExpiresAt:   time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	if m.auth != nil {
		t.Fatal("expected nil in-memory auth before hydrate")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer disk-token" {
			t.Fatalf("auth=%q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, knowledgeFixtureJSON)
	}))
	defer server.Close()
	t.Setenv("CORTISOL_SERVER_URL", server.URL)

	handled, cmd := m.handleLearningCommand("/search-learning payment retries")
	if !handled || cmd == nil {
		t.Fatal("expected search cmd after hydrate")
	}
	if m.auth == nil || m.auth.Token != "disk-token" {
		t.Fatalf("auth=%v", m.auth)
	}
	msg := cmd()
	m.Update(msg)
	if len(m.knowledgeHits) != 5 {
		t.Fatalf("hits=%d", len(m.knowledgeHits))
	}
}

func TestLearningCommandAfterAuthSessionMsg(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.focus = -1
	m.draft.Focus()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sess" {
			t.Fatalf("auth=%q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, knowledgeFixtureJSON)
	}))
	defer server.Close()
	t.Setenv("CORTISOL_SERVER_URL", server.URL)
	t.Setenv("CORTISOL_CREDENTIALS", filepath.Join(t.TempDir(), "credentials"))

	m.Update(authSessionMsg{creds: sessionCredentials{
		Token:       "sess",
		GitHubLogin: "bob",
		OrgID:       "org-1",
		OrgName:     "Org",
	}})
	if m.auth == nil || m.auth.Token != "sess" {
		t.Fatal("expected attachAuth from authSessionMsg")
	}

	m.draft.SetValue("/search-learning payment retries")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected search cmd, notice=%q auth=%v", m.clipboardNotice, m.auth)
	}
	if strings.Contains(m.clipboardNotice, "/login") {
		t.Fatalf("still asking for login: %q", m.clipboardNotice)
	}
	m.Update(cmd())
	if len(m.knowledgeHits) != 5 {
		t.Fatalf("hits=%d", len(m.knowledgeHits))
	}
}

func TestHandleLearningCommandUsage(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	t.Setenv("CORTISOL_CREDENTIALS", filepath.Join(t.TempDir(), "missing"))
	m.auth = &authIdentity{Token: "tok", OrgID: "org-1"}

	handled, cmd := m.handleLearningCommand("/search-learning")
	if !handled || cmd != nil {
		t.Fatal("expected usage without cmd")
	}
	if !strings.Contains(m.clipboardNotice, "Usage: /search-learning") {
		t.Fatalf("notice=%q", m.clipboardNotice)
	}

	handled, cmd = m.handleLearningCommand("/send-learning bad")
	if !handled || cmd != nil {
		t.Fatal("expected send usage")
	}
	if !strings.Contains(m.clipboardNotice, "Usage: /send-learning") {
		t.Fatalf("notice=%q", m.clipboardNotice)
	}
}

func TestRunKnowledgeSearchFeedsPicker(t *testing.T) {
	var gotAuth, gotQuery, gotOrg, gotK string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/knowledge" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query().Get("query")
		gotOrg = r.URL.Query().Get("organizationId")
		gotK = r.URL.Query().Get("k")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, knowledgeFixtureJSON)
	}))
	defer server.Close()
	t.Setenv("CORTISOL_SERVER_URL", server.URL)

	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.auth = &authIdentity{Token: "sess-token", OrgID: "org-novapay"}

	handled, cmd := m.handleLearningCommand("/search-learning payment retries")
	if !handled || cmd == nil {
		t.Fatal("expected search cmd")
	}
	if !m.knowledgeSearching {
		t.Fatal("expected searching after begin")
	}

	msg := cmd()
	m.Update(msg)

	if gotAuth != "Bearer sess-token" {
		t.Fatalf("auth=%q", gotAuth)
	}
	if gotQuery != "payment retries" || gotOrg != "org-novapay" || gotK != "5" {
		t.Fatalf("query=%q org=%q k=%q", gotQuery, gotOrg, gotK)
	}
	if m.knowledgeSearching {
		t.Fatal("still searching")
	}
	if len(m.knowledgeHits) != 5 {
		t.Fatalf("hits=%d", len(m.knowledgeHits))
	}
	if !strings.Contains(m.View(), "5 matches") {
		t.Fatalf("view=%s", m.View())
	}
}

func TestRunKnowledgePost(t *testing.T) {
	var gotAuth string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/knowledge" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"doc-1"}`))
	}))
	defer server.Close()
	t.Setenv("CORTISOL_SERVER_URL", server.URL)

	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.auth = &authIdentity{Token: "tok"}

	handled, cmd := m.handleLearningCommand("/send-learning payments | Use backoff")
	if !handled || cmd == nil {
		t.Fatal("expected post cmd")
	}
	msg := cmd()
	m.Update(msg)

	if gotAuth != "Bearer tok" {
		t.Fatalf("auth=%q", gotAuth)
	}
	if body["content"] != "Use backoff" {
		t.Fatalf("body=%v", body)
	}
	topics, _ := body["topics"].([]any)
	if len(topics) != 1 || topics[0] != "payments" {
		t.Fatalf("topics=%v", topics)
	}
	if m.clipboardNotice != "Posted learning" {
		t.Fatalf("notice=%q", m.clipboardNotice)
	}
}
