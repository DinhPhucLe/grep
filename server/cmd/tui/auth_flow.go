package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type authIdentity struct {
	Token       string
	Name        string
	GitHubLogin string
	OrgName     string
	OrgID       string
}

type authDeviceMsg struct {
	start deviceStartResponse
	err   error
}

type authPollTickMsg struct {
	deviceCode string
	interval  time.Duration
	gen       int
}

type authPendingMsg struct {
	deviceCode string
	interval  time.Duration
	gen       int
}

type authSessionMsg struct {
	creds sessionCredentials
	err   error
	code  string
	gen   int
}

type authLogoutMsg struct {
	err error
}

type knowledgeLiveMsg struct {
	author  string
	preview string
}

func (m *model) loadAuthOnStart() tea.Cmd {
	return func() tea.Msg {
		creds, err := loadCredentials()
		if err != nil {
			return authSessionMsg{err: err}
		}
		return authSessionMsg{creds: creds}
	}
}

func (m *model) beginLogin() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		start, err := startGitHubDevice(ctx)
		return authDeviceMsg{start: start, err: err}
	}
}

// pollIntervalFromServer uses the device-flow interval GitHub/API returned.
// Floor at 1s only to avoid a zero/negative tick; never invent a faster cadence.
func pollIntervalFromServer(seconds int) time.Duration {
	if seconds < 1 {
		seconds = 5
	}
	return time.Duration(seconds) * time.Second
}

func (m *model) pollLogin(deviceCode string, interval time.Duration, gen int) tea.Cmd {
	if interval < time.Second {
		interval = time.Second
	}
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return authPollTickMsg{deviceCode: deviceCode, interval: interval, gen: gen}
	})
}

func (m *model) doPoll(deviceCode string, interval time.Duration, gen int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		session, code, err := pollGitHubDevice(ctx, deviceCode)
		if err != nil {
			return authSessionMsg{err: err, code: code, gen: gen}
		}
		if code == "authorization_pending" || code == "slow_down" {
			next := interval
			// RFC 8628: on slow_down, increase the interval by 5 seconds.
			if code == "slow_down" {
				next = interval + 5*time.Second
			}
			return authPendingMsg{deviceCode: deviceCode, interval: next, gen: gen}
		}
		if code != "" {
			return authSessionMsg{err: fmt.Errorf("login poll: %s", code), code: code, gen: gen}
		}
		if strings.TrimSpace(session.Token) == "" {
			return authSessionMsg{err: fmt.Errorf("empty session token from server"), gen: gen}
		}
		creds := sessionCredentials{
			Token:       session.Token,
			ExpiresAt:   parseSessionExpiry(session.ExpiresAt),
			UserID:      session.User.ID,
			Name:        session.User.Name,
			GitHubLogin: session.User.GitHubLogin,
			OrgID:       session.Organization.ID,
			OrgName:     session.Organization.Name,
			ServerURL:   cortisolServerURL(),
		}
		if err := saveCredentials(creds); err != nil {
			return authSessionMsg{err: err, gen: gen}
		}
		return authSessionMsg{creds: creds, gen: gen}
	}
}

func (m *model) beginLogout() tea.Cmd {
	token := ""
	if m.auth != nil {
		token = m.auth.Token
	}
	return func() tea.Msg {
		if token != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = logoutSession(ctx, token)
		}
		err := clearCredentials()
		return authLogoutMsg{err: err}
	}
}

func (m *model) attachAuth(creds sessionCredentials) tea.Cmd {
	m.auth = &authIdentity{
		Token:       creds.Token,
		Name:        creds.Name,
		GitHubLogin: creds.GitHubLogin,
		OrgName:     creds.OrgName,
		OrgID:       creds.OrgID,
	}
	m.clearLoginPrompt()
	m.clipboardNotice = fmt.Sprintf("Logged in as %s · %s", displayLogin(creds), creds.OrgName)
	return m.watchKnowledgeEvents(creds.Token)
}

func (m *model) clearLoginPrompt() {
	m.authDeviceCode = ""
	m.authUserCode = ""
	m.authVerificationURI = ""
	m.authBrowserOpened = false
	m.authPollInterval = 0
	m.authPollGen++
}

func (m *model) loginPending() bool {
	return strings.TrimSpace(m.authDeviceCode) != "" && strings.TrimSpace(m.authUserCode) != ""
}

func (m *model) loginPrompt() string {
	if !m.loginPending() {
		return ""
	}
	if m.authBrowserOpened {
		return fmt.Sprintf("Waiting for GitHub… code %s · click footer to retry · Esc cancel", m.authUserCode)
	}
	return fmt.Sprintf("GitHub login: open %s · enter %s · click footer to reopen · Esc cancel",
		m.authVerificationURI, m.authUserCode)
}

type authLoginAssistMsg struct {
	copied bool
	opened bool
	err    error
}

func openBrowser(rawURL string) error {
	u := strings.TrimSpace(rawURL)
	if u == "" {
		return fmt.Errorf("empty url")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	return cmd.Start()
}

func (m *model) copyLoginCodeAndOpenBrowser() tea.Cmd {
	code := strings.TrimSpace(m.authUserCode)
	uri := strings.TrimSpace(m.authVerificationURI)
	cb := m.clipboard
	return func() tea.Msg {
		out := authLoginAssistMsg{}
		if code != "" && cb != nil {
			if err := cb.WriteAll(code); err != nil {
				out.err = err
			} else {
				out.copied = true
			}
		}
		if uri != "" {
			if err := openBrowser(uri); err != nil {
				if out.err == nil {
					out.err = err
				}
			} else {
				out.opened = true
			}
		}
		return out
	}
}

func displayLogin(creds sessionCredentials) string {
	if creds.GitHubLogin != "" {
		return creds.GitHubLogin
	}
	return creds.Name
}

func (m *model) watchKnowledgeEvents(token string) tea.Cmd {
	if m.sseCancel != nil {
		m.sseCancel()
		m.sseCancel = nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.sseCancel = cancel
	ch := make(chan knowledgeLiveMsg, 8)
	m.knowledgeLiveCh = ch
	go func() {
		defer close(ch)
		_ = listenKnowledgeEvents(ctx, token, func(evt knowledgeCreatedEvent) {
			author := "teammate"
			if len(evt.Item.Authors) > 0 {
				if evt.Item.Authors[0].Name != "" {
					author = evt.Item.Authors[0].Name
				} else {
					author = evt.Item.Authors[0].UserID
				}
			}
			preview := strings.TrimSpace(evt.Item.Content)
			if len([]rune(preview)) > 80 {
				preview = string([]rune(preview)[:80]) + "…"
			}
			select {
			case ch <- knowledgeLiveMsg{author: author, preview: preview}:
			default:
			}
		})
	}()
	return waitKnowledgeLive(ch)
}

func waitKnowledgeLive(ch <-chan knowledgeLiveMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (m *model) handleAuthMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch v := msg.(type) {
	case authDeviceMsg:
		if v.err != nil {
			m.clearLoginPrompt()
			m.clipboardNotice = "Login failed: " + v.err.Error()
			return nil, true
		}
		m.authPollGen++
		gen := m.authPollGen
		m.authDeviceCode = v.start.DeviceCode
		m.authUserCode = v.start.UserCode
		m.authVerificationURI = v.start.VerificationURI
		m.authBrowserOpened = false
		m.authPollInterval = pollIntervalFromServer(v.start.Interval)
		m.clipboardNotice = m.loginPrompt()
		// One poll chain only. Interval comes from the device-start response.
		return tea.Batch(
			m.doPoll(v.start.DeviceCode, m.authPollInterval, gen),
			m.copyLoginCodeAndOpenBrowser(),
		), true
	case authLoginAssistMsg:
		if !m.loginPending() {
			return nil, true
		}
		m.authBrowserOpened = true
		switch {
		case v.copied:
			m.clipboardNotice = fmt.Sprintf("Waiting for GitHub… code %s · Esc cancel", m.authUserCode)
		default:
			m.clipboardNotice = m.loginPrompt()
		}
		// One immediate poll after opening the browser; cadence stays server interval.
		return m.doPoll(m.authDeviceCode, m.authPollInterval, m.authPollGen), true
	case authPendingMsg:
		if v.gen != m.authPollGen || v.deviceCode == "" || v.deviceCode != m.authDeviceCode {
			return nil, true
		}
		m.authPollInterval = v.interval
		if m.authBrowserOpened {
			m.clipboardNotice = fmt.Sprintf("Waiting for GitHub… code %s · Esc cancel", m.authUserCode)
		}
		return m.pollLogin(v.deviceCode, v.interval, v.gen), true
	case authPollTickMsg:
		if v.gen != m.authPollGen || v.deviceCode == "" || v.deviceCode != m.authDeviceCode {
			return nil, true
		}
		return m.doPoll(v.deviceCode, v.interval, v.gen), true
	case authSessionMsg:
		// gen!=0 is a device-poll generation. Stale gens (Esc cancel, superseded
		// /login, or a poll that finished after clearLoginPrompt) must never
		// attach or clear state — including after the banner is already gone.
		if v.gen != 0 && v.gen != m.authPollGen {
			return nil, true
		}
		// gen==0 is startup disk hydrate / loadAuthOnStart. Its failures must
		// not abort an in-flight device flow (race: /login before load finishes).
		if v.gen == 0 && m.loginPending() && v.err != nil {
			return nil, true
		}
		if v.err != nil {
			if v.code == "authorization_pending" || v.code == "slow_down" {
				if m.loginPending() {
					interval := m.authPollInterval
					if interval < time.Second {
						interval = pollIntervalFromServer(5)
					}
					if v.code == "slow_down" {
						interval += 5 * time.Second
					}
					m.authPollInterval = interval
					return m.pollLogin(m.authDeviceCode, interval, m.authPollGen), true
				}
				return nil, true
			}
			errText := v.err.Error()
			if !m.loginPending() && (strings.Contains(errText, "no such file") ||
				strings.Contains(errText, "session expired") ||
				strings.Contains(errText, "empty token")) {
				return nil, true
			}
			m.clearLoginPrompt()
			m.clipboardNotice = "Login: " + errText
			return nil, true
		}
		if strings.TrimSpace(v.creds.Token) == "" {
			m.clearLoginPrompt()
			m.clipboardNotice = "Login failed: empty session token"
			return nil, true
		}
		m.clearLoginPrompt()
		return m.attachAuth(v.creds), true
	case authLogoutMsg:
		if m.sseCancel != nil {
			m.sseCancel()
			m.sseCancel = nil
		}
		m.auth = nil
		m.clearLoginPrompt()
		m.knowledgeLiveCh = nil
		if v.err != nil {
			m.clipboardNotice = "Logout error: " + v.err.Error()
		} else {
			m.clipboardNotice = "Logged out"
		}
		return nil, true
	case knowledgeLiveMsg:
		if !m.loginPending() {
			m.clipboardNotice = fmt.Sprintf("New knowledge from %s: %s", v.author, v.preview)
		}
		if m.knowledgeLiveCh != nil {
			return waitKnowledgeLive(m.knowledgeLiveCh), true
		}
		return nil, true
	}
	return nil, false
}
