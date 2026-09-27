package main

import (
	"context"
	"cortisol-server/internal/evaluation"
	"cortisol-server/internal/quiz"
	"cortisol-server/internal/timing"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type uiOptions struct {
	NoIcons, ReducedMotion, NoColor bool
	EvaluationServer                string
	ApprovalPolicy                  string
	TimingLog                       string
}
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
	timingSink      timing.Sink
	promptTrace     context.Context
	promptStarted   time.Time
	codexStarted    time.Time
	decisionStarted map[string]time.Time

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
	showHelp                                  bool
	helpOffset                                int
	clipboard                                 clipboardAccess
	selection                                 textSelection
	clipboardNotice, quitDraft                string
	quitMode                                  bool
	pendingPastes                             int
	evaluating                                bool
	evaluationSequence                        int
	cancelEvaluation                          context.CancelFunc
	lastEvaluation                            *promptEvaluation
	quiz                                      *quizSession
	grader                                    func(context.Context, string, quiz.Question, []quiz.File, string) (answerGrade, error)
	openSource                                func(context.Context, string, int) error
	quizJump                                  bool
	evaluationContext                         []evaluation.Message
	knowledgeSearching                        bool
	knowledgeSearchGen                        int
	knowledgeHits                             []knowledgeHit
	knowledgeSelected                         int
	knowledgeOffset                           int
	knowledgePingLeft                         int
	knowledgeHighlightLeft                    int
	knowledgeConnected                        map[string]knowledgeHit
	knowledgeConnectedOrder                   []string
	knowledgeLinksOverlay                     bool
	knowledgeLinksOffset                      int
	knowledgePanelOpen                        bool
	knowledgeSearchFailed                     bool
	knowledgeExpanded                         bool
	knowledgePreviewIdx                       int
	knowledgePreviewOffset                    int
	auth                                      *authIdentity
	authCreds                                 sessionCredentials
	authEnforced, returnToLogin               bool
	loggingOut                                bool
	sseCancel                                 context.CancelFunc
	knowledgeLiveCh                           <-chan knowledgeLiveMsg
}
type frameMsg time.Time
type terminalSizeMsg struct{ width, height int }
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
	// Default CursorLine paints a solid black/white bar behind the placeholder.
	d.FocusedStyle.CursorLine = lipgloss.NewStyle()
	d.BlurredStyle.CursorLine = lipgloss.NewStyle()
	d.Focus()
	d.SetHeight(1)
	if o.ReducedMotion {
		d.Cursor.SetMode(cursor.CursorStatic)
	}
	m := &model{client: c, ctx: context.Background(), opts: o, workspace: cwd, status: "Connecting", width: 80, height: 24, draft: d, viewport: viewport.New(80, 16), follow: true, focus: -1, byID: map[string]*conversationItem{}, seenRequests: map[string]bool{}, grader: gradeWithCodex, knowledgePreviewIdx: -1}
	m.clipboard = systemClipboard{}
	m.openSource = openVSCodeSource
	m.resize()
	return m
}
func tick() tea.Cmd {
	return tea.Tick(time.Second/30, func(t time.Time) tea.Msg { return frameMsg(t) })
}
func (m *model) Init() tea.Cmd {
	authCmd := m.loadAuthOnStart()
	if m.authEnforced {
		authCmd = tea.Batch(m.watchKnowledgeEvents(m.authCreds.Token), m.sessionCheckLater())
	}
	if m.client == nil {
		return tea.Batch(tick(), authCmd)
	}
	return tea.Batch(tick(), authCmd, m.wait(), func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if err := m.client.Initialize(ctx); err != nil {
			return readyMsg{err: err}
		}
		id, err := m.client.StartThread(ctx, m.workspace, m.opts.ApprovalPolicy)
		if err == nil && m.client.log != nil {
			err = m.client.log.BindThread(id)
		}
		return readyMsg{thread: id, err: err}
	})
}
func (m *model) wait() tea.Cmd { return func() tea.Msg { return m.client.events.next(m.ctx) } }
func (m *model) call(method string, p any) tea.Cmd {
	if m.authEnforced && (m.auth == nil || m.loggingOut) {
		return nil
	}
	if method == "turn/start" {
		m.codexStarted = time.Now()
	}

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
	_, cmd := m.update(msg)
	m.resize()
	return m, cmd
}
func (m *model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if cmd, handled := m.handleAuthMsg(msg); handled {
		return m, cmd
	}
	if m.authEnforced && m.auth == nil {
		return m, nil
	}
	if m.loggingOut {
		switch msg.(type) {
		case tea.KeyMsg, tea.MouseMsg, clipboardResult:
			return m, nil
		}
	}
	switch v := msg.(type) {
	case quizPreparedMsg, quizGeneratedMsg, quizAnswerSavedMsg, quizAnswerGradedMsg:
		return m, m.handleQuizMessage(v)
	case quizSourceOpenedMsg:
		return m, m.handleQuizSourceOpened(v)
	case evaluationDoneMsg:
		return m, m.finishEvaluation(v)
	case clipboardResult:
		if !v.copied {
			m.pendingPastes = max(0, m.pendingPastes-1)
		}
		if v.err != nil {
			m.clipboardNotice = "Clipboard unavailable: " + v.err.Error()
			return m, nil
		}
		if v.copied {
			m.clipboardNotice = "Copied to clipboard"
			m.selection = textSelection{}
		} else {
			text := strings.ReplaceAll(strings.ReplaceAll(v.text, "\r\n", "\n"), "\r", "\n")
			if v.requestID != "" {
				if m.requestInputActive() {
					r := m.requests[0]
					if string(r.message.ID) == v.requestID && r.question == v.question {
						r.input, _ = r.input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true})
						m.clipboardNotice = "Pasted — Enter to confirm"
					}
				}
				return m, nil
			}
			m.draft.InsertString(text)
			m.clipboardNotice = "Pasted — Enter to send"
		}
		return m, nil
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
		m.stopEvaluation()
		if m.quizActive() && (m.quiz.phase == "running" || m.quiz.phase == "preparing") {
			m.quizFailure(fmt.Errorf("Codex disconnected before completing the turn"))
		}
		m.recordCodexEnd(fmt.Errorf("disconnected"))
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
				m.recordCodexEnd(v.err)
				m.busy = false
				if m.quizActive() {
					m.quizFailure(fmt.Errorf("Codex could not start the turn"))
				}
			}
		}
	case wireMessage:
		if len(v.ID) > 0 {
			return m, m.enqueueRequest(v)
		}
		m.reduce(v)
		if v.Method == "turn/completed" && m.quizActive() && m.quiz.phase == "running" && !m.busy {
			if m.status == "Ready" {
				return m, m.startQuizGeneration()
			}
			var ended struct {
				Turn struct{ Status string }
			}
			if json.Unmarshal(v.Params, &ended) == nil && (ended.Turn.Status == "interrupted" || ended.Turn.Status == "failed") {
				// The same implementation can continue on a later, unscored turn.
				// Keep its original score and baseline until then.
				m.quiz.phase = "awaiting_reply"
				m.quiz.turnID = ""
				m.status += " — review pending if you continue"
				return m, nil
			}
			m.quizFailure(fmt.Errorf("Codex turn did not complete successfully"))
		}
	case tea.WindowSizeMsg:
		m.selection = textSelection{}
		m.width = max(1, v.Width)
		m.height = max(1, v.Height)
		m.resize()
		m.dirty = true
		m.refresh()
	case terminalSizeMsg:
		if v.width > 0 && v.height > 0 && (v.width != m.width || v.height != m.height) {
			return m.update(tea.WindowSizeMsg{Width: v.width, Height: v.height})
		}
	case frameMsg:
		m.frame++
		m.tickKnowledgeEffects()
		if m.busy && !m.opts.ReducedMotion && m.frame%15 == 0 {
			m.dirty = true
		}
		if m.dirty {
			m.refresh()
		}
		if m.client != nil && m.frame%6 == 0 {
			return m, tea.Batch(tick(), pollTerminalSize)
		}
		return m, tick()
	case tea.MouseMsg:
		if m.showHelp {
			if v.Button == tea.MouseButtonWheelUp {
				m.helpOffset = max(0, m.helpOffset-3)
			} else if v.Button == tea.MouseButtonWheelDown {
				m.helpOffset += 3
			}
			return m, nil
		}

		if len(m.requests) > 0 {
			m.requestMouse(v)
			return m, nil
		}
		if m.knowledgeMouse(v) {
			return m, nil
		}
		if m.selectionMouse(v) {
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
		// Paste is content, not a sequence of shortcuts or submit keys.
		// Handle it before focus navigation, approvals, and slash commands.
		if v.Paste && v.Type == tea.KeyRunes {
			text := strings.ReplaceAll(strings.ReplaceAll(string(v.Runes), "\r\n", "\n"), "\r", "\n")
			if len(m.requests) > 0 {
				if m.requestInputActive() {
					r := m.requests[0]
					r.input, _ = r.input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true})
				}
				return m, nil
			}
			if m.showHelp || m.quitMode {
				return m, nil
			}
			m.selection = textSelection{}
			m.focus = -1
			m.draft.Focus()
			m.draft.InsertString(text)
			m.clipboardNotice = "Pasted — Enter to send"
			return m, nil
		}
		if v.String() == "enter" && m.pendingPastes > 0 {
			return m, nil
		}
		if v.String() == "ctrl+c" {
			return m, m.copyText()
		}
		if handled, cmd := m.quitKey(v); handled {
			return m, cmd
		}
		m.clipboardNotice = ""
		if len(m.selection.lines) > 0 {
			m.selection = textSelection{}
			if v.String() == "esc" {
				return m, nil
			}
		}
		if m.showHelp {
			m.helpKey(v)
			return m, nil
		}
		if m.knowledgeKey(v) {
			return m, nil
		}
		if v.String() == "f1" {
			m.showHelp = true
			m.helpOffset = 0
			return m, nil
		}
		if !v.Paste && v.Type == tea.KeyRunes && string(v.Runes) == "/" && (len(m.requests) == 0 && m.focus != -1 || len(m.requests) > 0 && !m.requestInputActive()) {
			m.quitMode = true
			m.quitDraft = "/"
			return m, nil
		}
		if (v.String() == "ctrl+v" || v.String() == "insert" || v.String() == "alt+insert") && (m.focus == -1 && len(m.requests) == 0 || m.requestInputActive()) {
			return m, m.pasteText()
		}
		if len(m.requests) > 0 {
			return m, m.requestKey(v)
		}
		switch v.String() {
		case "ctrl+o":
			return m, m.nextQuizSource()
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
			if m.knowledgeResultsVisible() && !m.knowledgeSearching {
				if len(m.knowledgeHits) > 0 {
					controls = append(controls, knowledgeFocusToggle)
				}
				if m.knowledgeExpanded && len(m.knowledgeHits) > 0 {
					controls = append(controls, knowledgeFocusResults)
				}
			}
			if m.knowledgeLinkCount() > 0 {
				controls = append(controls, knowledgeFocusLinks)
			}
			for n, i := range m.items {
				if i.kind == "fileChange" || i.kind == "evaluation" {
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
			if m.knowledgePreviewIdx >= 0 {
				m.closeKnowledgePreview()
				m.focus = knowledgeFocusResults
				m.draft.Blur()
				m.resize()
				return m, nil
			}
			if m.knowledgeLinksOverlay {
				m.closeKnowledgeLinksOverlay()
				return m, nil
			}
			if m.quizActive() && m.quiz.phase != "running" {
				return m, nil
			}
			if m.evaluating {
				m.stopEvaluation()
				m.busy = false
				m.status = "Evaluation canceled — Enter to retry"
				return m, nil
			}
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
			trimmed := strings.TrimSpace(text)
			switch trimmed {
			case "/quit", "/exit":
				return m, tea.Quit
			case "/login":
				m.draft.Reset()
				m.resize()
				return m, m.beginLogin()
			case "/logout":
				m.draft.Reset()
				m.resize()
				return m, m.beginLogout()
			}
			if m.quizActive() {
				return m, m.quizEnter(trimmed)
			}
			if m.busy || !m.connected || trimmed == "" {
				return m, nil
			}
			m.busy = true
			m.turnID = ""
			m.status = "Evaluating prompt…"
			m.follow = true
			m.dirty = true
			m.resize()
			m.refresh()
			return m, m.evaluatePrompt(text)
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
	m.selection = textSelection{}
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
			ID, Type, Text, Status, Command, Server, Tool string
			AggregatedOutput                              *string
			ExitCode                                      *int
			Changes                                       json.RawMessage
			Result                                        json.RawMessage
			Arguments                                     json.RawMessage
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
		if m.quizActive() && m.quiz.phase == "running" {
			m.quiz.turnID = p.Turn.ID
		}
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
		if p.Item.Type == "mcpToolCall" {
			var envelope struct {
				Item json.RawMessage `json:"item"`
			}
			raw := envelope.Item
			if json.Unmarshal(msg.Params, &envelope) == nil && len(envelope.Item) > 0 {
				raw = envelope.Item
			} else {
				raw, _ = json.Marshal(p.Item)
			}
			m.handleMcpToolCall(msg.Method, raw)
			m.dirty = true
			if !m.follow {
				m.newOutput = true
			}
			return
		}
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
		var turnErr error
		if p.Turn.Status != "completed" {
			turnErr = fmt.Errorf("turn ended without success")
		}
		m.recordCodexEnd(turnErr)
		m.busy = false
		if m.quizActive() && m.quiz.phase == "running" {
			m.quiz.turnID = p.Turn.ID
		}
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

func (m *model) traceContext() context.Context {
	if m.promptTrace != nil {
		return m.promptTrace
	}
	return m.ctx
}
func (m *model) recordCodexEnd(err error) {
	timing.Record(m.traceContext(), "codex.turn", m.codexStarted, err, nil)
	m.codexStarted = time.Time{}
}
