package main

import (
	"context"
	"cortisol-server/internal/quiz"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

const testUserID = "66f600000000000000000001"

func TestAnswerSaveFailureRetainsDraftAndRetryIsIndividual(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/quiz-answers" || r.Method != "POST" || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		var input struct {
			UserID     string  `json:"user_id"`
			QuizID     string  `json:"quiz_id"`
			QuestionID string  `json:"question_id"`
			Answer     string  `json:"answer"`
			Graded     float64 `json:"graded"`
			Reasoning  string  `json:"reasoning"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if input.UserID != testUserID || input.QuizID != "quiz-run" || input.QuestionID != "q1" || input.Answer != "my answer" || input.Graded != 0.5 || input.Reasoning != "Missing the fallback." {
			t.Errorf("wrong answer payload: %+v", input)
		}
		calls++
		if calls == 1 {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"answer-id","user_id":"66f600000000000000000001","quiz_id":"quiz-run","question_id":"q1","status":"graded","graded":0.5,"created_at":"2026-09-27T00:00:00Z"}`))
	}))
	defer server.Close()
	m := generatedFilesQuizModel(t, server.URL)
	m.opts.UserID = testUserID
	m.quiz.quizID = "quiz-run"
	m.quiz.result = twoQuestions()
	m.quiz.phase = "question"
	m.quiz.panel = &conversationItem{kind: "quizPanel"}
	m.draft.SetValue("my answer")
	grades := 0
	m.grader = func(context.Context, string, quiz.Question, []quiz.File, string) (answerGrade, error) {
		grades++
		return answerGrade{Accuracy: 0.5, Explanation: "Missing the fallback."}, nil
	}
	grade := m.quizEnter("my answer")
	if grade == nil || m.quiz.phase != "grading" || m.quiz.index != 0 || calls != 0 {
		t.Fatal("answer was sent before grading")
	}
	_, save := m.Update(grade())
	if save == nil || m.quiz.phase != "saving" {
		t.Fatal("grade did not start save")
	}
	if cmd := m.quizEnter("duplicate"); cmd != nil {
		t.Fatal("duplicate save scheduled")
	}
	m.Update(save())
	if m.quiz.phase != "question" || m.busy || m.draft.Value() != "my answer" || !strings.Contains(m.quiz.panel.raw, "not saved") {
		t.Fatal("failed save lost draft or claimed success")
	}
	save = m.quizEnter("my answer")
	if save == nil || m.quiz.phase != "saving" || grades != 1 {
		t.Fatal("retry regraded or did not start save")
	}
	m.Update(save())
	if m.quiz.phase != "reveal" || m.quiz.index != 0 || m.draft.Value() != "" || !strings.Contains(m.quiz.panel.raw, "Answer saved") {
		t.Fatal("save acknowledgment did not open continuation")
	}
	if calls != 2 || grades != 1 || !strings.Contains(m.quiz.panel.raw, "Accuracy: 0.50") {
		t.Fatalf("got %d calls and %d grades", calls, grades)
	}
}

func TestStaleAnswerSaveCannotChangeNewQuiz(t *testing.T) {
	m := generatedFilesQuizModel(t, "")
	old := m.quiz
	old.phase = "saving"
	m.quiz = &quizSession{phase: "question", index: 1}
	m.draft.SetValue("new draft")
	m.Update(quizAnswerSavedMsg{session: old, index: 0, answer: "old answer"})
	if m.quiz.phase != "question" || m.quiz.index != 1 || m.draft.Value() != "new draft" {
		t.Fatal("late save changed new quiz")
	}
}

func TestPromptEvaluationDoesNotRegisterIdentity(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("Authorization") != "" {
			t.Error("user ID became auth")
		}
		switch r.URL.Path {
		case "/evaluations":
			w.Write([]byte(`{"evaluation":{"verdict":"not_applicable","summary":"Confirmation","ambiguity_score":null,"gaps":[]}}`))
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
		}
	}))
	defer server.Close()
	m := newModel(nil, t.TempDir(), uiOptions{EvaluationServer: server.URL, UserID: testUserID})
	m.connected = true
	m.draft.SetValue("yes")
	_, evaluate := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_, send := m.Update(evaluate())
	if send == nil || strings.Join(paths, ",") != "/evaluations" {
		t.Fatalf("evaluation should run directly: %v", paths)
	}
}

func TestQuizGenerationCarriesUserAndStorageID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request quiz.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.UserID != testUserID || request.ThreadID != "thread" || request.TurnID != "turn" {
			t.Errorf("missing quiz provenance: %+v", request)
		}
		json.NewEncoder(w).Encode(quiz.Response{QuizID: "quiz-run", Result: twoQuestions()})
	}))
	defer server.Close()
	m := generatedFilesQuizModel(t, server.URL)
	m.opts.UserID = testUserID
	m.Update(m.startQuizGeneration()())
	if m.quiz.quizID != "quiz-run" || m.quiz.phase != "question" {
		t.Fatal("quiz storage ID was lost")
	}
}
