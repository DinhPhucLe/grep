package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"time"
)

type uiOptions struct{ NoIcons, ReducedMotion, NoColor bool }
type conversationItem struct {
	key, kind, raw, status, command, output string
	done, expanded                          bool
	exitCode                                *int
	cache                                   string
	cacheRaw                                string
	cacheWidth                              int
	stream                                  strings.Builder
}
type model struct {
	client                                    *appServer
	ctx                                       context.Context
	opts                                      uiOptions
	workspace, threadID, turnID, status       string
	connected, busy, dirty, follow, newOutput bool
	width, height, focus, frame               int
	draft                                     textarea.Model
	viewport                                  viewport.Model
	items                                     []*conversationItem
	byID                                      map[string]*conversationItem
	requests                                  []*pendingRequest
	seenRequests                              map[string]bool
	connectionLost                            bool
	revealFocus                               bool
}
type frameMsg time.Time
type readyMsg struct {
	thread string
	err    error
}
type callDoneMsg struct {
	method string
	err    error
}

func newModel(c *appServer, cwd string, o uiOptions) *model {
	d := textarea.New()
	d.ShowLineNumbers = false
	d.Prompt = ""
	d.CharLimit = 0
	d.Placeholder = "Ask Codex..."
	d.Focus()
	d.SetHeight(1)
	if o.ReducedMotion {
		d.Cursor.SetMode(cursor.CursorStatic)
	}
	m := &model{client: c, ctx: context.Background(), opts: o, workspace: cwd, status: "Connecting", width: 80, height: 24, draft: d, viewport: viewport.New(80, 16), follow: true, focus: -1, byID: map[string]*conversationItem{}, seenRequests: map[string]bool{}}
	m.resize()
	return m
}
func tick() tea.Cmd {
	return tea.Tick(time.Second/30, func(t time.Time) tea.Msg { return frameMsg(t) })
}
func (m *model) Init() tea.Cmd {
	if m.client == nil {
		return tick()
	}
	return tea.Batch(tick(), m.wait(), func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if err := m.client.Initialize(ctx); err != nil {
			return readyMsg{err: err}
		}
		id, err := m.client.StartThread(ctx, m.workspace)
		if err == nil && m.client.log != nil {
			err = m.client.log.BindThread(id)
		}
		return readyMsg{thread: id, err: err}
	})
}
func (m *model) wait() tea.Cmd { return func() tea.Msg { return m.client.events.next(m.ctx) } }
func (m *model) call(method string, p any) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return callDoneMsg{method: method}
		}
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.client.call(ctx, method, p)
		return callDoneMsg{method: method, err: err}
	}
}
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case rpcBatch:
		var cmds []tea.Cmd
		for _, e := range v {
			_, cmd := m.Update(e)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		cmds = append(cmds, m.wait())
		return m, tea.Batch(cmds...)
	case readyMsg:
		if m.connectionLost {
			return m, nil
		}
		if v.err != nil {
			m.status = "Failed: " + v.err.Error()
			m.connected = false
		} else {
			m.threadID = v.thread
			m.connected = true
			m.status = "Ready"
		}
	case disconnectedMsg:
		m.connectionLost = true
		m.connected = false
		m.busy = false
		m.status = fmt.Sprint("Disconnected: ", v.err)
		m.requests = nil
		m.draft.Focus()
		m.resize()
		m.dirty = true
	case protocolErrorMsg:
		m.status = "Protocol error: " + v.err.Error()
	case callDoneMsg:
		if v.err != nil {
			m.status = "Failed: " + v.err.Error()
			if v.method == "turn/start" {
				m.busy = false
			}
		}
	case wireMessage:
		if len(v.ID) > 0 {
			return m, m.enqueueRequest(v)
		}
		m.reduce(v)
	case tea.WindowSizeMsg:
		m.width = max(1, v.Width)
		m.height = max(1, v.Height)
		m.resize()
		m.dirty = true
		m.refresh()
	case frameMsg:
		m.frame++
		if m.busy && !m.opts.ReducedMotion && m.frame%15 == 0 {
			m.dirty = true
		}
		if m.dirty {
			m.refresh()
		}
		return m, tick()
	case tea.MouseMsg:
		if len(m.requests) > 0 {
			m.requestMouse(v)
			return m, nil
		}
		if v.Action != tea.MouseActionPress {
			return m, nil
		}
		switch v.Button {
		case tea.MouseButtonWheelUp:
			m.scrollHistory(-m.viewport.MouseWheelDelta)
		case tea.MouseButtonWheelDown:
			m.scrollHistory(m.viewport.MouseWheelDelta)
		}
		return m, nil
	case tea.KeyMsg:
		if v.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if len(m.requests) > 0 {
			return m, m.requestKey(v)
		}
		switch v.String() {
		case "pgup":
			m.scrollHistory(-m.viewport.Height)
			return m, nil
		case "pgdown":
			m.scrollHistory(m.viewport.Height)
			return m, nil
		case "up", "down":
			if m.focus != -1 {
				step := 1
				if v.String() == "up" {
					step = -1
				}
				m.scrollHistory(step)
				return m, nil
			}
		case "ctrl+home":
			if m.dirty {
				m.refresh()
			}
			m.viewport.GotoTop()
			m.syncFollowing()
			return m, nil
		case "ctrl+end":
			m.latest()
			return m, nil
		case "tab", "shift+tab":
			controls := []int{-1, -2} // composer, history, then activity cards
			for n, i := range m.items {
				if i.kind == "fileChange" {
					controls = append(controls, n)
				}
			}
			pos := 0
			for n, f := range controls {
				if f == m.focus {
					pos = n
				}
			}
			step := 1
			if v.String() == "shift+tab" {
				step = -1
			}
			m.focus = controls[(pos+step+len(controls))%len(controls)]
			if m.focus >= 0 {
				m.revealFocus = true
				m.follow = false
			}
			if m.focus == -1 {
				m.draft.Focus()
			} else {
				m.draft.Blur()
			}
			m.dirty = true
			m.refresh()
			return m, nil
		case "end":
			if m.focus != -1 {
				m.latest()
				return m, nil
			}
		case "esc":
			if m.busy && m.turnID != "" {
				m.status = "Interrupt requested"
				return m, m.call("turn/interrupt", map[string]any{"threadId": m.threadID, "turnId": m.turnID})
			}
		case "alt+enter", "shift+enter":
			if m.focus == -1 {
				m.draft.InsertString("\n")
				m.resize()
			}
			return m, nil
		case "enter":
			if m.focus == -2 {
				return m, nil
			}
			if m.focus >= 0 {
				m.items[m.focus].expanded = !m.items[m.focus].expanded
				m.dirty = true
				m.refresh()
				return m, nil
			}
			text := m.draft.Value()
			if strings.TrimSpace(text) == "/quit" || strings.TrimSpace(text) == "/exit" {
				return m, tea.Quit
			}
			if m.busy || !m.connected || strings.TrimSpace(text) == "" {
				return m, nil
			}
			m.items = append(m.items, &conversationItem{kind: "userMessage", raw: text, done: true})
			m.draft.Reset()
			m.busy = true
			m.turnID = ""
			m.status = "Waiting for response"
			m.follow = true
			m.dirty = true
			m.resize()
			m.refresh()
			return m, m.call("turn/start", map[string]any{"threadId": m.threadID, "input": []map[string]any{{"type": "text", "text": text}}})
		}
		if m.focus == -1 {
			var cmd tea.Cmd
			m.draft, cmd = m.draft.Update(msg)
			m.resize()
			return m, cmd
		}
	default:
		if len(m.requests) == 0 && m.focus == -1 {
			var cmd tea.Cmd
			m.draft, cmd = m.draft.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

// Scroll the conversation without changing composer focus or its draft.
func (m *model) scrollHistory(lines int) {
	if m.dirty {
		m.refresh()
	}
	if lines < 0 {
		m.viewport.ScrollUp(-lines)
	} else {
		m.viewport.ScrollDown(lines)
	}
	m.syncFollowing()
}
func (m *model) syncFollowing() {
	m.follow = m.viewport.AtBottom()
	if m.follow {
		m.newOutput = false
	}
}
func (m *model) latest() {
	if m.dirty {
		m.refresh()
	}
	m.viewport.GotoBottom()
	m.syncFollowing()
}

func (m *model) item(turn, id, kind string) *conversationItem {
	key := m.threadID + "/" + turn + "/" + id
	if i := m.byID[key]; i != nil {
		return i
	}
	i := &conversationItem{key: key, kind: kind}
	m.byID[key] = i
	m.items = append(m.items, i)
	return i
}
func (m *model) reduce(msg wireMessage) {
	if msg.Method == "serverRequest/resolved" {
		var p struct {
			ThreadID  string
			RequestID json.RawMessage
		}
		if json.Unmarshal(msg.Params, &p) != nil || p.ThreadID != m.threadID {
			return
		}
		for n, r := range m.requests {
			if string(r.message.ID) == string(p.RequestID) {
				m.requests = append(m.requests[:n], m.requests[n+1:]...)
				break
			}
		}
		if len(m.requests) == 0 && m.focus == -1 {
			m.draft.Focus()
		}
		m.resize()
		return
	}
	var p struct {
		ThreadID, TurnID, ItemID, Delta string
		Item                            struct {
			ID, Type, Text, Status, Command string
			AggregatedOutput                *string
			ExitCode                        *int
			Changes                         json.RawMessage
		}
		Turn struct {
			ID, Status string
			Error      *struct{ Message string }
		}
		Error     struct{ Message string }
		WillRetry bool
	}
	if json.Unmarshal(msg.Params, &p) != nil || p.ThreadID != m.threadID {
		return
	}
	switch msg.Method {
	case "turn/started":
		m.turnID = p.Turn.ID
		m.busy = true
		m.status = "Waiting for response"
	case "item/agentMessage/delta":
		i := m.item(p.TurnID, p.ItemID, "agentMessage")
		if !i.done {
			if i.stream.Len() != len(i.raw) {
				i.stream.Reset()
				i.stream.WriteString(i.raw)
			}
			i.stream.WriteString(p.Delta)
			i.raw = i.stream.String()
			m.status = "Responding"
		}
	case "item/commandExecution/outputDelta", "item/fileChange/outputDelta":
		i := m.item(p.TurnID, p.ItemID, strings.Split(msg.Method, "/")[1])
		i.output += p.Delta
	case "item/started", "item/completed":
		if p.Item.Type != "agentMessage" && p.Item.Type != "commandExecution" && p.Item.Type != "fileChange" {
			return
		}
		i := m.item(p.TurnID, p.Item.ID, p.Item.Type)
		i.kind = p.Item.Type
		i.done = msg.Method == "item/completed"
		i.cache = ""
		i.status = p.Item.Status
		i.command = p.Item.Command
		i.exitCode = p.Item.ExitCode
		if i.kind == "agentMessage" {
			i.raw = p.Item.Text
		} else if i.kind == "fileChange" {
			i.raw = string(p.Item.Changes)
		}
		if p.Item.AggregatedOutput != nil {
			i.output = *p.Item.AggregatedOutput
		}
		if !i.done && i.kind == "commandExecution" {
			m.status = "Codex is working"
		}
		if !i.done && i.kind == "fileChange" {
			m.status = "File change proposed"
		}
	case "turn/completed":
		if m.turnID != "" && p.Turn.ID != m.turnID {
			return
		}
		m.busy = false
		m.status = "Ready"
		switch p.Turn.Status {
		case "failed":
			m.status = "Failed"
			if p.Turn.Error != nil {
				m.status += ": " + p.Turn.Error.Message
			}
		case "interrupted":
			m.status = "Interrupted"
		case "completed":
		default:
			m.status = "Turn ended: " + p.Turn.Status
		}
		for _, i := range m.items {
			if strings.HasPrefix(i.key, m.threadID+"/"+p.Turn.ID+"/") {
				i.done = true
			}
		}
	case "error":
		m.status = "Error: " + p.Error.Message
	default:
		return
	}
	m.dirty = true
	if !m.follow {
		m.newOutput = true
	}
}
