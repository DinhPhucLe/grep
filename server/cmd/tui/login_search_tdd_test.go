package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Behavior inventory (login + search seam):
// 1. Active device-flow errors must surface and end the banner (not silent-ignore "empty token").
// 2. Startup credential-load misses stay quiet when no device flow is active.
// 3. Successful session clears the banner and makes /search-learning issue a search cmd.
// 4. Late browser-assist messages after success must not resurrect the banner.
// 5. /search_learning and /SEARCH-LEARNING are accepted as the same command.

func TestActiveLoginEmptyTokenErrorSurfacesAndClearsBanner(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	t.Setenv("CORTISOL_CREDENTIALS", filepath.Join(t.TempDir(), "missing"))

	m.Update(authDeviceMsg{start: deviceStartResponse{
		DeviceCode:      "dev",
		UserCode:        "STUCK-CODE",
		VerificationURI: "https://github.com/login/device",
		Interval:        5,
	}})
	if !m.loginPending() {
		t.Fatal("expected pending device flow")
	}

	// Poll-path empty token (same gen as the device flow) must surface and end
	// the banner. Startup loadAuthOnStart misses use gen==0 and must not abort
	// an in-flight flow — covered separately.
	m.Update(authSessionMsg{err: fmt.Errorf("empty session token from server"), gen: m.authPollGen})

	if m.loginPending() {
		t.Fatal("active login error left device-flow banner pending forever")
	}
	if m.auth != nil {
		t.Fatal("error path must not attach auth")
	}
	if !strings.Contains(m.clipboardNotice, "Login:") || !strings.Contains(m.clipboardNotice, "empty") {
		t.Fatalf("expected visible login error, notice=%q", m.clipboardNotice)
	}
	if strings.Contains(m.View(), "STUCK-CODE") {
		t.Fatalf("device code still on screen after failed poll: %s", m.View())
	}
}

func TestStartupEmptyTokenStaysQuietWithoutDeviceFlow(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(authSessionMsg{err: fmt.Errorf("empty token")})
	if m.clipboardNotice != "" {
		t.Fatalf("startup miss should stay quiet, notice=%q", m.clipboardNotice)
	}
	if m.loginPending() {
		t.Fatal("should not start device flow")
	}
}

func TestLoginSuccessThenSearchLearningWorks(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m.clipboard = &memClipboard{}
	t.Setenv("CORTISOL_CREDENTIALS", filepath.Join(t.TempDir(), "credentials"))

	m.Update(authDeviceMsg{start: deviceStartResponse{
		DeviceCode:      "dev",
		UserCode:        "OK-CODE",
		VerificationURI: "https://github.com/login/device",
		Interval:        5,
	}})
	m.Update(authSessionMsg{creds: sessionCredentials{
		Token:       "sess-token",
		GitHubLogin: "alice",
		OrgID:       "org-1",
		OrgName:     "Org",
	}})

	if m.loginPending() {
		t.Fatal("banner still pending after success")
	}
	if m.auth == nil || m.auth.Token != "sess-token" {
		t.Fatalf("auth not attached: %+v", m.auth)
	}
	if strings.Contains(m.View(), "OK-CODE") || strings.Contains(m.View(), "github.com/login/device") {
		t.Fatalf("login banner still visible: %s", m.View())
	}

	// Simulate typing after login (clears "Logged in as" notice the way the TUI does).
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if m.loginPending() {
		t.Fatal("typing resurrected login pending")
	}

	handled, cmd := m.handleLearningCommand("/search-learning payment retries")
	if !handled {
		t.Fatal("search command not handled")
	}
	if cmd == nil {
		t.Fatalf("search failed after login, notice=%q auth=%v", m.clipboardNotice, m.auth)
	}
	if strings.Contains(m.clipboardNotice, "/login") {
		t.Fatalf("asked to login despite session: %q", m.clipboardNotice)
	}
}

func TestSearchLearningAcceptsUnderscoreAndCase(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.auth = &authIdentity{Token: "tok", OrgID: "org-1"}

	for _, line := range []string{
		"/search_learning payment retries",
		"/SEARCH-LEARNING payment retries",
		"/Search-Learning payment retries",
	} {
		handled, cmd := m.handleLearningCommand(line)
		if !handled || cmd == nil {
			t.Fatalf("%q: handled=%v cmd=%v notice=%q", line, handled, cmd != nil, m.clipboardNotice)
		}
	}
}

func TestLateLoginAssistAfterSuccessDoesNotResurrectBanner(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.Update(authSessionMsg{creds: sessionCredentials{
		Token: "tok", GitHubLogin: "alice", OrgName: "Org",
	}})
	m.Update(authLoginAssistMsg{copied: true, opened: true})

	if m.loginPending() {
		t.Fatal("assist resurrected pending login")
	}
	if strings.Contains(m.View(), "enter it on GitHub") || strings.Contains(m.View(), "github.com/login/device") {
		t.Fatalf("assist resurrected login UI: %s", m.View())
	}
}
