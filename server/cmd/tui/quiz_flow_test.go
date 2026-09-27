package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cortisol-server/internal/quiz"
)

func fourQuestions() quiz.Result {
	result := quiz.Result{Questions: []quiz.Question{}}
	for i := 1; i <= 4; i++ {
		result.Questions = append(result.Questions, quiz.Question{ID: fmt.Sprintf("q%d", i), Topic: fmt.Sprintf("Choice %d", i), Question: fmt.Sprintf("What happens in case %d?", i), GapIndices: []int{0}, Evidence: []quiz.Evidence{{FilePath: "main.go", StartLine: i, EndLine: i}}})
	}
	return result
}

func generatedFilesQuizModel(t *testing.T, server string) *model {
	t.Helper()
	m := testQuizModel(t, server)
	if err := os.WriteFile(filepath.Join(m.workspace, "main.go"), []byte("one\ntwo\nthree\nfour\nfive\nsix\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m.Update(event("item/completed", `{"threadId":"thread","turnId":"turn","item":{"id":"change","type":"fileChange","status":"completed","changes":[{"path":"main.go","kind":{"type":"add"}}]}}`))
	return m
}

type quizCompletionFunc func(context.Context, string, string, json.RawMessage) (json.RawMessage, error)

func (f quizCompletionFunc) Complete(ctx context.Context, prompt, input string, schema json.RawMessage) (json.RawMessage, error) {
	return f(ctx, prompt, input, schema)
}

func TestGeneratedQuizRendersAndAnswersFinishWithoutRequests(t *testing.T) {
	var generation, completions atomic.Int32
	service := quiz.NewService(quizCompletionFunc(func(ctx context.Context, prompt, input string, schema json.RawMessage) (json.RawMessage, error) {
		completions.Add(1)
		var request quiz.Request
		if err := json.Unmarshal([]byte(input), &request); err != nil || request.Validate() != nil || request.MaxQuestions != 4 {
			t.Error("invalid whole-quiz request")
		}
		if !strings.Contains(string(schema), `"questions"`) {
			t.Error("model was not asked for all questions")
		}
		return json.Marshal(fourQuestions())
	}), "test")
	generate := quiz.NewHandler(service, time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/quizzes":
			generation.Add(1)
			generate(w, r)
		default:
			t.Errorf("unexpected follow-up endpoint: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	m := generatedFilesQuizModel(t, server.URL)
	completed := event("turn/completed", `{"threadId":"thread","turn":{"id":"turn","status":"completed"}}`)
	_, cmd := m.Update(completed)
	if cmd == nil {
		t.Fatal("generation never started")
	}
	response := cmd()
	_, followup := m.Update(response)
	if followup == nil || m.quiz.phase != "question" || m.busy || !strings.Contains(m.quiz.panel.raw, "Question 1/4") {
		t.Fatal("generated quiz did not render and stop")
	}
	m.Update(followup())
	// Replayed notifications/results must not restart generation or reset progress.
	for i := 0; i < 4; i++ {
		_, followup = m.Update(completed)
		if followup != nil {
			t.Fatal("duplicate turn completion restarted generation")
		}
		_, followup = m.Update(response)
		if followup != nil || m.quiz.index != i {
			t.Fatal("late response restarted quiz")
		}
		if cmd := m.quizEnter("my answer"); cmd == nil {
			t.Fatal("answer did not start grading")
		} else {
			m.Update(cmd())
		}
		if m.quiz.phase != "reveal" {
			t.Fatal("local answer failed to reveal")
		}
		next := m.quizEnter("")
		if i < 3 {
			if next == nil {
				t.Fatal("next question did not open source")
			}
			msg := next()
			if _, ok := msg.(quizSourceOpenedMsg); !ok {
				t.Fatal("advancing scheduled work other than opening source")
			}
			m.Update(msg)
		} else if next != nil {
			t.Fatal("finished quiz scheduled more work")
		}
	}
	if m.quizActive() || m.busy || generation.Load() != 1 || completions.Load() != 1 {
		t.Fatalf("did not terminate: phase=%s generation=%d", m.quiz.phase, generation.Load())
	}
}

func TestQuizGenerationFailureEndsWithoutRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	m := generatedFilesQuizModel(t, server.URL)
	response := m.startQuizGeneration()()
	_, next := m.Update(response)
	if next != nil || m.quizActive() || m.busy || calls.Load() != 1 {
		t.Fatal("generation failure did not return to chat")
	}
	if !strings.Contains(m.quiz.panel.raw, "502") || strings.Contains(m.quiz.panel.raw, "/retry") {
		t.Fatal("failure lost its reason or offered retry")
	}
	if cmd := m.quizEnter("/retry"); cmd != nil {
		t.Fatal("removed retry command started generation")
	}
	m.Update(response)
	if m.quizActive() || calls.Load() != 1 {
		t.Fatal("late failure restarted generation")
	}
}

func TestEmptyQuizOpensReviewWithoutFollowup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(quiz.Response{Result: quiz.Result{Questions: []quiz.Question{}, NoQuestionsReason: "No grounded choices"}})
	}))
	defer server.Close()
	m := generatedFilesQuizModel(t, server.URL)
	_, next := m.Update(m.startQuizGeneration()())
	if next != nil || m.quizActive() || m.busy {
		t.Fatal("empty valid quiz did not terminate")
	}
}
