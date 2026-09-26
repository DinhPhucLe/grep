package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cortisol-server/internal/evaluation"
	tea "github.com/charmbracelet/bubbletea"
)

func TestPromptEvaluationBeforeCodex(t *testing.T) {
	for _, score := range []float64{0, 0.3, 0.31, 1} {
		t.Run(fmt.Sprint(score), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/evaluations" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				var input evaluation.Request
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
				}
				if input.Input != "original prompt" || len(input.Conversation) != 2 || input.Conversation[0].Content != "prior user" || input.Conversation[1].Content != "prior answer" {
					t.Errorf("wrong context: %+v", input)
				}
				body := evaluation.Body{Verdict: "clear", Summary: "Test evaluation", AmbiguityScore: score, Gaps: []evaluation.Gap{}}
				if score > ambiguityThreshold {
					body.Verdict = "ambiguous"
					body.Gaps = []evaluation.Gap{{Description: "missing behavior", Consequence: "different result"}}
				}
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(evaluation.Record{Evaluation: body})
			}))
			defer server.Close()
			m := newModel(nil, "workspace", uiOptions{EvaluationServer: server.URL})
			m.connected, m.threadID = true, "thread"
			m.items = []*conversationItem{{kind: "userMessage", raw: "prior user"}, {kind: "agentMessage", raw: "prior answer"}}
			m.draft.SetValue("original prompt")
			_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if !m.evaluating || !m.busy || len(m.items) != 2 {
				t.Fatal("prompt bypassed evaluation")
			}
			_, send := m.Update(cmd())
			if send == nil || m.lastEvaluation == nil || m.lastEvaluation.NeedsQuiz != (score > 0.3) {
				t.Fatal("wrong threshold branch")
			}
			if got := send().(callDoneMsg); got.method != "turn/start" {
				t.Fatal(got)
			}
			if m.items[2].raw != "original prompt" || m.draft.Value() != "" {
				t.Fatal("prompt changed or draft not cleared")
			}
		})
	}
}

func TestEvaluationFailuresKeepDraft(t *testing.T) {
	for _, body := range []string{`{}`, `{"evaluation":{"verdict":"clear","summary":"missing score","gaps":[]}}`, `{"evaluation":{"verdict":"clear","summary":"bad score","ambiguity_score":2,"gaps":[]}}`, "unavailable"} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if body == "unavailable" {
					w.WriteHeader(503)
				} else {
					w.WriteHeader(201)
				}
				fmt.Fprint(w, body)
			}))
			defer server.Close()
			m := newModel(nil, "w", uiOptions{EvaluationServer: server.URL})
			m.connected = true
			m.draft.SetValue("keep me")
			_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			_, send := m.Update(cmd())
			if send != nil || m.busy || m.draft.Value() != "keep me" || !strings.Contains(m.status, "Evaluation failed") {
				t.Fatal("failed evaluation dispatched or lost draft")
			}
		})
	}
}

func TestCanceledEvaluationIgnoresLateReply(t *testing.T) {
	m := newModel(nil, "w", uiOptions{})
	m.connected = true
	m.draft.SetValue("keep me")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	sequence := m.evaluationSequence
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_, cmd := m.Update(evaluationDoneMsg{sequence: sequence, prompt: "keep me"})
	if cmd != nil || m.busy || m.evaluating || m.draft.Value() != "keep me" {
		t.Fatal("late evaluation started a turn")
	}
}
