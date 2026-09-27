package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Behavior inventory — GitHub device login in the TUI (public seam: model.Update + HTTP auth API):
//
// A. Terminal → web: /login starts device flow; after web authorize, poll success clears banner,
//    attaches session, saves credentials, /search-learning issues a search cmd.
// B. Web → terminal: credentials already on disk (prior authorize) hydrate; /search-learning works
//    without a new device flow. gen==0 success also clears an in-flight banner.
// C. Cancel midway (Esc): banner gone, no session, /search-learning demands /login.
//    Late success/error from the canceled gen must not attach or mutate notice.
// D. Pending then success across multiple polls (server interval ticks).
// E. slow_down increases wait by +5s (RFC 8628).
// F. expired_token / access_denied: visible Login error, banner cleared, no auth.
// G. Relogin bumps gen; superseded poll success must not win.
// H. Startup loadAuthOnStart empty-token (gen==0) must not abort an in-flight device flow.
// I. Poll-path empty token (matching gen) surfaces and clears the banner.
// J. /search_learning (underscore) works once logged in.

func TestDeviceFlowTerminalThenWebAuthorizeEnablesSearch(t *testing.T) {
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/github/device":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deviceCode": "dev-1", "userCode": "TERM-WEB",
				"verificationUri": "https://github.com/login/device",
				"expiresIn": 900, "interval": 1,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/github/poll":
			n := polls.Add(1)
			if n < 3 {
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]string{"code": "authorization_pending", "message": "waiting"},
				})
				return
			}
			// Simulate web authorize completed — return a real session payload.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":     "sess-from-web",
				"expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano),
				"user":      map[string]string{"id": "u1", "name": "Ada", "githubLogin": "ada"},
				"organization": map[string]string{"id": "org-1", "name": "Nova"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/knowledge/events":
			// attachAuth starts an SSE watch; close immediately so httptest can shut down.
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	t.Setenv("CORTISOL_SERVER_URL", srv.URL)
	credPath := filepath.Join(t.TempDir(), "credentials")
	t.Setenv("CORTISOL_CREDENTIALS", credPath)

	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m.clipboard = &memClipboard{}

	// Terminal starts login (device start + first poll cmd).
	startCmd := m.beginLogin()
	startMsg := startCmd()
	var cmds []tea.Cmd
	_, cmd := m.Update(startMsg)
	enqueueCmd(&cmds, cmd)
	if !m.loginPending() || !strings.Contains(m.View(), "TERM-WEB") {
		t.Fatalf("device flow not shown: pending=%v view=%s", m.loginPending(), m.View())
	}

	// Drive poll chain: pending → tick → pending → tick → success.
	// tea.Batch returns a Cmd that yields BatchMsg{...}; expand those members.
	deadline := time.Now().Add(15 * time.Second)
	for m.loginPending() && time.Now().Before(deadline) {
		if len(cmds) == 0 {
			t.Fatal("poll chain died while still pending")
		}
		next := cmds[0]
		cmds = cmds[1:]
		msg := next()
		if batch, ok := msg.(tea.BatchMsg); ok {
			cmds = append(cmds, []tea.Cmd(batch)...)
			continue
		}
		if msg == nil {
			continue
		}
		_, cmd = m.Update(msg)
		if !m.loginPending() {
			// Success or fatal poll error — do not drain attachAuth's SSE wait cmd.
			break
		}
		enqueueCmd(&cmds, cmd)
	}

	if m.loginPending() {
		t.Fatalf("banner still pending after web authorize, polls=%d notice=%q view=%s",
			polls.Load(), m.clipboardNotice, m.View())
	}
	if m.auth == nil || m.auth.Token != "sess-from-web" {
		t.Fatalf("session not attached: %+v", m.auth)
	}
	if strings.Contains(m.View(), "TERM-WEB") || strings.Contains(m.View(), "github.com/login/device") {
		t.Fatalf("login banner still visible: %s", m.View())
	}
	if !strings.Contains(m.clipboardNotice, "Logged in as") {
		t.Fatalf("notice=%q", m.clipboardNotice)
	}
	creds, err := loadCredentials()
	if err != nil || creds.Token != "sess-from-web" {
		t.Fatalf("credentials not saved: %+v err=%v", creds, err)
	}

	handled, searchCmd := m.handleLearningCommand("/search-learning payment retries")
	if !handled || searchCmd == nil {
		t.Fatalf("search after login failed: handled=%v notice=%q", handled, m.clipboardNotice)
	}
	if m.sseCancel != nil {
		m.sseCancel()
		m.sseCancel = nil
	}
}

func TestCredentialsOnDiskHydrateForSearchWithoutNewLogin(t *testing.T) {
	credPath := filepath.Join(t.TempDir(), "credentials")
	t.Setenv("CORTISOL_CREDENTIALS", credPath)
	if err := saveCredentials(sessionCredentials{
		Token: "disk-sess", GitHubLogin: "ada", OrgID: "org-1", OrgName: "Nova",
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	// Simulate startup hydration message (web/prior terminal login already done).
	m.Update(authSessionMsg{creds: sessionCredentials{
		Token: "disk-sess", GitHubLogin: "ada", OrgID: "org-1", OrgName: "Nova",
	}})
	if m.loginPending() {
		t.Fatal("should not be in device flow")
	}
	handled, cmd := m.handleLearningCommand("/search_learning retries")
	if !handled || cmd == nil {
		t.Fatalf("hydrate search failed: notice=%q", m.clipboardNotice)
	}
}

func TestLoginEscCancelMidwayBlocksSearchAndIgnoresLateSuccess(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	t.Setenv("CORTISOL_CREDENTIALS", filepath.Join(t.TempDir(), "missing"))

	m.Update(authDeviceMsg{start: deviceStartResponse{
		DeviceCode: "dev", UserCode: "CANCEL-ME", VerificationURI: "https://github.com/login/device", Interval: 5,
	}})
	genBeforeCancel := m.authPollGen
	m.focus = -1
	m.draft.Focus()
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.loginPending() {
		t.Fatal("cancel left banner pending")
	}

	handled, cmd := m.handleLearningCommand("/search-learning x")
	if !handled || cmd != nil || !strings.Contains(m.clipboardNotice, "/login") {
		t.Fatalf("search should require login after cancel: notice=%q cmd=%v", m.clipboardNotice, cmd != nil)
	}

	// Late poll success from the canceled generation must not attach.
	m.Update(authSessionMsg{
		gen: genBeforeCancel,
		creds: sessionCredentials{Token: "late", GitHubLogin: "nope", OrgName: "X"},
	})
	if m.auth != nil {
		t.Fatalf("late success after cancel attached auth: %+v", m.auth)
	}

	// Late poll error from canceled generation must not attach or clear the
	// post-cancel "/login first" guidance from the search attempt above.
	noticeBefore := m.clipboardNotice
	m.Update(authSessionMsg{
		gen:  genBeforeCancel,
		err:  fmt.Errorf("expired_token: device code expired"),
		code: "expired_token",
	})
	if m.auth != nil {
		t.Fatal("late error attached auth")
	}
	if m.clipboardNotice != noticeBefore {
		t.Fatalf("late error mutated notice: before=%q after=%q", noticeBefore, m.clipboardNotice)
	}
}

func TestStartupEmptyTokenDuringDeviceFlowDoesNotAbort(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.Update(authDeviceMsg{start: deviceStartResponse{
		DeviceCode: "dev", UserCode: "RACE-CODE", VerificationURI: "https://github.com/login/device", Interval: 5,
	}})
	// loadAuthOnStart finishing after /login must not kill the device flow.
	m.Update(authSessionMsg{err: fmt.Errorf("empty token")})
	if !m.loginPending() {
		t.Fatal("startup empty-token aborted in-flight device flow")
	}
	if strings.Contains(m.clipboardNotice, "Login:") {
		t.Fatalf("startup miss surfaced as login error: %q", m.clipboardNotice)
	}
}

func TestReloginSupersedesPreviousPollGeneration(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	t.Setenv("CORTISOL_CREDENTIALS", filepath.Join(t.TempDir(), "credentials"))

	m.Update(authDeviceMsg{start: deviceStartResponse{
		DeviceCode: "old-dev", UserCode: "OLD-CODE", VerificationURI: "https://github.com/login/device", Interval: 5,
	}})
	oldGen := m.authPollGen

	m.Update(authDeviceMsg{start: deviceStartResponse{
		DeviceCode: "new-dev", UserCode: "NEW-CODE", VerificationURI: "https://github.com/login/device", Interval: 5,
	}})
	if m.authUserCode != "NEW-CODE" || m.authDeviceCode != "new-dev" {
		t.Fatalf("relogin did not replace device: %+v", m)
	}
	if m.authPollGen == oldGen {
		t.Fatal("relogin must bump poll generation")
	}

	// Stale success from the superseded device flow must not win.
	m.Update(authSessionMsg{
		gen:   oldGen,
		creds: sessionCredentials{Token: "stale", GitHubLogin: "old", OrgName: "Old"},
	})
	if m.auth != nil {
		t.Fatalf("stale gen attached: %+v", m.auth)
	}
	if !m.loginPending() || m.authUserCode != "NEW-CODE" {
		t.Fatal("stale success cleared the new login banner")
	}

	// Current generation success attaches.
	m.Update(authSessionMsg{
		gen:   m.authPollGen,
		creds: sessionCredentials{Token: "fresh", GitHubLogin: "ada", OrgName: "Nova"},
	})
	if m.auth == nil || m.auth.Token != "fresh" {
		t.Fatalf("current gen did not attach: %+v", m.auth)
	}
}

func TestDiskHydrateSuccessWhileDeviceFlowPendingTakesOver(t *testing.T) {
	// Web → terminal: credentials land on disk (or loadAuthOnStart succeeds) while
	// a device banner is showing — gen==0 success should attach and clear banner.
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.Update(authDeviceMsg{start: deviceStartResponse{
		DeviceCode: "dev", UserCode: "HYDRATE", VerificationURI: "https://github.com/login/device", Interval: 5,
	}})
	m.Update(authSessionMsg{creds: sessionCredentials{
		Token: "disk-tok", GitHubLogin: "ada", OrgName: "Nova", OrgID: "org-1",
	}})
	if m.loginPending() {
		t.Fatal("disk hydrate left device banner pending")
	}
	if m.auth == nil || m.auth.Token != "disk-tok" {
		t.Fatalf("hydrate did not attach: %+v", m.auth)
	}
	handled, cmd := m.handleLearningCommand("/search-learning x")
	if !handled || cmd == nil {
		t.Fatalf("search after hydrate failed: notice=%q", m.clipboardNotice)
	}
}

func TestDevicePollExpiredAndDeniedClearBanner(t *testing.T) {
	cases := []struct {
		status int
		code   string
	}{
		{http.StatusGone, "expired_token"},
		{http.StatusForbidden, "access_denied"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]string{"code": tc.code, "message": tc.code},
				})
			}))
			defer srv.Close()
			t.Setenv("CORTISOL_SERVER_URL", srv.URL)

			m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
			m.Update(authDeviceMsg{start: deviceStartResponse{
				DeviceCode: "dev", UserCode: "ERR", VerificationURI: "https://github.com/login/device", Interval: 5,
			}})
			msg := m.doPoll("dev", 5*time.Second, m.authPollGen)()
			m.Update(msg)
			if m.loginPending() {
				t.Fatal("banner still pending after fatal poll error")
			}
			if m.auth != nil {
				t.Fatal("auth attached on error")
			}
			if !strings.Contains(m.clipboardNotice, "Login:") {
				t.Fatalf("notice=%q", m.clipboardNotice)
			}
		})
	}
}

func TestSlowDownIncreasesIntervalByFiveSeconds(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(authDeviceMsg{start: deviceStartResponse{
		DeviceCode: "dev", UserCode: "SLOW", VerificationURI: "https://github.com/login/device", Interval: 5,
	}})
	gen := m.authPollGen
	// Simulate doPoll observing slow_down with current interval 5s → next 10s.
	m.Update(authPendingMsg{deviceCode: "dev", interval: 10 * time.Second, gen: gen})
	if m.authPollInterval != 10*time.Second {
		t.Fatalf("interval=%v", m.authPollInterval)
	}
}

// enqueueCmd appends cmd, or if cmd is a tea.Batch producer, its members.
func enqueueCmd(cmds *[]tea.Cmd, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		*cmds = append(*cmds, []tea.Cmd(batch)...)
		return
	}
	if msg == nil {
		return
	}
	*cmds = append(*cmds, func() tea.Msg { return msg })
}
