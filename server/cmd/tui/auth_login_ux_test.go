package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type memClipboard struct{ text string }

func (c *memClipboard) ReadAll() (string, error)  { return c.text, nil }
func (c *memClipboard) WriteAll(s string) error   { c.text = s; return nil }

func TestLoginPromptSurvivesMouseClick(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	cb := &memClipboard{}
	m.clipboard = cb

	m.Update(authDeviceMsg{start: deviceStartResponse{
		DeviceCode:      "dev",
		UserCode:        "ABCD-EFGH",
		VerificationURI: "https://github.com/login/device",
		Interval:        5,
	}})
	if !m.loginPending() {
		t.Fatal("expected pending login")
	}
	prompt := m.loginPrompt()
	if !strings.Contains(prompt, "ABCD-EFGH") || !strings.Contains(prompt, "https://github.com/login/device") {
		t.Fatalf("prompt=%q", prompt)
	}
	if !strings.Contains(m.View(), "ABCD-EFGH") {
		t.Fatalf("view missing code: %s", m.View())
	}

	// Click in conversation area used to wipe clipboardNotice and lose the code.
	m.Update(tea.MouseMsg{
		X: 2, Y: 2,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if !m.loginPending() {
		t.Fatal("login cleared by click")
	}
	if !strings.Contains(m.View(), "ABCD-EFGH") {
		t.Fatalf("code disappeared after click: %s", m.View())
	}

	// Any key also used to clear clipboardNotice; durable prompt must remain.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if !strings.Contains(m.View(), "ABCD-EFGH") {
		t.Fatalf("code disappeared after key: %s", m.View())
	}
}

func TestCopyLoginCodeWritesClipboard(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	cb := &memClipboard{}
	m.clipboard = cb
	m.authDeviceCode = "dev"
	m.authUserCode = "WXYZ-1234"
	m.authVerificationURI = "https://github.com/login/device"

	msg := m.copyLoginCodeAndOpenBrowser()()
	assist, ok := msg.(authLoginAssistMsg)
	if !ok {
		t.Fatalf("msg=%T", msg)
	}
	if !assist.copied || cb.text != "WXYZ-1234" {
		t.Fatalf("copied=%v clipboard=%q err=%v", assist.copied, cb.text, assist.err)
	}
}

func TestLoginPromptClearsAfterSession(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m.clipboard = &memClipboard{}

	m.Update(authDeviceMsg{start: deviceStartResponse{
		DeviceCode:      "dev",
		UserCode:        "ABCD-EFGH",
		VerificationURI: "https://github.com/login/device",
		Interval:        5,
	}})
	if !strings.Contains(m.View(), "ABCD-EFGH") {
		t.Fatal("expected login prompt")
	}

	m.Update(authSessionMsg{creds: sessionCredentials{
		Token:       "tok",
		GitHubLogin: "alice",
		OrgName:     "Org",
	}})
	if m.loginPending() {
		t.Fatal("login should not be pending after session")
	}
	view := m.View()
	if strings.Contains(view, "ABCD-EFGH") || strings.Contains(view, "github.com/login/device") {
		t.Fatalf("login prompt still visible after auth: %s", view)
	}
	if !strings.Contains(view, "Logged in as alice") {
		t.Fatalf("expected logged-in notice: %s", view)
	}
}

func TestLoginAssistKeepsServerPollInterval(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.clipboard = &memClipboard{}

	m.Update(authDeviceMsg{start: deviceStartResponse{
		DeviceCode:      "dev",
		UserCode:        "FAST-CODE",
		VerificationURI: "https://github.com/login/device",
		Interval:        5,
	}})
	if m.authPollInterval != 5*time.Second {
		t.Fatalf("initial interval=%v want server interval", m.authPollInterval)
	}

	m.Update(authLoginAssistMsg{copied: true, opened: true})
	if !m.authBrowserOpened {
		t.Fatal("expected browser opened flag")
	}
	if m.authPollInterval != 5*time.Second {
		t.Fatalf("assist must not hardcode a faster interval, got %v", m.authPollInterval)
	}

	gen := m.authPollGen
	m.Update(authPendingMsg{deviceCode: "dev", interval: 5 * time.Second, gen: gen})
	if m.authPollInterval != 5*time.Second {
		t.Fatalf("pending interval=%v", m.authPollInterval)
	}

	// slow_down adds 5s per RFC 8628
	m.Update(authPendingMsg{deviceCode: "dev", interval: 10 * time.Second, gen: gen})
	if m.authPollInterval != 10*time.Second {
		t.Fatalf("slow_down interval=%v", m.authPollInterval)
	}
}

func TestStalePollGenIgnoredDuringLogin(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(authDeviceMsg{start: deviceStartResponse{
		DeviceCode: "dev", UserCode: "CODE", VerificationURI: "https://github.com/login/device", Interval: 5,
	}})
	gen := m.authPollGen
	m.Update(authPendingMsg{deviceCode: "dev", interval: time.Second, gen: gen - 1})
	if m.authPollGen != gen {
		t.Fatal("stale pending mutated poll gen")
	}
}

func TestLoginEscCancels(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.authDeviceCode = "dev"
	m.authUserCode = "CODE-1234"
	m.authVerificationURI = "https://github.com/login/device"
	m.focus = -1
	m.draft.Focus()

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.loginPending() {
		t.Fatal("expected cancel")
	}
	if !strings.Contains(m.clipboardNotice, "canceled") {
		t.Fatalf("notice=%q", m.clipboardNotice)
	}
}

func TestPollIntervalFromServer(t *testing.T) {
	if got := pollIntervalFromServer(5); got != 5*time.Second {
		t.Fatalf("got %v", got)
	}
	if got := pollIntervalFromServer(0); got != 5*time.Second {
		t.Fatalf("zero should fall back to 5s, got %v", got)
	}
}
