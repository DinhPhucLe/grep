package main

import (
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

func (m *model) requestView() string {
	r := m.requests[0]
	w := max(1, m.width-m.borderRows()-2)
	title := "Input required"
	details := ""
	choice := ""
	inputPrefix := ""
	if len(r.questions) > 0 {
		q := r.questions[r.question]
		title = fmt.Sprintf("Question %d/%d", r.question+1, len(r.questions))
		details = q.Question
		if r.selected < len(q.Options) {
			o := q.Options[r.selected]
			details += "\n\n" + o.Label + "\n" + o.Description
			choice = fmt.Sprintf("Option %d/%d: %s", r.selected+1, len(q.Options)+1, o.Label)
		} else {
			choice = "Your answer"
			inputPrefix = "> "
		}
	} else {
		title = "Allow Codex to continue?"
		details = m.approvalSummary(r.message)
		if r.showDetails {
			title = "Technical details"
			var pretty any
			_ = json.Unmarshal(r.message.Params, &pretty)
			b, _ := json.MarshalIndent(pretty, "", "  ")
			details = r.message.Method + "\n" + string(b)
		}
		choice = fmt.Sprintf("%s (%d/%d)", r.choices[r.selected].label, r.selected+1, len(r.choices))
		if r.message.Method == "mcpServer/elicitation/request" && r.selected == 2 {
			inputPrefix = "JSON: "
		}
	}
	detailLines := strings.Split(safeText(details), "\n")
	if len(r.questions) > 0 {
		detailLines = strings.Split(ansi.Wrap(safeText(details), w, ""), "\n")
	}
	maxWidth := 0
	for _, line := range detailLines {
		maxWidth = max(maxWidth, ansi.StringWidth(line))
	}
	for _, c := range r.choices {
		maxWidth = max(maxWidth, ansi.StringWidth(safeText(c.label))+2)
	}
	r.maxHorizontal = max(0, maxWidth-w)
	r.horizontal = min(r.horizontal, r.maxHorizontal)
	r.barWidth = w
	r.barRow = -1
	budget := m.requestBudget()
	if budget == 1 && inputPrefix != "" {
		r.input.Width = max(1, w-ansi.StringWidth(inputPrefix)-ansi.StringWidth(r.input.Prompt)-1)
		return ansi.Truncate(inputPrefix+r.input.View(), w, "")
	}
	titleRows := 0
	if budget >= 3 {
		titleRows = 1
	}
	choiceRows := 1
	showChoices := len(r.questions) == 0 && budget >= 10
	if showChoices {
		choiceRows = min(5, len(r.choices))
	}
	barRows := 0
	if r.maxHorizontal > 0 && budget >= 4 {
		barRows = 1
	}
	inputRows := 0
	if inputPrefix != "" {
		inputRows = 1
	}
	hintRows := 0
	if budget >= 8 {
		hintRows = 1
	}
	pageSize := max(0, budget-titleRows-choiceRows-barRows-inputRows-hintRows)
	start := min(r.detailOffset, max(0, len(detailLines)-1))
	var lines []string
	if titleRows > 0 {
		lines = append(lines, m.accent(title, "3"))
	}
	for _, line := range detailLines[start:min(len(detailLines), start+pageSize)] {
		lines = append(lines, horizontalSlice(line, r.horizontal, w))
	}
	if len(detailLines) > pageSize && hintRows > 0 {
		lines = append(lines, fmt.Sprintf("Ctrl+N/P details (%d/%d)", start+1, len(detailLines)))
	}
	if showChoices {
		startChoice := max(0, r.selected-4)
		for n := startChoice; n < min(len(r.choices), startChoice+5); n++ {
			prefix := "  "
			if n == r.selected {
				prefix = "> "
			}
			line := prefix + horizontalSlice(safeText(r.choices[n].label), r.horizontal, max(1, w-2))
			if n == r.selected {
				line = m.selectedStyle(line)
			}
			lines = append(lines, line)
		}
	} else {
		lines = append(lines, m.selectedStyle("> "+horizontalSlice(safeText(choice), r.horizontal, max(1, w-2))))
	}
	if inputPrefix != "" {
		r.input.Width = max(1, w-ansi.StringWidth(inputPrefix)-ansi.StringWidth(r.input.Prompt)-1)
		lines = append(lines, inputPrefix+r.input.View())
	}
	if barRows > 0 {
		r.barRow = len(lines)
		track := max(1, w-2)
		thumb := max(1, track*w/(w+r.maxHorizontal))
		pos := (track - thumb) * r.horizontal / r.maxHorizontal
		left, right := "◀", "▶"
		if m.opts.NoIcons {
			left, right = "<", ">"
		}
		lines = append(lines, m.accent(left+strings.Repeat("─", pos)+strings.Repeat("━", thumb)+strings.Repeat("─", track-thumb-pos)+right, "6"))
	}
	for n, line := range lines {
		lines[n] = ansi.Truncate(line, w, "")
	}
	return strings.Join(lines, "\n")
}

type requestChoice struct {
	label  string
	result any
}
type inputQuestion struct {
	ID, Header, Question string
	IsSecret             bool
	Options              []struct{ Label, Description string }
}
type pendingRequest struct {
	message                                           wireMessage
	choices                                           []requestChoice
	selected, detailOffset                            int
	questions                                         []inputQuestion
	question                                          int
	answers                                           map[string]any
	input                                             textinput.Model
	showDetails                                       bool
	horizontal, maxHorizontal, barRow, barY, barWidth int
}

func (m *model) enqueueRequest(msg wireMessage) tea.Cmd {
	m.selection = textSelection{}
	key := string(msg.ID)
	if m.seenRequests[key] {
		return nil
	}
	m.seenRequests[key] = true
	r := &pendingRequest{message: msg, answers: map[string]any{}, input: textinput.New()}
	r.input.CharLimit = 0
	r.input.Focus()
	if m.opts.ReducedMotion {
		r.input.Cursor.SetMode(cursor.CursorStatic)
	}
	decision := func(label string, value any) {
		r.choices = append(r.choices, requestChoice{label, map[string]any{"decision": value}})
	}
	switch msg.Method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		decision("Don't allow", "decline")
		decision("Allow once", "accept")
		decision("Allow for this conversation", "acceptForSession")
		decision("Stop this task", "cancel")
		var p struct {
			ProposedExecpolicyAmendment     json.RawMessage
			ProposedNetworkPolicyAmendments []json.RawMessage
			AvailableDecisions              []json.RawMessage
		}
		_ = json.Unmarshal(msg.Params, &p)
		if len(p.ProposedExecpolicyAmendment) > 0 && string(p.ProposedExecpolicyAmendment) != "null" {
			decision("Accept with command rule", map[string]any{"acceptWithExecpolicyAmendment": map[string]any{"execpolicy_amendment": p.ProposedExecpolicyAmendment}})
		}
		for _, a := range p.ProposedNetworkPolicyAmendments {
			decision("Apply network rule: "+string(a), map[string]any{"applyNetworkPolicyAmendment": map[string]any{"network_policy_amendment": a}})
		}
		if len(p.AvailableDecisions) > 0 {
			r.choices = nil
			// Keep the exact decision objects from the server. Put a non-granting
			// choice first when one exists, without inventing an unsupported reply.
			for _, denial := range []string{"decline", "cancel"} {
				for _, d := range p.AvailableDecisions {
					if string(d) == `"`+denial+`"` {
						decision(decisionLabel(d), d)
					}
				}
			}
			for _, d := range p.AvailableDecisions {
				if string(d) != `"decline"` && string(d) != `"cancel"` {
					decision(decisionLabel(d), d)
				}
			}
		} else {
			for n, c := range r.choices {
				v, _ := json.Marshal(c.result.(map[string]any)["decision"])
				r.choices[n].label = decisionLabel(v)
			}
		}
	case "execCommandApproval", "applyPatchApproval":
		decision("Don't allow", map[string]any{"denied": map[string]any{"rejection": "Declined by user"}})
		decision("Allow once", "approved")
		decision("Allow for this conversation", "approved_for_session")
		decision("Stop this task", "abort")
	case "item/permissions/requestApproval":
		var p struct{ Permissions json.RawMessage }
		_ = json.Unmarshal(msg.Params, &p)
		r.choices = []requestChoice{{"Don't allow", map[string]any{"permissions": map[string]any{}, "scope": "turn"}}, {"Allow for this task", map[string]any{"permissions": p.Permissions, "scope": "turn"}}, {"Allow for this conversation", map[string]any{"permissions": p.Permissions, "scope": "session"}}}
	case "item/tool/requestUserInput":
		var p struct{ Questions []inputQuestion }
		_ = json.Unmarshal(msg.Params, &p)
		r.questions = p.Questions
		if len(r.questions) == 0 {
			return m.reply(msg, map[string]any{"answers": r.answers}, nil)
		}
		r.configureQuestion()
	case "mcpServer/elicitation/request":
		// Most MCP elicitations are allow/deny (e.g. "run tool quack?"). Offer a
		// one-key Allow with empty content; keep custom JSON for real forms.
		r.choices = []requestChoice{
			{"Don't allow", map[string]any{"action": "decline", "content": nil, "_meta": nil}},
			{"Allow", map[string]any{"action": "accept", "content": map[string]any{}, "_meta": nil}},
			{"Send form data (JSON)", nil},
			{"Cancel request", map[string]any{"action": "cancel", "content": nil, "_meta": nil}},
		}
		r.selected = 1
	default:
		return m.reply(msg, nil, &rpcError{Code: -32601, Message: "Unsupported client request: " + msg.Method})
	}
	m.requests = append(m.requests, r)
	m.draft.Blur()
	m.syncRequestInputFocus()
	m.resize()
	return nil
}
func (r *pendingRequest) configureQuestion() {
	r.selected = 0
	r.input.Reset()
	r.input.EchoMode = textinput.EchoNormal
	if r.questions[r.question].IsSecret {
		r.input.EchoMode = textinput.EchoPassword
	}
}
func (m *model) reply(msg wireMessage, result any, e *rpcError) tea.Cmd {
	return func() tea.Msg {
		payload := map[string]any{"id": msg.ID}
		if e != nil {
			payload["error"] = e
		} else {
			payload["result"] = result
		}
		if m.client == nil {
			return callDoneMsg{method: "reply"}
		}
		return callDoneMsg{method: "reply", err: m.client.writeJSON(payload)}
	}
}
func (m *model) requestKey(k tea.KeyMsg) tea.Cmd {
	r := m.requests[0]
	count := len(r.choices)
	if len(r.questions) > 0 {
		count = len(r.questions[r.question].Options) + 1
	}
	switch k.String() {
	case "left", "right", "alt+left", "alt+right":
		editing := len(r.questions) > 0 || m.requestInputActive()
		if !editing || k.Alt {
			step := 8
			if k.String() == "left" || k.String() == "alt+left" {
				step = -8
			}
			r.horizontal = max(0, min(r.maxHorizontal, r.horizontal+step))
			return nil
		}
	case "ctrl+d":
		if len(r.questions) == 0 {
			r.showDetails = !r.showDetails
			r.detailOffset = 0
			r.horizontal = 0
			m.resize()
		}
		return nil
	case "ctrl+n":
		r.detailOffset += 1
		return nil
	case "ctrl+p":
		r.detailOffset = max(0, r.detailOffset-1)
		return nil
	case "tab", "down":
		r.selected = (r.selected + 1) % max(1, count)
		r.detailOffset = 0
		m.syncRequestInputFocus()
		return nil
	case "shift+tab", "up":
		r.selected = (r.selected + max(1, count) - 1) % max(1, count)
		r.detailOffset = 0
		m.syncRequestInputFocus()
		return nil
	case "enter":
		if m.requestInputActive() && (strings.TrimSpace(r.input.Value()) == "/quit" || strings.TrimSpace(r.input.Value()) == "/exit") {
			return tea.Quit
		}
		var result any
		if len(r.questions) > 0 {
			q := r.questions[r.question]
			answer := r.input.Value()
			if r.selected < len(q.Options) {
				answer = q.Options[r.selected].Label
			}
			if strings.TrimSpace(answer) == "" {
				return nil
			}
			r.answers[q.ID] = map[string]any{"answers": []string{answer}}
			r.question++
			if r.question < len(r.questions) {
				r.configureQuestion()
				m.syncRequestInputFocus()
				m.resize()
				return nil
			}
			result = map[string]any{"answers": r.answers}
		} else {
			result = r.choices[r.selected].result
			if r.message.Method == "mcpServer/elicitation/request" && r.selected == 2 {
				raw := strings.TrimSpace(r.input.Value())
				if raw == "" {
					raw = "{}"
				}
				var content any
				if json.Unmarshal([]byte(raw), &content) != nil {
					m.status = "Enter valid JSON for the form (or pick Allow)"
					return nil
				}
				result = map[string]any{"action": "accept", "content": content, "_meta": nil}
			}
		}
		m.requests = m.requests[1:]
		if len(m.requests) == 0 && m.focus == -1 {
			m.draft.Focus()
		}
		m.resize()
		m.dirty = true
		return m.reply(r.message, result, nil)
	}
	var cmd tea.Cmd
	r.input, cmd = r.input.Update(k)
	return cmd
}

func (m *model) requestMouse(v tea.MouseMsg) {
	r := m.requests[0]
	if v.Action == tea.MouseActionPress {
		switch v.Button {
		case tea.MouseButtonWheelLeft:
			r.horizontal = max(0, r.horizontal-8)
			return
		case tea.MouseButtonWheelRight:
			r.horizontal = min(r.maxHorizontal, r.horizontal+8)
			return
		}
	}
	if r.barRow < 0 || v.Y != r.barY || r.maxHorizontal == 0 || v.Button != tea.MouseButtonLeft {
		return
	}
	if v.Action != tea.MouseActionPress && v.Action != tea.MouseActionMotion {
		return
	}
	if v.X <= 1 {
		r.horizontal = max(0, r.horizontal-8)
	} else if v.X >= r.barWidth {
		r.horizontal = min(r.maxHorizontal, r.horizontal+8)
	} else {
		r.horizontal = (v.X - 1) * r.maxHorizontal / max(1, r.barWidth-2)
	}
}
