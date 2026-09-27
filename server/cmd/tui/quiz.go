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
	phase             string // preparing, running, generating, awaiting_reply, question, saving, grading, reveal, done
	quizID            string
	index             int
	sourceIndex       int
	editorSequence    int
	editorNotice      string
	editorCancel      context.CancelFunc
	feedback          string
	savedDraft        string
	pendingAnswer     string
	pendingGrade      *answerGrade
	panel             *conversationItem
	cancel            context.CancelFunc
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
	quizID  string
}

type quizAnswerSavedMsg struct {
	session *quizSession
	index   int
	answer  string
	grade   answerGrade
	err     error
}

type quizAnswerGradedMsg struct {
	session *quizSession
	index   int
	answer  string
	grade   answerGrade
	err     error
}

func (m *model) quizActive() bool {
	return m.quiz != nil && m.quiz.phase != "done" && m.quiz.phase != "awaiting_reply"
}

func (m *model) prepareQuiz(prompt string) tea.Cmd {
	q := &quizSession{request: quiz.Request{Input: prompt, Evaluation: m.lastEvaluation.Record.Evaluation, Conversation: m.evaluationContext}, phase: "preparing"}
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
	q.panel.raw = "Preparing review questions about the completed implementation…"
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
	request.UserID = m.authCreds.UserID
	if request.UserID != "" {
		request.ThreadID, request.TurnID = m.threadID, q.turnID
	}
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
		if err == nil && request.UserID != "" && len(response.Questions) > 0 && response.QuizID == "" {
			err = fmt.Errorf("quiz API did not return a quiz ID for saving answers; update the server")
		}
		return quizGeneratedMsg{session: q, request: request, result: response.Result, err: err, quizID: response.QuizID}
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
		return fmt.Errorf("API /%s unavailable or timed out", route)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var failure struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&failure) == nil {
			switch failure.Error.Code {
			case "cortex_invalid_response", "cortex_error", "quiz_timeout", "migration_required", "user_not_found", "project_not_found", "quiz_not_found", "answer_conflict":
				if failure.Error.Message != "" && len(failure.Error.Message) <= 500 {
					return fmt.Errorf("API /%s returned HTTP %d (%s): %s", route, resp.StatusCode, failure.Error.Code, failure.Error.Message)
				}
			}
		}
		return fmt.Errorf("API /%s returned HTTP %d", route, resp.StatusCode)
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
		q.quizID = v.quizID
		if len(q.result.Questions) == 0 {
			if len(q.request.Files) == 0 {
				timing.Record(m.traceContext(), "quiz.skipped_no_files", q.generationStarted, nil, map[string]int{"files": 0})
			}
			m.finishQuiz("")
			if len(q.request.Files) == 0 {
				// Keep the original evaluation and baseline across clarification
				// turns, while leaving the composer available for normal chat.
				q.phase = "awaiting_reply"
			}
			return nil
		}
		q.phase = "question"
		m.busy = false
		// Keep the active question at the end of the conversation.
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
		return m.openQuizSource()
	case quizAnswerSavedMsg:
		if m.quiz != v.session || v.session.phase != "saving" || v.session.index != v.index {
			return nil
		}
		q := m.quiz
		m.busy = false
		if v.err != nil {
			q.phase = "question"
			m.showQuizQuestion("Answer not saved: " + v.err.Error() + "\nYour answer is still in the composer. Press Enter to retry saving.")
			return nil
		}
		if strings.TrimSpace(m.draft.Value()) == v.answer {
			m.draft.Reset()
		}
		q.pendingGrade = nil
		m.showGradedAnswer(v.answer, v.grade, true)
		return nil
	case quizAnswerGradedMsg:
		if m.quiz != v.session || v.session.phase != "grading" || v.session.index != v.index {
			return nil
		}
		q := m.quiz
		m.busy = false
		if v.err != nil || v.grade.Validate() != nil {
			q.phase = "question"
			m.showQuizQuestion("Accuracy unavailable: Codex could not evaluate this answer. Your answer is still in the composer. Press Enter to retry.")
			return nil
		}
		if v.grade.Accuracy == 1 {
			v.grade.Explanation = ""
		}
		q.pendingAnswer, q.pendingGrade = v.answer, &v.grade
		if m.authCreds.UserID != "" {
			return m.startAnswerSave(v.answer, v.grade)
		}
		if strings.TrimSpace(m.draft.Value()) == v.answer {
			m.draft.Reset()
		}
		m.showGradedAnswer(v.answer, v.grade, false)

	}
	return nil
}

func (m *model) showGradedAnswer(answer string, grade answerGrade, saved bool) {
	q := m.quiz
	q.phase = "reveal"
	where := "Shown locally only; answer not saved."
	if saved {
		where = "Answer saved."
	}
	feedback := "Your answer: " + answer + "\n" + where + "\n" + fmt.Sprintf("Accuracy: %.2f", grade.Accuracy)
	if grade.Accuracy < 1 {
		feedback += "\n" + grade.Explanation
	}
	m.showQuizQuestion(feedback + "\nPress Enter to continue.")
}

func (m *model) startAnswerGrading(answer string) tea.Cmd {
	q := m.quiz
	q.phase = "grading"
	m.busy = true
	m.showQuizQuestion("Evaluating your answer with Codex…")
	m.status = "Evaluating answer…"
	ctx, cancel := context.WithTimeout(m.traceContext(), 90*time.Second)
	q.cancel = cancel
	index, workspace, question, files, grader := q.index, m.workspace, q.result.Questions[q.index], q.request.Files, m.grader
	return func() tea.Msg {
		defer cancel()
		grade, err := grader(ctx, workspace, question, files, answer)
		return quizAnswerGradedMsg{session: q, index: index, answer: answer, grade: grade, err: err}
	}
}

func (m *model) startAnswerSave(answer string, grade answerGrade) tea.Cmd {
	q := m.quiz
	q.phase = "saving"
	m.busy = true
	m.showQuizQuestion("Saving your graded answer…")
	m.status = "Saving answer…"
	ctx, cancel := context.WithTimeout(m.traceContext(), 15*time.Second)
	q.cancel = cancel
	index, server := q.index, m.apiServer()
	score := grade.Accuracy
	request := quiz.AnswerRequest{UserID: m.authCreds.UserID, QuizID: q.quizID, QuestionID: q.result.Questions[index].ID, Answer: answer, Graded: &score, Reasoning: grade.Explanation}
	return func() tea.Msg {
		defer cancel()
		var receipt quiz.AnswerReceipt
		err := postQuizJSON(ctx, server, "quiz-answers", request, &receipt)
		if err == nil && (receipt.ID == "" || receipt.UserID != request.UserID || receipt.QuizID != request.QuizID || receipt.QuestionID != request.QuestionID || receipt.Status != "graded" || receipt.Graded != score) {
			err = fmt.Errorf("invalid answer save acknowledgment")
		}
		return quizAnswerSavedMsg{session: q, index: index, answer: answer, grade: grade, err: err}
	}
}

func (m *model) quizFailure(err error) {
	q := m.quiz
	if q.panel == nil {
		q.panel = &conversationItem{kind: "quizPanel", done: true}
		m.items = append(m.items, q.panel)
	}
	m.finishQuiz("Quiz unavailable: " + err.Error() + "\nQuiz ended. Continue the conversation below.")
	if q.turnID == "" {
		m.draft.SetValue(q.request.Input)
	}
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
	q.feedback = feedback
	question := q.result.Questions[q.index]
	var b strings.Builder
	fmt.Fprintf(&b, "Question %d/%d\n", q.index+1, q.questionCount())
	b.WriteString("Review the implementation in your editor, then answer. Codex evaluates each answer.\n")
	if m.authCreds.UserID == "" {
		b.WriteString("Answers will stay local. Sign in to save them.\n")
	}
	fmt.Fprintf(&b, "\n%s\n", question.Question)
	if feedback != "" {
		b.WriteString("\n" + feedback + "\n")
	}
	for index, ref := range question.Evidence {
		marker := " "
		if index == q.sourceIndex {
			marker = ">"
		}
		fmt.Fprintf(&b, "\n%s %s:%d–%d", marker, ref.FilePath, ref.StartLine, ref.EndLine)
	}
	b.WriteString("\nCtrl+O opens/cycles source references in VS Code.\n")
	if q.editorNotice != "" {
		b.WriteString("\n" + q.editorNotice + "\n")
	}
	q.panel.raw = b.String()
	m.status = fmt.Sprintf("Quiz %d/%d — answer in the composer", q.index+1, q.questionCount())
	m.draft.Placeholder = "Your answer…"
	if q.phase == "reveal" {
		m.status = "Answer evaluated — Enter for next question"
		m.draft.Placeholder = "Press Enter to continue…"
	}
	m.quizFocus()
}

func (m *model) quizEnter(text string) tea.Cmd {
	q := m.quiz
	if q.phase == "saving" || q.phase == "grading" {
		return nil
	}
	if text == "/reveal" && q.phase != "running" && q.phase != "preparing" {
		if q.cancel != nil {
			q.cancel()
		}
		m.finishQuiz("Review ended. Continue the conversation below.")
		return nil
	}
	if q.phase == "reveal" {
		q.pendingGrade = nil
		q.pendingAnswer = ""
		q.index++
		if q.index >= q.questionCount() {
			m.finishQuiz("Review complete.")
		} else {
			q.phase = "question"
			q.sourceIndex = 0
			q.editorNotice = ""
			m.showQuizQuestion("")
			return m.openQuizSource()
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
	if m.authCreds.UserID != "" {
		if q.quizID == "" {
			m.showQuizQuestion("Answer not saved: this quiz has no storage ID. Keep your answer and generate a new quiz with the updated server.")
			return nil
		}
	}
	if q.pendingGrade != nil && q.pendingAnswer == text && m.authCreds.UserID != "" {
		return m.startAnswerSave(text, *q.pendingGrade)
	}
	q.pendingGrade = nil
	return m.startAnswerGrading(text)
}

func (m *model) finishQuiz(message string) {
	q := m.quiz
	q.phase = "done"
	if q.editorCancel != nil {
		q.editorCancel()
	}
	if q.cancel != nil {
		q.cancel()
	}
	m.busy = false
	if q.panel != nil {
		if message == "" {
			for i, item := range m.items {
				if item == q.panel {
					m.items = append(m.items[:i], m.items[i+1:]...)
					break
				}
			}
			q.panel = nil
		} else {
			q.panel.raw = message
		}
	}
	m.draft.SetValue(q.savedDraft)
	m.draft.Placeholder = "Ask Codex…"
	m.status = "Ready"
	if message == "" {
		m.follow = true
		m.quizJump = false
		m.dirty = true
		m.refresh()
	} else {
		m.quizFocus()
	}
}

func (q *quizSession) questionCount() int {
	return len(q.result.Questions)
}
