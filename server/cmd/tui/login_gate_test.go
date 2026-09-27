package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLoginGateBlocksInputAndSavesDashboardSession(t *testing.T) {
	t.Setenv("CORTISOL_CREDENTIALS", filepath.Join(t.TempDir(), "credentials"))
	g := &loginGate{ctx: context.Background(), width: 80, height: 24}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("run something")}, {Type: tea.KeyRunes, Runes: []rune("o"), Paste: true}} {
		if _, cmd := g.Update(key); cmd != nil {
			t.Fatal("signed-out input ran an action")
		}
	}
	id := strings.Repeat("a", 64)
	g.handoff = terminalHandoff{ID: id, Secret: "terminal-only"}
	g.Update(gatePolled{id: "old-request", session: sessionAPIResponse{Token: "wrong"}})
	if g.creds.Token != "" {
		t.Fatal("stale login accepted")
	}
	session := sessionAPIResponse{Token: "shared-dashboard-token", ExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339)}
	session.User.ID, session.User.GitHubLogin = "user-id", "octocat"
	session.Organization.ID = "org-id"
	_, cmd := g.Update(gatePolled{id: id, session: session})
	if cmd == nil || g.creds.Token != session.Token {
		t.Fatal("dashboard sign-in did not unlock terminal")
	}
	saved, err := loadCredentials()
	if err != nil || saved.Token != session.Token || saved.UserID != "user-id" {
		t.Fatalf("saved session: %+v %v", saved, err)
	}
}

func TestDashboardURLContainsNoPollingSecret(t *testing.T) {
	t.Setenv("CORTISOL_DASHBOARD_URL", "https://dashboard.example.com/")
	link, err := dashboardLink(strings.Repeat("b", 64))
	if err != nil || !strings.Contains(link, "#terminal=") {
		t.Fatalf("%s %v", link, err)
	}
	t.Setenv("CORTISOL_DASHBOARD_URL", "javascript:alert(1)")
	if _, err := dashboardLink("id"); err == nil {
		t.Fatal("unsafe dashboard scheme accepted")
	}
}

func TestSavedSessionRequiresServerValidation(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v1/auth/me" || r.Header.Get("Authorization") != "Bearer saved-token" {
			t.Error("wrong validation request")
		}
		w.WriteHeader(401)
	}))
	defer srv.Close()
	t.Setenv("CORTISOL_SERVER_URL", srv.URL)
	t.Setenv("CORTISOL_CREDENTIALS", filepath.Join(t.TempDir(), "credentials"))
	creds := sessionCredentials{Token: "saved-token", ServerURL: srv.URL, ExpiresAt: time.Now().Add(time.Hour)}
	if err := saveCredentials(creds); err != nil {
		t.Fatal(err)
	}
	g := &loginGate{ctx: context.Background(), checking: true}
	_, cmd := g.Update(g.Init()())
	if g.creds.Token != "" || cmd != nil || calls != 1 {
		t.Fatal("revoked saved session unlocked TUI")
	}
	creds.ServerURL = "https://other-server.example"
	if _, err := validateSession(context.Background(), creds); err == nil || calls != 1 {
		t.Fatal("credential sent to a different server")
	}
}

func TestSessionRevocationLocksWorkspace(t *testing.T) {
	m := newModel(nil, ".", uiOptions{})
	m.authEnforced = true
	m.setAuthIdentity(sessionCredentials{Token: "shared-token", UserID: "user"})
	m.connected = true
	m.draft.SetValue("run a command")
	_, cmd := m.Update(sessionChecked{token: "old-token", err: errSessionInvalid})
	if cmd != nil || m.auth == nil {
		t.Fatal("stale session check revoked current login")
	}
	_, cmd = m.Update(sessionChecked{token: "shared-token", err: errSessionInvalid})
	if cmd == nil || m.auth != nil || !m.returnToLogin {
		t.Fatal("workspace did not lock after logout")
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil || m.evaluating {
		t.Fatal("signed-out prompt was submitted")
	}
	if cmd := m.call("turn/start", nil); cmd != nil {
		t.Fatal("signed-out agent action permitted")
	}
	if !strings.Contains(m.View(), "Sign-in required") {
		t.Fatal("workspace still displayed while signed out")
	}
}

func TestDashboardHandoffPollingUsesPrivateSecret(t *testing.T) {
	id := strings.Repeat("c", 64)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/cli/poll" {
			t.Error(r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["id"] != id || body["secret"] != "private" {
			t.Error("missing proof from terminal")
		}
		w.WriteHeader(202)
		_, _ = w.Write([]byte(`{"error":{"code":"authorization_pending"}}`))
	}))
	defer srv.Close()
	t.Setenv("CORTISOL_SERVER_URL", srv.URL)
	g := &loginGate{ctx: context.Background(), handoff: terminalHandoff{ID: id, Secret: "private"}}
	_, cmd := g.Update(gatePoll{id})
	msg := cmd().(gatePolled)
	if msg.err != nil || !msg.pending {
		t.Fatalf("%+v", msg)
	}
	if _, next := g.Update(msg); next == nil || g.creds.Token != "" {
		t.Fatal("pending authorization unlocked TUI or stopped polling")
	}
}

func TestLogoutFailurePreservesSessionForRetry(t *testing.T) {
	m := newModel(nil, ".", uiOptions{})
	m.authEnforced = true
	m.authCreds = sessionCredentials{Token: "still-active"}
	m.handleAuthMsg(authLogoutMsg{err: errors.New("offline")})
	if m.auth == nil || m.returnToLogin || !strings.Contains(m.clipboardNotice, "Logout failed") {
		t.Fatal("failed logout silently discarded shared session")
	}
}
