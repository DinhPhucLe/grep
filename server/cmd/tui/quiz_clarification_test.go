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
	"cortisol-server/internal/timing"
	tea "github.com/charmbracelet/bubbletea"
)

func TestClarificationOnlyTurnReturnsToChatWithoutQuizAPI(t *testing.T) {
	var quizCalls atomic.Int32
	evaluations := make(chan evaluation.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/evaluations" {
			quizCalls.Add(1)
			w.WriteHeader(500)
			return
		}
		var input evaluation.Request
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		evaluations <- input
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(evaluation.Record{Evaluation: evaluation.Body{Verdict: "clear", Summary: "The user selected email/password", AmbiguityScore: 0.1, Gaps: []evaluation.Gap{}}})
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
	if quizCalls.Load() != 0 {
		t.Fatal("clarification-only turn called quiz API")
	}
	if m.draft.Value() != "Email and password" || m.draft.Placeholder != "Ask Codex…" {
		t.Fatal("draft/composer not restored")
	}
	if !strings.Contains(m.viewport.View(), clarification) {
		t.Fatal("clarification is still hidden")
	}
	for _, item := range m.items {
		if item.quizOwner == m.quiz && item.withheld {
			t.Fatal("held output was not released")
		}
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
	if next != nil || m.quiz.phase != "failed" || calls.Load() != 0 {
		t.Fatal("collection error was treated as normal empty output")
	}
	if !strings.Contains(m.quiz.panel.raw, "not text") {
		t.Fatal("collection failure lost its reason")
	}
	for _, item := range m.items {
		if item.kind == "agentMessage" && !item.withheld {
			t.Fatal("failed collection released held output")
		}
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
