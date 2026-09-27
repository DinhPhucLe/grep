package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"cortisol-server/internal/evaluation"
	"cortisol-server/internal/quiz"
	"cortisol-server/internal/timing"
	tea "github.com/charmbracelet/bubbletea"
)

func TestClarificationOnlyTurnReturnsToChatWithoutQuizAPI(t *testing.T) {
	var quizCalls atomic.Int32
	evaluations := make(chan evaluation.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/evaluations" {
			quizCalls.Add(1)
			var request quiz.Request
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request.Evaluation.AmbiguityScore == nil || request.Evaluation.Verdict != "ambiguous" {
				t.Error("original evaluation was lost")
			}
			json.NewEncoder(w).Encode(quiz.Response{Result: fourQuestions()})
			return
		}
		var input evaluation.Request
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		evaluations <- input
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(evaluation.Record{Evaluation: evaluation.Body{Verdict: "not_applicable", Summary: "The user selected an offered option", AmbiguityScore: nil, Gaps: []evaluation.Gap{}}})
	}))
	defer server.Close()
	m := testQuizModel(t, server.URL)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	var timings []timing.Event
	m.promptTrace = timing.New(m.ctx, func(e timing.Event) { timings = append(timings, e) })
	clarification := "Should users sign in with email/password, GitHub, or Google?"
	m.Update(event("item/completed", `{"threadId":"thread","turnId":"turn","item":{"id":"clarification","type":"agentMessage","text":"`+clarification+`"}}`))
	m.draft.SetValue("Email and password")
	_, collect := m.Update(event("turn/completed", `{"threadId":"thread","turn":{"id":"turn","status":"completed"}}`))
	if collect == nil {
		t.Fatal("completed turn did not inspect generated files")
	}
	reply := collect()
	_, next := m.Update(reply)
	if next != nil || m.quizActive() || m.busy || m.status != "Ready" {
		t.Fatalf("clarification did not return to chat: phase=%s status=%s", m.quiz.phase, m.status)
	}
	for _, item := range m.items {
		if item.kind == "quizPanel" {
			t.Fatal("no-code turn left a quiz skip notice")
		}
	}
	if quizCalls.Load() != 0 {
		t.Fatal("clarification-only turn called quiz API")
	}
	if m.draft.Value() != "Email and password" || m.draft.Placeholder != "Ask Codex…" {
		t.Fatal("draft/composer not restored")
	}
	if !strings.Contains(m.viewport.View(), clarification) {
		t.Fatal("clarification is still hidden")
	}
	_, next = m.Update(reply)
	if next != nil {
		t.Fatal("duplicate no-files result scheduled work")
	}
	skipped := 0
	for _, e := range timings {
		if e.Stage == "quiz.skipped_no_files" {
			skipped++
		}
	}
	if skipped != 1 {
		t.Fatalf("skip not diagnosed exactly once: %d", skipped)
	}
	_, evaluate := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if evaluate == nil || !m.evaluating {
		t.Fatal("follow-up answer was blocked by quiz state")
	}
	_, start := m.Update(evaluate())
	if start == nil || m.evaluating {
		t.Fatal("follow-up did not continue to Codex")
	}
	input := <-evaluations
	if input.Input != "Email and password" {
		t.Fatal("follow-up changed")
	}
	found := false
	for _, message := range input.Conversation {
		if message.Role == "assistant" && message.Content == clarification {
			found = true
		}
	}
	if !found {
		t.Fatal("clarification missing from follow-up evaluation context")
	}
	if m.quiz.phase != "running" || m.lastEvaluation.Record.Evaluation.AmbiguityScore != nil {
		t.Fatal("unrated reply did not resume the original pending quiz")
	}
	if err := os.WriteFile(filepath.Join(m.workspace, "main.go"), []byte("one\ntwo\nthree\nfour\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m.Update(event("turn/started", `{"threadId":"thread","turn":{"id":"implementation","status":"inProgress"}}`))
	m.Update(event("item/completed", `{"threadId":"thread","turnId":"implementation","item":{"id":"change","type":"fileChange","status":"completed","changes":[{"path":"main.go","kind":{"type":"add"}}]}}`))
	_, generate := m.Update(event("turn/completed", `{"threadId":"thread","turn":{"id":"implementation","status":"completed"}}`))
	if generate == nil {
		t.Fatal("implementation after clarification did not trigger quiz")
	}
	m.Update(generate())
	if quizCalls.Load() != 1 || m.quiz.phase != "question" || !strings.Contains(m.quiz.panel.raw, "Question 1/4") {
		t.Fatalf("quiz was not rendered after the implementation: calls=%d phase=%s status=%s panel=%+v", quizCalls.Load(), m.quiz.phase, m.status, m.quiz.panel)
	}
}

func TestQuizCollectionErrorsStillFailWithoutAPI(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	m := testQuizModel(t, server.URL)
	if err := os.WriteFile(filepath.Join(m.workspace, "binary.bin"), []byte{0, 1, 2}, 0600); err != nil {
		t.Fatal(err)
	}
	m.Update(event("item/completed", `{"threadId":"thread","turnId":"turn","item":{"id":"change","type":"fileChange","status":"completed","changes":[{"path":"binary.bin","kind":{"type":"add"}}]}}`))
	m.Update(event("item/completed", `{"threadId":"thread","turnId":"turn","item":{"id":"answer","type":"agentMessage","text":"Implementation output"}}`))
	_, next := m.Update(m.startQuizGeneration()())
	if next != nil || m.quiz.phase != "done" || calls.Load() != 0 {
		t.Fatal("collection error was treated as normal empty output")
	}
	if !strings.Contains(m.quiz.panel.raw, "not text") {
		t.Fatal("collection failure lost its reason")
	}

}

func TestLateNoFilesResultDoesNotCloseNewQuiz(t *testing.T) {
	m := testQuizModel(t, "")
	collect := m.startQuizGeneration()
	oldSession := m.quiz
	m.quizEnter("/reveal")
	m.prepareQuiz("another ambiguous request")
	m.Update(collect())
	if m.quiz == oldSession || m.quiz.phase != "preparing" {
		t.Fatal("late empty result closed the new session")
	}
	if m.quiz.cancel != nil {
		m.quiz.cancel()
	}
}

func TestUnchangedGitWorkspaceReturnsToChat(t *testing.T) {
	m := testQuizModel(t, "http://127.0.0.1:1")
	if out, err := exec.Command("git", "init", "--quiet", m.workspace).CombinedOutput(); err != nil {
		t.Fatalf("git init: %s %v", out, err)
	}
	if err := os.WriteFile(filepath.Join(m.workspace, "existing.go"), []byte("preexisting untracked code"), 0600); err != nil {
		t.Fatal(err)
	}
	var err error
	m.quiz.baseline, err = snapshotWorkspace(m.ctx, m.workspace)
	if err != nil || len(m.quiz.baseline) != 1 {
		t.Fatalf("baseline: %v %v", m.quiz.baseline, err)
	}
	_, next := m.Update(m.startQuizGeneration()())
	if next != nil || m.quizActive() || m.busy {
		t.Fatal("unchanged files caused a quiz or failure")
	}
}

func TestNewClearRequestDiscardsPendingQuiz(t *testing.T) {
	m := testQuizModel(t, "")
	m.Update(m.startQuizGeneration()())
	old := m.quiz
	score := 0.1
	m.evaluating = true
	cmd := m.finishEvaluation(evaluationDoneMsg{sequence: m.evaluationSequence, prompt: "change the title to Hello", record: evaluation.Record{Evaluation: evaluation.Body{Verdict: "clear", Summary: "Specific edit", AmbiguityScore: &score, Gaps: []evaluation.Gap{}}}})
	if cmd == nil || m.quizActive() || old.phase != "done" {
		t.Fatal("new clear implementation inherited pending quiz")
	}
}

func TestInterruptedImplementationCanResumeOriginalQuiz(t *testing.T) {
	for _, status := range []string{"interrupted", "failed"} {
		t.Run(status, func(t *testing.T) {
			m := testQuizModel(t, "")
			original := m.quiz
			m.Update(event("turn/completed", `{"threadId":"thread","turn":{"id":"turn","status":"`+status+`"}}`))
			if m.quiz != original || m.quiz.phase != "awaiting_reply" || m.quizActive() || m.busy {
				t.Fatalf("interrupted implementation lost pending review: phase=%s", m.quiz.phase)
			}
			m.evaluating = true
			m.draft.SetValue("try to continue again")
			cmd := m.finishEvaluation(evaluationDoneMsg{sequence: m.evaluationSequence, prompt: "try to continue again", record: evaluation.Record{Evaluation: evaluation.Body{Verdict: "not_applicable", Summary: "Continuation", Gaps: []evaluation.Gap{}}}})
			if cmd == nil || m.quiz != original || m.quiz.phase != "running" || m.quiz.request.Input != "fix it" {
				t.Fatal("continuation did not reuse the original quiz context")
			}
			m.Update(event("turn/started", `{"threadId":"thread","turn":{"id":"continuation"}}`))
			if err := os.WriteFile(filepath.Join(m.workspace, "main.go"), []byte("changed implementation\n"), 0600); err != nil {
				t.Fatal(err)
			}
			_, generate := m.Update(event("turn/completed", `{"threadId":"thread","turn":{"id":"continuation","status":"completed"}}`))
			if generate == nil {
				t.Fatal("completed continuation did not start the original quiz review")
			}
		})
	}
}
