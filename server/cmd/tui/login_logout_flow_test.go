package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Behavior inventory — login session lifecycle (TUI public seam):
//
// 1. /login Enter starts device flow against the auth API.
// 2. Successful authorize attaches in-memory session AND persists credentials.
// 3. /logout clears in-memory auth, credentials file, and blocks /search-learning.
// 4. Expired credentials on disk do not hydrate a session.
// 5. slow_down from poll HTTP increases the next wait by +5s.

func TestSlashLoginEnterStartsDeviceFlow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/github/device" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deviceCode": "dev", "userCode": "ENTER-OK",
				"verificationUri": "https://github.com/login/device",
				"expiresIn": 900, "interval": 5,
			})
			return
		}
		t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()
	t.Setenv("CORTISOL_SERVER_URL", srv.URL)
	t.Setenv("CORTISOL_CREDENTIALS", filepath.Join(t.TempDir(), "missing"))

	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.focus = -1
	m.draft.Focus()
	m.draft.SetValue("/login")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected beginLogin cmd")
	}
	msg := cmd()
	_, cmd = m.Update(msg)
	if !m.loginPending() || !strings.Contains(m.View(), "ENTER-OK") {
		t.Fatalf("device flow not started: pending=%v view=%s", m.loginPending(), m.View())
	}
	_ = cmd
}

func TestLogoutClearsSessionCredentialsAndBlocksSearch(t *testing.T) {
	credPath := filepath.Join(t.TempDir(), "credentials")
	t.Setenv("CORTISOL_CREDENTIALS", credPath)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/logout" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()
	t.Setenv("CORTISOL_SERVER_URL", srv.URL)

	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.auth = &authIdentity{Token: "live", GitHubLogin: "ada", OrgName: "Nova", OrgID: "org-1"}
	if err := saveCredentials(sessionCredentials{
		Token: "live", GitHubLogin: "ada", OrgName: "Nova", OrgID: "org-1",
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	logoutCmd := m.beginLogout()
	msg := logoutCmd()
	m.Update(msg)
	if m.auth != nil {
		t.Fatalf("auth still set: %+v", m.auth)
	}
	if !strings.Contains(m.clipboardNotice, "Logged out") {
		t.Fatalf("notice=%q", m.clipboardNotice)
	}
	if _, err := loadCredentials(); err == nil {
		t.Fatal("credentials file should be cleared")
	}

	handled, cmd := m.handleLearningCommand("/search-learning x")
	if !handled || cmd != nil || !strings.Contains(m.clipboardNotice, "/login") {
		t.Fatalf("search after logout: notice=%q cmd=%v", m.clipboardNotice, cmd != nil)
	}
}

func TestExpiredDiskCredentialsDoNotHydrateSession(t *testing.T) {
	credPath := filepath.Join(t.TempDir(), "credentials")
	t.Setenv("CORTISOL_CREDENTIALS", credPath)
	if err := saveCredentials(sessionCredentials{
		Token: "expired-tok", ExpiresAt: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	msg := m.loadAuthOnStart()()
	m.Update(msg)
	if m.auth != nil {
		t.Fatalf("expired creds hydrated auth: %+v", m.auth)
	}
	if m.ensureAuth() {
		t.Fatal("ensureAuth must reject expired credentials")
	}
}

func TestSlowDownFromPollHTTPIncreasesInterval(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"code": "slow_down", "message": "slow"},
		})
	}))
	defer srv.Close()
	t.Setenv("CORTISOL_SERVER_URL", srv.URL)

	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(authDeviceMsg{start: deviceStartResponse{
		DeviceCode: "dev", UserCode: "SLOW", VerificationURI: "https://github.com/login/device", Interval: 5,
	}})
	gen := m.authPollGen
	msg := m.doPoll("dev", 5*time.Second, gen)()
	pending, ok := msg.(authPendingMsg)
	if !ok {
		t.Fatalf("msg=%T %+v", msg, msg)
	}
	if pending.interval != 10*time.Second {
		t.Fatalf("interval=%v want 10s", pending.interval)
	}
	m.Update(pending)
	if m.authPollInterval != 10*time.Second {
		t.Fatalf("model interval=%v", m.authPollInterval)
	}
	if !m.loginPending() {
		t.Fatal("slow_down must keep banner pending")
	}
}
