package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"cortisol-server/internal/quiz"
)

func sixQuestions() quiz.Result {
	result := quiz.Result{Questions: []quiz.Question{}}
	for i := 1; i <= 6; i++ {
		result.Questions = append(result.Questions, quiz.Question{ID: fmt.Sprintf("q%d", i), Topic: fmt.Sprintf("Choice %d", i), Question: fmt.Sprintf("What happens in case %d?", i), GapIndices: []int{0}, Evidence: []quiz.Evidence{{FilePath: "main.go", StartLine: i, EndLine: i}}})
	}
	return result
}

func batchQuizModel(t *testing.T, server string) *model {
	t.Helper()
	m := testQuizModel(t, server)
	if err := os.WriteFile(filepath.Join(m.workspace, "main.go"), []byte("one\ntwo\nthree\nfour\nfive\nsix\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m.Update(event("item/completed", `{"threadId":"thread","turnId":"turn","item":{"id":"change","type":"fileChange","status":"completed","changes":[{"path":"main.go","kind":{"type":"add"}}]}}`))
	return m
}

func TestSuccessfulBatchRendersOnceAndSixAnswersTerminate(t *testing.T) {
	var generation, grading atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/quizzes":
			generation.Add(1)
			json.NewEncoder(w).Encode(quiz.Response{Model: "test", Result: sixQuestions()})
		case "/quiz-answers":
			grading.Add(1)
			json.NewEncoder(w).Encode(quiz.Grade{Correct: true})
		default:
			t.Errorf("unexpected follow-up endpoint: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	m := batchQuizModel(t, server.URL)
	completed := event("turn/completed", `{"threadId":"thread","turn":{"id":"turn","status":"completed"}}`)
	_, cmd := m.Update(completed)
	if cmd == nil {
		t.Fatal("generation never started")
	}
	response := cmd()
	_, followup := m.Update(response)
	if followup != nil || m.quiz.phase != "question" || m.busy || !strings.Contains(m.quiz.panel.raw, "Question 1/6") {
		t.Fatal("successful batch did not render and stop")
	}
	// Replayed notifications/results must not start another batch or reset progress.
	for i := 0; i < 6; i++ {
		_, followup = m.Update(completed)
		if followup != nil {
			t.Fatal("duplicate turn completion restarted generation")
		}
		_, followup = m.Update(response)
		if followup != nil || m.quiz.index != i {
			t.Fatal("late response restarted quiz")
		}
		grade := m.quizEnter("my answer")
		if grade == nil {
			t.Fatal("answer not dispatched")
		}
		_, followup = m.Update(grade())
		if followup != nil {
			t.Fatal("grading scheduled unexpected work")
		}
		if m.quiz.phase != "reveal" {
			t.Fatal("successful grading failed to reveal")
		}
		if next := m.quizEnter(""); next != nil {
			t.Fatal("advancing scheduled more generation")
		}
	}
	if m.quizActive() || m.busy || generation.Load() != 1 || grading.Load() != 6 {
		t.Fatalf("did not terminate: phase=%s generation=%d grading=%d", m.quiz.phase, generation.Load(), grading.Load())
	}
}

func TestBatchFailureWaitsForExplicitRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			json.NewEncoder(w).Encode(quiz.Response{Result: quiz.Result{Questions: nil}})
			return
		}
		json.NewEncoder(w).Encode(quiz.Response{Result: sixQuestions()})
	}))
	defer server.Close()
	m := batchQuizModel(t, server.URL)
	cmd := m.startQuizGeneration()
	_, next := m.Update(cmd())
	if next != nil || m.quiz.phase != "failed" || m.busy || calls.Load() != 1 {
		t.Fatal("invalid success response looped or left UI busy")
	}
	if cmd := m.quizEnter("answer"); cmd != nil {
		t.Fatal("failed quiz retried without /retry")
	}
	retry := m.quizEnter("/retry")
	if retry == nil {
		t.Fatal("explicit retry missing")
	}
	_, next = m.Update(retry())
	if next != nil || m.quiz.phase != "question" || calls.Load() != 2 {
		t.Fatal("retry did not terminate successfully")
	}
}

func TestEmptyBatchOpensReviewWithoutFollowup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(quiz.Response{Result: quiz.Result{Questions: []quiz.Question{}, NoQuestionsReason: "No grounded choices"}})
	}))
	defer server.Close()
	m := batchQuizModel(t, server.URL)
	_, next := m.Update(m.startQuizGeneration()())
	if next != nil || m.quizActive() || m.busy {
		t.Fatal("empty valid quiz did not terminate")
	}
}
