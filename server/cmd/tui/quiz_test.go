package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"cortisol-server/internal/evaluation"
	"cortisol-server/internal/quiz"
	tea "github.com/charmbracelet/bubbletea"
)

func testQuizModel(t *testing.T, server string) *model {
	t.Helper()
	m := newModel(nil, t.TempDir(), uiOptions{EvaluationServer: server, NoColor: true, ReducedMotion: true})
	m.connected = true
	m.threadID = "thread"
	m.busy = true
	m.lastEvaluation = &promptEvaluation{NeedsQuiz: true, Record: evaluation.Record{Evaluation: evaluation.Body{Verdict: "ambiguous", Summary: "Behavior unspecified", AmbiguityScore: 0.8, Gaps: []evaluation.Gap{{Description: "fallback", Consequence: "different behavior"}}}}}
	cmd := m.prepareQuiz("fix it")
	_, send := m.Update(cmd())
	m.Update(send())
	m.Update(event("turn/started", `{"threadId":"thread","turn":{"id":"turn"}}`))
	return m
}

func twoQuestions() quiz.Result {
	return quiz.Result{Questions: []quiz.Question{
		{ID: "q1", Topic: "First branch", Question: "What happens in the first case?", GapIndices: []int{0}, Evidence: []quiz.Evidence{{FilePath: "main.go", StartLine: 2, EndLine: 2}}},
		{ID: "q2", Topic: "Second branch", Question: "What happens in the second case?", GapIndices: []int{0}, Evidence: []quiz.Evidence{{FilePath: "main.go", StartLine: 4, EndLine: 4}}},
	}}
}

func TestQuizBatchHTTPFlowAndMasking(t *testing.T) {
	gradeCalls := 0
	generationCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/quizzes":
			generationCalls++
			var request quiz.Request
			if json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Files) != 1 || !strings.Contains(request.Files[0].Content, "SECRET_ONE") {
				t.Error("generated code not supplied")
			}
			json.NewEncoder(w).Encode(quiz.Response{Model: "test", Result: twoQuestions()})
		case "/quiz-answers":
			var request quiz.AnswerRequest
			if json.NewDecoder(r.Body).Decode(&request) != nil || request.Validate() != nil {
				t.Error("bad answer request")
			}
			gradeCalls++
			if gradeCalls == 1 {
				w.WriteHeader(503)
				return
			}
			json.NewEncoder(w).Encode(quiz.Grade{Correct: gradeCalls == 4})
		default:
			t.Errorf("unexpected API %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	m := testQuizModel(t, server.URL)
	if err := os.WriteFile(filepath.Join(m.workspace, "main.go"), []byte("visible import\nSECRET_ONE\nvisible helper\nSECRET_TWO\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m.Update(event("item/agentMessage/delta", `{"threadId":"thread","turnId":"turn","itemId":"answer","delta":"SECRET_ONE and SECRET_TWO"}`))
	m.Update(event("item/completed", `{"threadId":"thread","turnId":"turn","item":{"id":"change","type":"fileChange","status":"completed","changes":[{"path":"main.go","kind":{"type":"add"},"diff":"SECRET_ONE"}]}}`))
	m.refresh()
	if strings.Contains(m.viewport.View(), "SECRET_") {
		t.Fatal("stream leaked before quiz")
	}
	_, generate := m.Update(event("turn/completed", `{"threadId":"thread","turn":{"id":"turn","status":"completed"}}`))
	if generate == nil {
		t.Fatal("completion did not start generation")
	}
	_, followup := m.Update(generate())
	if followup != nil || len(m.quiz.result.Questions) != 2 {
		t.Fatal("batch did not terminate generation")
	}
	if generationCalls != 1 || m.quiz.phase != "question" {
		t.Fatal("quiz not ready")
	}
	assertHidden := func(one, two bool) {
		t.Helper()
		view := quizFileView(m.quiz, "main.go")
		if strings.Contains(view, "SECRET_ONE") == one || strings.Contains(view, "SECRET_TWO") == two {
			t.Fatalf("wrong mask: %s", view)
		}
		if !strings.Contains(view, "visible helper") {
			t.Fatal("trivial code hidden")
		}
	}
	assertHidden(true, true)
	if strings.Contains(m.quiz.panel.raw, "SECRET_") {
		t.Fatal("focused question leaked hidden code through context")
	}
	if strings.Contains(m.quiz.panel.raw, "second case") || !strings.Contains(m.quiz.panel.raw, "main.go:2") {
		t.Fatal("not focused on first question")
	}
	answer := func() {
		t.Helper()
		m.draft.SetValue("my answer")
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("answer not sent")
		}
		m.Update(cmd())
	}
	answer() // API failure is not an attempt.
	if m.quiz.attempts != 0 || m.quiz.phase != "question" {
		t.Fatal("API error consumed attempt")
	}
	answer()
	assertHidden(true, true)
	if m.quiz.attempts != 1 {
		t.Fatal("first incorrect attempt missing")
	}
	answer()
	assertHidden(false, true)
	if strings.Contains(m.quiz.panel.raw, "SECRET_TWO") {
		t.Fatal("first reveal leaked the next question's code")
	}
	if m.quiz.phase != "reveal" || m.quiz.index != 0 {
		t.Fatal("advanced without reveal")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.quiz.index != 1 || m.quiz.phase != "question" || !strings.Contains(m.quiz.panel.raw, "main.go:4") {
		t.Fatal("next question not focused")
	}
	assertHidden(false, true)
	answer()
	assertHidden(false, false)
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.quizActive() || m.busy {
		t.Fatal("quiz did not finish")
	}
	for _, item := range m.items {
		if item.withheld {
			t.Fatal("output not released")
		}
	}
}

func TestQuizLateReplyAndApprovalVisibility(t *testing.T) {
	m := testQuizModel(t, "")
	m.Update(event("item/agentMessage/delta", `{"threadId":"thread","turnId":"turn","itemId":"a","delta":"PRIVATE_CODE"}`))
	m.Update(wireMessage{ID: json.RawMessage(`7`), Method: "item/commandExecution/requestApproval", Params: json.RawMessage(`{"command":"pwd"}`)})
	if len(m.requests) != 1 || !strings.Contains(m.requestView(), "Allow") {
		t.Fatal("approval lost")
	}
	m.requests = nil
	m.quiz.phase = "generating"
	m.quiz.panel = &conversationItem{kind: "quizPanel"}
	m.items = append(m.items, m.quiz.panel)
	session := m.quiz
	m.quizEnter("/reveal")
	m.Update(quizGeneratedMsg{session: session, result: twoQuestions()})
	if m.quizActive() {
		t.Fatal("late response restarted quiz")
	}
}

func TestCollectQuizFilesGitBaselineAndRootBoundary(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "--quiet", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", out, err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("existing.go", "preexisting dirty code")
	write("changed.go", "before")
	before, err := snapshotWorkspace(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	write("changed.go", "after shell edit")
	files, err := collectQuizFiles(context.Background(), dir, before, nil)
	if err != nil || len(files) != 1 || files[0].Path != "changed.go" {
		t.Fatalf("wrong changed files: %+v %v", files, err)
	}
	outside := filepath.Join(t.TempDir(), "secret.go")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape.go")); err != nil {
		t.Fatal(err)
	}
	_, err = collectQuizFiles(context.Background(), dir, nil, []string{`[{"path":"escape.go","kind":{"type":"add"}}]`})
	if err == nil {
		t.Fatal("read file outside workspace through symlink")
	}
}

func TestFailedQuizStaysMaskedUntilExplicitReveal(t *testing.T) {
	m := testQuizModel(t, "")
	m.Update(event("item/agentMessage/delta", `{"threadId":"thread","turnId":"turn","itemId":"a","delta":"PRIVATE_CODE"}`))
	m.quizFailure(fmt.Errorf("provider failed"))
	m.refresh()
	if strings.Contains(m.viewport.View(), "PRIVATE_CODE") {
		t.Fatal("failure leaked held output")
	}
	m.quizEnter("/reveal")
	for _, item := range m.items {
		if item.withheld {
			t.Fatal("explicit reveal failed")
		}
	}
}
