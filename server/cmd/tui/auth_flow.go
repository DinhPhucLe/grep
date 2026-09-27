package main

import (
	"context"
	"fmt"
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
}

type authPendingMsg struct {
	deviceCode string
	interval  time.Duration
}

type authSessionMsg struct {
	creds sessionCredentials
	err   error
	code  string
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

func (m *model) pollLogin(deviceCode string, interval time.Duration) tea.Cmd {
	if interval < time.Second {
		interval = 5 * time.Second
	}
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return authPollTickMsg{deviceCode: deviceCode, interval: interval}
	})
}

func (m *model) doPoll(deviceCode string, interval time.Duration) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		session, code, err := pollGitHubDevice(ctx, deviceCode)
		if err != nil {
			return authSessionMsg{err: err, code: code}
		}
		if code == "authorization_pending" || code == "slow_down" {
			next := interval
			if code == "slow_down" {
				next = interval + time.Second
			}
			return authPendingMsg{deviceCode: deviceCode, interval: next}
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
			return authSessionMsg{err: err}
		}
		return authSessionMsg{creds: creds}
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
	m.authDeviceCode = ""
	m.clipboardNotice = fmt.Sprintf("Logged in as %s · %s", displayLogin(creds), creds.OrgName)
	return m.watchKnowledgeEvents(creds.Token)
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
			m.clipboardNotice = "Login failed: " + v.err.Error()
			return nil, true
		}
		m.authDeviceCode = v.start.DeviceCode
		interval := time.Duration(v.start.Interval) * time.Second
		m.clipboardNotice = fmt.Sprintf("GitHub login: open %s and enter %s", v.start.VerificationURI, v.start.UserCode)
		return tea.Batch(m.pollLogin(v.start.DeviceCode, interval), m.doPoll(v.start.DeviceCode, interval)), true
	case authPendingMsg:
		if v.deviceCode == "" || v.deviceCode != m.authDeviceCode {
			return nil, true
		}
		return m.pollLogin(v.deviceCode, v.interval), true
	case authPollTickMsg:
		if v.deviceCode == "" || v.deviceCode != m.authDeviceCode {
			return nil, true
		}
		return m.doPoll(v.deviceCode, v.interval), true
	case authSessionMsg:
		if v.err != nil {
			if v.code == "authorization_pending" || v.code == "slow_down" {
				return nil, true
			}
			errText := v.err.Error()
			if strings.Contains(errText, "no such file") || strings.Contains(errText, "session expired") || strings.Contains(errText, "empty token") {
				return nil, true
			}
			m.clipboardNotice = "Login: " + errText
			return nil, true
		}
		if v.creds.Token == "" {
			return nil, true
		}
		return m.attachAuth(v.creds), true
	case authLogoutMsg:
		if m.sseCancel != nil {
			m.sseCancel()
			m.sseCancel = nil
		}
		m.auth = nil
		m.authDeviceCode = ""
		m.knowledgeLiveCh = nil
		if v.err != nil {
			m.clipboardNotice = "Logout error: " + v.err.Error()
		} else {
			m.clipboardNotice = "Logged out"
		}
		return nil, true
	case knowledgeLiveMsg:
		m.clipboardNotice = fmt.Sprintf("New knowledge from %s: %s", v.author, v.preview)
		if m.knowledgeLiveCh != nil {
			return waitKnowledgeLive(m.knowledgeLiveCh), true
		}
		return nil, true
	}
	return nil, false
}
