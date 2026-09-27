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
type authSessionMsg struct {
	creds sessionCredentials
	err   error
}
type authLogoutMsg struct{ err error }
type sessionCheckTick struct{ token string }
type sessionChecked struct {
	token string
	creds sessionCredentials
	err   error
}
type knowledgeLiveMsg struct {
	author  string
	preview string
}

func (m *model) loadAuthOnStart() tea.Cmd {
	return func() tea.Msg {
		creds, err := loadCredentials()
		if err == nil {
			creds, err = validateSession(m.ctx, creds)
		}
		return authSessionMsg{creds: creds, err: err}
	}
}

func (m *model) beginLogin() tea.Cmd {
	if m.auth != nil {
		m.clipboardNotice = "Signed in as " + displayLogin(m.authCreds) + " · " + m.auth.OrgName
		return nil
	}
	m.returnToLogin = true
	return tea.Quit
}

func (m *model) beginLogout() tea.Cmd {
	token := m.authCreds.Token
	m.stopEvaluation()
	var interrupt tea.Cmd
	if m.busy && m.turnID != "" {
		interrupt = m.call("turn/interrupt", map[string]any{"threadId": m.threadID, "turnId": m.turnID})
	}
	m.loggingOut = true
	m.quiz = nil
	return tea.Batch(interrupt, func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		if err := logoutSession(ctx, token); err != nil {
			return authLogoutMsg{err: err}
		}
		// The revoked token cannot restore a session, even if local cleanup fails.
		_ = clearCredentials()
		return authLogoutMsg{}
	})
}

func (m *model) setAuthIdentity(creds sessionCredentials) {
	m.authCreds = creds
	m.auth = &authIdentity{Token: creds.Token, Name: creds.Name, GitHubLogin: creds.GitHubLogin, OrgName: creds.OrgName, OrgID: creds.OrgID}
	m.clipboardNotice = fmt.Sprintf("Logged in as %s · %s", displayLogin(creds), creds.OrgName)
}

func displayLogin(creds sessionCredentials) string {
	if creds.GitHubLogin != "" {
		return creds.GitHubLogin
	}
	return creds.Name
}

func (m *model) sessionCheckLater() tea.Cmd {
	token := m.authCreds.Token
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return sessionCheckTick{token} })
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
	case authSessionMsg:
		if v.err == nil && v.creds.Token != "" {
			m.setAuthIdentity(v.creds)
			return m.watchKnowledgeEvents(v.creds.Token), true
		}
		return nil, true
	case sessionCheckTick:
		if m.auth == nil || v.token != m.auth.Token {
			return nil, true
		}
		creds := m.authCreds
		return func() tea.Msg {
			checked, err := validateSession(m.ctx, creds)
			return sessionChecked{token: creds.Token, creds: checked, err: err}
		}, true
	case sessionChecked:
		if m.auth == nil || v.token != m.auth.Token {
			return nil, true
		}
		if v.err != nil {
			m.stopEvaluation()
			m.auth = nil
			m.returnToLogin = true
			if m.sseCancel != nil {
				m.sseCancel()
			}
			return tea.Quit, true
		}
		m.authCreds = v.creds
		return m.sessionCheckLater(), true
	case authLogoutMsg:
		m.loggingOut = false
		if v.err != nil {
			m.setAuthIdentity(m.authCreds)
			m.clipboardNotice = "Logout failed: " + v.err.Error()
			return nil, true
		}
		if m.sseCancel != nil {
			m.sseCancel()
			m.sseCancel = nil
		}
		m.auth = nil
		m.returnToLogin = true
		m.knowledgeLiveCh = nil
		return tea.Quit, true
	case knowledgeLiveMsg:
		if m.auth == nil {
			return nil, true
		}
		m.clipboardNotice = fmt.Sprintf("New knowledge from %s: %s", v.author, v.preview)
		if m.knowledgeLiveCh != nil {
			return waitKnowledgeLive(m.knowledgeLiveCh), true
		}
		return nil, true
	}
	return nil, false
}
