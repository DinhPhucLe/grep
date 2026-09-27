package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"cortisol-server/internal/evaluation"
	"cortisol-server/internal/timing"
	tea "github.com/charmbracelet/bubbletea"
)

const ambiguityThreshold = evaluation.AmbiguityThreshold

type promptEvaluation struct {
	Record    evaluation.Record
	NeedsQuiz bool
}

type evaluationDoneMsg struct {
	sequence int
	prompt   string
	record   evaluation.Record
	err      error
}

func (m *model) stopEvaluation() {
	if m.cancelEvaluation != nil {
		m.cancelEvaluation()
		m.cancelEvaluation = nil
	}
	m.evaluating = false
	m.evaluationSequence++ // Ignore late replies after cancellation or disconnect.
}

func (m *model) evaluatePrompt(prompt string) tea.Cmd {
	m.stopEvaluation()
	m.promptTrace = timing.New(m.ctx, m.timingSink)
	m.promptStarted = time.Now()
	m.lastEvaluation = nil
	m.evaluating = true
	sequence := m.evaluationSequence
	ctx, cancel := context.WithTimeout(m.traceContext(), 75*time.Second)
	m.cancelEvaluation = cancel
	request := evaluation.Request{Input: prompt}
	// Send recent conversation as context, excluding activity and evaluation cards.
	// Keep within the API's 50-message and combined-text limits.
	total := len(prompt)
	for n := len(m.items) - 1; n >= 0 && len(request.Conversation) < 50; n-- {
		i := m.items[n]
		role := ""
		switch i.kind {
		case "userMessage":
			role = "user"
		case "agentMessage":
			role = "assistant"
		default:
			continue
		}
		if i.raw == "" {
			continue
		}
		if total+len(i.raw) > 256000 {
			break
		}
		total += len(i.raw)
		request.Conversation = append(request.Conversation, evaluation.Message{Role: role, Content: i.raw})
	}
	for left, right := 0, len(request.Conversation)-1; left < right; left, right = left+1, right-1 {
		request.Conversation[left], request.Conversation[right] = request.Conversation[right], request.Conversation[left]
	}
	m.evaluationContext = append([]evaluation.Message(nil), request.Conversation...)
	server := m.opts.EvaluationServer
	if server == "" {
		server = "http://127.0.0.1:8080"
	}
	return func() tea.Msg {
		defer cancel()
		record, err := fetchEvaluation(ctx, server, request)
		return evaluationDoneMsg{sequence: sequence, prompt: prompt, record: record, err: err}
	}
}

func fetchEvaluation(ctx context.Context, server string, input evaluation.Request) (record evaluation.Record, err error) {
	ctx = timing.ForRequest(ctx, "evaluations")
	started := time.Now()
	defer func() { timing.Record(ctx, "http.client", started, err, nil) }()

	if err := input.Validate(); err != nil {
		return record, err
	}
	u, err := url.Parse(server)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return record, fmt.Errorf("invalid evaluation server URL")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return record, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.JoinPath("evaluations").String(), bytes.NewReader(body))
	if err != nil {
		return record, err
	}
	req.Header.Set("Content-Type", "application/json")
	timing.Headers(ctx, req)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return record, fmt.Errorf("could not reach evaluation API (check server or timeout)")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return record, fmt.Errorf("evaluation API returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return record, fmt.Errorf("could not read evaluation response")
	}
	// Explicit null skips evaluation; a missing field is not a valid response.
	var required struct {
		Evaluation struct {
			Score json.RawMessage `json:"ambiguity_score"`
		} `json:"evaluation"`
	}
	if json.Unmarshal(data, &required) != nil || required.Evaluation.Score == nil || json.Unmarshal(data, &record) != nil || record.Evaluation.Validate() != nil {
		return record, fmt.Errorf("invalid evaluation response")
	}
	return record, nil
}

func (m *model) finishEvaluation(result evaluationDoneMsg) tea.Cmd {
	if !m.evaluating || result.sequence != m.evaluationSequence || !m.connected {
		return nil
	}
	m.stopEvaluation()
	if result.err != nil {
		m.busy = false
		m.status = "Evaluation failed: " + result.err.Error() + " — Enter to retry"
		return nil // Keep the draft; do not send an unevaluated prompt to Codex.
	}
	score := result.record.Evaluation.AmbiguityScore
	if m.quiz != nil && m.quiz.phase == "awaiting_reply" {
		if score == nil {
			// The reply stays unrated; implementation still belongs to the
			// original ambiguous request and its workspace baseline.
			m.quiz.phase = "running"
			m.quiz.turnID = ""
		} else {
			// A new implementation request supersedes the pending one.
			m.quiz.phase = "done"
		}
	}
	m.lastEvaluation = &promptEvaluation{Record: result.record, NeedsQuiz: score != nil && *score > ambiguityThreshold}
	branch := "Pass — no quiz"
	if m.lastEvaluation.NeedsQuiz {
		branch = "Quiz after implementation — review referenced code in your editor"
	}
	m.items = append(m.items, &conversationItem{kind: "userMessage", raw: result.prompt, done: true})
	if score != nil {
		m.items = append(m.items, &conversationItem{kind: "evaluation", command: fmt.Sprintf("Ambiguity score: %.2f", *score), status: branch, raw: result.record.Evaluation.Summary, done: true})
	}
	if m.draft.Value() == result.prompt {
		m.draft.Reset()
	}
	m.status = "Waiting for response"
	m.dirty = true
	m.refresh()
	if m.lastEvaluation.NeedsQuiz {
		return m.prepareQuiz(result.prompt)
	}
	return m.call("turn/start", map[string]any{"threadId": m.threadID, "input": []map[string]any{{"type": "text", "text": result.prompt}}})
}
