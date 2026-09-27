package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cortisol-server/internal/quiz"
	"cortisol-server/internal/timing"
	tea "github.com/charmbracelet/bubbletea"
)

type quizSession struct {
	generationStarted time.Time
	firstReady        bool
	request           quiz.Request
	result            quiz.Result
	baseline          fileSnapshot
	turnID            string
	phase             string // preparing, running, generating, question, grading, reveal, failed, done
	index, attempts   int
	resolved          map[int]bool
	savedDraft        string
	panel             *conversationItem
	cancel            context.CancelFunc
	feedback          string
}
type quizPreparedMsg struct {
	session  *quizSession
	baseline fileSnapshot
	err      error
}
type quizGeneratedMsg struct {
	session *quizSession
	request quiz.Request
	result  quiz.Result
	err     error
}
type quizGradedMsg struct {
	session *quizSession
	correct bool
	err     error
}

func (m *model) quizActive() bool { return m.quiz != nil && m.quiz.phase != "done" }

func (m *model) prepareQuiz(prompt string) tea.Cmd {
	q := &quizSession{request: quiz.Request{Input: prompt, Evaluation: m.lastEvaluation.Record.Evaluation, Conversation: m.evaluationContext}, phase: "preparing", resolved: map[int]bool{}}
	m.quiz = q
	m.status = "Preparing code review…"
	workspace := m.workspace
	ctx, cancel := context.WithTimeout(m.traceContext(), 30*time.Second)
	q.cancel = cancel
	return func() tea.Msg {
		defer cancel()
		started := time.Now()
		baseline, err := snapshotWorkspace(ctx, workspace)
		timing.Record(ctx, "workspace.baseline", started, err, nil)
		return quizPreparedMsg{q, baseline, err}
	}
}

func (m *model) startQuizGeneration() tea.Cmd {
	q := m.quiz
	q.phase = "generating"
	q.generationStarted = time.Now()
	m.busy = true
	m.status = "Preparing questions…"
	if q.panel == nil {
		q.savedDraft = m.draft.Value()
		m.draft.Reset()
		q.panel = &conversationItem{kind: "quizPanel", done: true, expanded: true}
		m.items = append(m.items, q.panel)
	}
	q.panel.raw = "Preparing quiz. Generated output is temporarily withheld in this TUI.\nFiles on disk and optional RPC logs remain accessible."
	m.quizFocus()
	var changes []string
	for _, item := range m.items {
		if item.kind == "fileChange" && strings.HasPrefix(item.key, m.threadID+"/"+q.turnID+"/") && item.status != "failed" && item.status != "declined" {
			changes = append(changes, item.raw)
		}
	}
	ctx, cancel := context.WithTimeout(m.traceContext(), 75*time.Second)
	q.cancel = cancel
	request, workspace, server, baseline := q.request, m.workspace, m.apiServer(), q.baseline
	return func() tea.Msg {
		defer cancel()
		started := time.Now()
		files, err := collectQuizFiles(ctx, workspace, baseline, changes)
		size := 0
		for _, file := range files {
			size += len(file.Content)
		}
		timing.Record(ctx, "workspace.collect", started, err, map[string]int{"files": len(files), "code_bytes": size})
		request.Files = files
		if err == nil && len(files) == 0 {
			// A successful Codex turn may only ask a clarification question.
			// There is no implementation to quiz; release the conversation instead.
			return quizGeneratedMsg{session: q, request: request, result: quiz.Result{
				Questions: []quiz.Question{}, NoQuestionsReason: "No generated text files to quiz.",
			}}
		}
		if err == nil {
			err = request.Validate()
		}
		var response quiz.Response
		if err == nil {
			err = postQuizJSON(ctx, server, "quizzes", request, &response)
		}
		if err == nil {
			err = response.Result.Validate(request)
		}
		return quizGeneratedMsg{session: q, request: request, result: response.Result, err: err}
	}
}

func (m *model) apiServer() string {
	if m.opts.EvaluationServer != "" {
		return m.opts.EvaluationServer
	}
	return "http://127.0.0.1:8080"
}

func postQuizJSON(ctx context.Context, server, route string, input, output any) (err error) {
	ctx = timing.ForRequest(ctx, route)
	started := time.Now()
	defer func() { timing.Record(ctx, "http.client", started, err, nil) }()

	u, err := url.Parse(server)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid API URL")
	}
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", u.JoinPath(route).String(), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	timing.Headers(ctx, req)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("quiz API unavailable or timed out")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("quiz API returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return fmt.Errorf("invalid quiz response size")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(output) != nil || decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("invalid quiz response")
	}
	return nil
}

func (m *model) handleQuizMessage(msg tea.Msg) tea.Cmd {
	switch v := msg.(type) {
	case quizPreparedMsg:
		if m.quiz != v.session || v.session.phase != "preparing" {
			return nil
		}
		if v.err != nil {
			m.quizFailure(v.err)
			return nil
		}
		v.session.baseline = v.baseline
		v.session.phase = "running"
		m.status = "Waiting for response"
		return m.call("turn/start", map[string]any{"threadId": m.threadID, "input": []map[string]any{{"type": "text", "text": v.session.request.Input}}})
	case quizGeneratedMsg:
		if m.quiz != v.session || v.session.phase != "generating" {
			return nil
		}
		if v.err != nil {
			m.quizFailure(v.err)
			return nil
		}
		q := m.quiz
		q.request = v.request
		q.result = v.result
		if len(q.result.Questions) == 0 {
			if len(q.request.Files) == 0 {
				timing.Record(m.traceContext(), "quiz.skipped_no_files", q.generationStarted, nil, map[string]int{"files": 0})
				m.finishQuiz("No generated text files to quiz. Continue the conversation below.")
			} else {
				m.finishQuiz("No grounded quiz questions were available. Code review is open.")
			}
			return nil
		}
		q.phase = "question"
		m.busy = false
		for _, file := range q.request.Files {
			m.items = append(m.items, &conversationItem{kind: "quizCode", command: file.Path, done: true, expanded: true, quizOwner: q})
		}
		// Keep the active question at the end while preserving the code browser above it.
		for n, item := range m.items {
			if item == q.panel {
				m.items = append(m.items[:n], m.items[n+1:]...)
				break
			}
		}
		m.items = append(m.items, q.panel)
		m.showQuizQuestion("")
		if !q.firstReady {
			q.firstReady = true
			timing.Record(m.traceContext(), "quiz.first_question_ready", m.promptStarted, nil, nil)
			timing.Record(m.traceContext(), "quiz.prepare_to_ready", q.generationStarted, nil, nil)
		}
		return nil
	case quizGradedMsg:
		if m.quiz != v.session || v.session.phase != "grading" {
			return nil
		}
		q := m.quiz
		m.busy = false
		if v.err != nil {
			q.phase = "question"
			m.showQuizQuestion("Grading unavailable. Your attempt was not counted; press Enter to retry.")
			return nil
		}
		q.attempts++
		m.draft.Reset()
		if !v.correct && q.attempts < 2 {
			q.phase = "question"
			m.showQuizQuestion("Not correct yet. Try once more.")
			return nil
		}
		q.resolved[q.index] = true
		q.phase = "reveal"
		feedback := "Two attempts used. Here is the implementation."
		if v.correct {
			feedback = "Correct. Here is the implementation."
		}
		m.showQuizQuestion(feedback + "\nPress Enter to continue.")
	}
	return nil
}

func (m *model) quizFailure(err error) {
	q := m.quiz
	q.phase = "failed"
	m.busy = false
	if q.panel == nil {
		q.panel = &conversationItem{kind: "quizPanel", done: true}
		m.items = append(m.items, q.panel)
	}
	q.panel.raw = "Quiz unavailable: " + err.Error() + "\nEnter /retry to try again, or /reveal to end this quiz and review all output. No answer attempt was counted."
	m.status = "Quiz unavailable"
	m.quizFocus()
}

func (m *model) quizFocus() {
	m.focus = -1
	m.draft.Focus()
	m.selection = textSelection{}
	m.follow = false
	m.quizJump = true
	m.dirty = true
	m.resize()
	m.refresh()
}

func (m *model) showQuizQuestion(feedback string) {
	started := time.Now()
	defer func() { timing.Record(m.traceContext(), "ui.question_render", started, nil, nil) }()
	q := m.quiz
	question := q.result.Questions[q.index]
	q.feedback = feedback
	var b strings.Builder
	fmt.Fprintf(&b, "Question %d/%d · attempt %d/2\n", q.index+1, q.questionCount(), min(q.attempts+1, 2))
	b.WriteString("Review snapshot · hidden in this TUI only; files remain on disk.\n")
	fmt.Fprintf(&b, "\n%s\n", question.Question)
	if feedback != "" {
		b.WriteString("\n" + feedback + "\n")
	}
	for _, ref := range question.Evidence {
		fmt.Fprintf(&b, "\n%s:%d–%d\n", ref.FilePath, ref.StartLine, ref.EndLine)
		for _, file := range q.request.Files {
			if file.Path == ref.FilePath {
				lines := strings.Split(strings.TrimSuffix(file.Content, "\n"), "\n")
				for n := max(1, ref.StartLine-2); n <= min(len(lines), ref.EndLine+2); n++ {
					line := lines[n-1]
					if quizLineHidden(q, file.Path, n) {
						line = "[hidden for quiz]"
					}
					fmt.Fprintf(&b, "%5d | %s\n", n, line)
				}
			}
		}
	}
	q.panel.raw = b.String()
	m.status = fmt.Sprintf("Quiz %d/%d — answer in the composer", q.index+1, q.questionCount())
	if q.phase == "reveal" {
		m.status = "Code revealed — Enter for next question"
	}
	m.draft.Placeholder = "Your answer…"
	m.quizFocus()
}

func (m *model) quizEnter(text string) tea.Cmd {
	q := m.quiz
	if text == "/reveal" && q.phase != "running" && q.phase != "preparing" {
		if q.cancel != nil {
			q.cancel()
		}
		m.finishQuiz("Quiz ended without scoring. All code is available for review.")
		return nil
	}
	if q.phase == "failed" {
		if text == "/retry" {
			m.draft.Reset()
			if q.turnID == "" {
				m.finishQuiz("Turn was not started; submit the prompt again.")
				m.draft.SetValue(q.request.Input)
				return nil
			}
			return m.startQuizGeneration()
		}
		return nil
	}
	if q.phase == "reveal" {
		q.index++
		q.attempts = 0
		m.draft.Reset()
		if q.index == q.questionCount() {
			m.finishQuiz("Quiz complete. All implementation code is now visible.")
		} else {
			q.phase = "question"
			m.showQuizQuestion("")
		}
		return nil
	}
	if q.phase != "question" || strings.TrimSpace(text) == "" {
		return nil
	}
	if len(text) > 8000 {
		m.status = "Answer is too long (maximum 8000 bytes)"
		return nil
	}
	q.phase = "grading"
	m.busy = true
	m.status = "Checking answer…"
	request := quiz.AnswerRequest{Quiz: q.request, Questions: q.result, QuestionID: q.result.Questions[q.index].ID, Answer: text}
	ctx, cancel := context.WithTimeout(m.traceContext(), 75*time.Second)
	q.cancel = cancel
	server := m.apiServer()
	return func() tea.Msg {
		defer cancel()
		var response struct {
			Correct *bool `json:"correct"`
		}
		err := postQuizJSON(ctx, server, "quiz-answers", request, &response)
		if err == nil && response.Correct == nil {
			err = fmt.Errorf("missing grade")
		}
		return quizGradedMsg{q, response.Correct != nil && *response.Correct, err}
	}
}

func (m *model) finishQuiz(message string) {
	q := m.quiz
	q.phase = "done"
	if q.cancel != nil {
		q.cancel()
	}
	m.busy = false
	for n := 0; n < q.questionCount(); n++ {
		q.resolved[n] = true
	}
	for _, item := range m.items {
		if item.quizOwner == q {
			item.withheld = false
		}
	}
	if q.panel != nil {
		q.panel.raw = message
	}
	m.draft.SetValue(q.savedDraft)
	m.draft.Placeholder = "Ask Codex…"
	m.status = "Ready"
	m.quizFocus()
}

func quizFileView(q *quizSession, path string) string {
	var content string
	for _, file := range q.request.Files {
		if file.Path == path {
			content = file.Content
			break
		}
	}
	var b strings.Builder
	b.WriteString(path + "\n")
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	for n, line := range lines {
		if quizLineHidden(q, path, n+1) {
			line = "[hidden for quiz]"
		}
		fmt.Fprintf(&b, "%5d | %s\n", n+1, line)
	}
	return b.String()
}

func quizLineHidden(q *quizSession, path string, line int) bool {
	if q.phase == "done" {
		return false
	}
	for index, question := range q.result.Questions {
		if q.resolved[index] {
			continue
		}
		for _, ref := range question.Evidence {
			if ref.FilePath == path && line >= ref.StartLine && line <= ref.EndLine {
				return true
			}
		}
	}
	return false
}

func (q *quizSession) questionCount() int {
	return len(q.result.Questions)
}
