package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type terminalHandoff struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}
type gateChecked struct {
	creds sessionCredentials
	err   error
}
type gateStarted struct {
	handoff terminalHandoff
	link    string
	err     error
}
type gatePoll struct{ id string }
type gatePolled struct {
	id      string
	session sessionAPIResponse
	pending bool
	err     error
}
type gateBrowserOpened struct{ err error }

type loginGate struct {
	ctx                context.Context
	width, height      int
	checking, starting bool
	handoff            terminalHandoff
	link, notice       string
	creds              sessionCredentials
}

func (g *loginGate) Init() tea.Cmd {
	return func() tea.Msg {
		creds, err := loadCredentials()
		if err == nil {
			creds, err = validateSession(g.ctx, creds)
		}
		return gateChecked{creds, err}
	}
}

func dashboardLink(id string) (string, error) {
	base := strings.TrimSpace(os.Getenv("CORTISOL_DASHBOARD_URL"))
	if base == "" {
		base = "http://localhost:5173"
	}
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return "", fmt.Errorf("CORTISOL_DASHBOARD_URL must be an HTTP or HTTPS URL")
	}
	u.Fragment = "terminal=" + id
	return u.String(), nil
}

func openDashboard(link string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.CommandContext(ctx, "open", link)
		case "windows":
			cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", link)
		default:
			cmd = exec.CommandContext(ctx, "xdg-open", link)
		}
		return gateBrowserOpened{cmd.Run()}
	}
}

func (g *loginGate) start() tea.Cmd {
	g.starting = true
	g.notice = "Opening dashboard sign-in…"
	return func() tea.Msg {
		if _, err := dashboardLink(""); err != nil {
			return gateStarted{err: err}
		}
		var h terminalHandoff
		err := postJSON(g.ctx, cortisolServerURL()+"/api/v1/auth/cli/start", nil, "", &h)
		if err != nil {
			return gateStarted{err: err}
		}
		if len(h.ID) != 64 || h.Secret == "" {
			return gateStarted{err: fmt.Errorf("server returned an incomplete sign-in request")}
		}
		link, err := dashboardLink(h.ID)
		return gateStarted{handoff: h, link: link, err: err}
	}
}

func gatePollLater(id string) tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return gatePoll{id} })
}

func (g *loginGate) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		g.width, g.height = v.Width, v.Height
	case gateChecked:
		g.checking = false
		if v.err == nil && v.creds.Token != "" {
			g.creds = v.creds
			return g, tea.Quit
		}
		g.notice = "Sign in on the dashboard to use Cortisol."
		if v.err != nil && !os.IsNotExist(v.err) {
			g.notice = v.err.Error()
		}
	case tea.KeyMsg:
		if v.Paste {
			return g, nil
		}
		switch v.String() {
		case "q", "ctrl+c", "esc":
			return g, tea.Quit
		case "enter", "o":
			if g.checking || g.starting {
				return g, nil
			}
			if g.link != "" {
				return g, openDashboard(g.link)
			}
			return g, g.start()
		case "r":
			if !g.starting && !g.checking {
				g.handoff = terminalHandoff{}
				g.link = ""
				return g, g.start()
			}
		}
	case gateStarted:
		g.starting = false
		if v.err != nil {
			g.notice = "Could not start sign-in: " + v.err.Error()
			return g, nil
		}
		g.handoff, g.link = v.handoff, v.link
		g.notice = "Sign in on the dashboard, then choose Connect terminal."
		return g, tea.Batch(openDashboard(g.link), gatePollLater(g.handoff.ID))
	case gateBrowserOpened:
		if v.err != nil {
			g.notice = "Could not open a browser. Open the link below manually."
		}
	case gatePoll:
		if v.id != g.handoff.ID || v.id == "" {
			return g, nil
		}
		h := g.handoff
		return g, func() tea.Msg {
			var session sessionAPIResponse
			code, err := postJSONStatus(g.ctx, cortisolServerURL()+"/api/v1/auth/cli/poll", map[string]string{"id": h.ID, "secret": h.Secret}, "", &session)
			return gatePolled{id: h.ID, session: session, pending: code == "authorization_pending", err: err}
		}
	case gatePolled:
		if v.id != g.handoff.ID {
			return g, nil
		}
		if v.err != nil {
			g.notice = "Sign-in failed: " + v.err.Error()
			g.handoff = terminalHandoff{}
			g.link = ""
			return g, nil
		}
		if v.pending {
			return g, gatePollLater(v.id)
		}
		if v.session.Token == "" || v.session.User.ID == "" || v.session.Organization.ID == "" {
			g.notice = "Incomplete sign-in response. Press R to retry."
			return g, nil
		}
		creds := credentialsFromSession(v.session)
		if err := saveCredentials(creds); err != nil {
			g.notice = "Could not save sign-in: " + err.Error()
			return g, nil
		}
		g.creds = creds
		return g, tea.Quit
	}
	return g, nil
}

func (g *loginGate) View() string {
	text := "CORTISOL\n\nSign in to continue\n\n"
	if g.checking {
		text += "Checking your saved session…"
	} else {
		text += safeText(g.notice)
	}
	if g.link != "" {
		text += "\n\nConnection code: " + strings.ToUpper(g.handoff.ID[:8]) + "\n\n" + g.link
	}
	text += "\n\nEnter  Open dashboard    R  Retry    Q  Quit"
	return fitScreen(ansi.Hardwrap(text, max(1, g.width), true), max(1, g.width), max(1, g.height))
}
