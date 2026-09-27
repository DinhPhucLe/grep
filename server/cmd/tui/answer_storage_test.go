package main

import (
	"cortisol-server/internal/quiz"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

const testParticipantID = "12345678-1234-4234-8234-123456789abc"

func TestAnswerSaveFailureRetainsDraftAndRetryIsIndividual(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/quiz-answers" || r.Method != "POST" || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		var input map[string]string
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if input["participant_id"] != testParticipantID || input["quiz_id"] != "quiz-run" || input["question_id"] != "q1" || input["answer"] != "my answer" || len(input) != 4 {
			t.Errorf("wrong answer payload: %+v", input)
		}
		calls++
		if calls == 1 {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"answer-id","participant_id":"12345678-1234-4234-8234-123456789abc","quiz_id":"quiz-run","question_id":"q1","status":"ungraded","created_at":"2026-09-27T00:00:00Z"}`))
	}))
	defer server.Close()
	m := generatedFilesQuizModel(t, server.URL)
	m.opts.ParticipantID = testParticipantID
	m.quiz.quizID = "quiz-run"
	m.quiz.result = twoQuestions()
	m.quiz.phase = "question"
	m.quiz.panel = &conversationItem{kind: "quizPanel"}
	m.draft.SetValue("my answer")
	save := m.quizEnter("my answer")
	if save == nil || m.quiz.phase != "saving" || m.quiz.index != 0 {
		t.Fatal("answer was not saved before advancing")
	}
	if cmd := m.quizEnter("duplicate"); cmd != nil {
		t.Fatal("duplicate save scheduled")
	}
	m.Update(save())
	if m.quiz.phase != "question" || m.busy || m.draft.Value() != "my answer" || !strings.Contains(m.quiz.panel.raw, "not saved") {
		t.Fatal("failed save lost draft or claimed success")
	}
	save = m.quizEnter("my answer")
	m.Update(save())
	if m.quiz.phase != "reveal" || m.quiz.index != 0 || m.draft.Value() != "" || !strings.Contains(m.quiz.panel.raw, "Saved") {
		t.Fatal("save acknowledgment did not open continuation")
	}
	if calls != 2 {
		t.Fatalf("got %d calls", calls)
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

func TestParticipantRegistrationPrecedesPromptEvaluation(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("Authorization") != "" {
			t.Error("participant ID became auth")
		}
		switch r.URL.Path {
		case "/participants":
			w.Write([]byte(`{"id":"12345678-1234-4234-8234-123456789abc","created_at":"2026-09-27T00:00:00Z","last_seen_at":"2026-09-27T00:00:00Z"}`))
		case "/evaluations":
			w.Write([]byte(`{"evaluation":{"verdict":"not_applicable","summary":"Confirmation","ambiguity_score":null,"gaps":[]}}`))
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
		}
	}))
	defer server.Close()
	m := newModel(nil, t.TempDir(), uiOptions{EvaluationServer: server.URL, ParticipantID: testParticipantID})
	m.connected = true
	m.draft.SetValue("yes")
	_, evaluate := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_, send := m.Update(evaluate())
	if send == nil || strings.Join(paths, ",") != "/participants,/evaluations" {
		t.Fatalf("wrong registration flow: %v", paths)
	}
}

func TestQuizGenerationCarriesParticipantAndStorageID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request quiz.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.ParticipantID != testParticipantID || len(request.ProjectID) != 64 || request.ThreadID != "thread" || request.TurnID != "turn" {
			t.Errorf("missing quiz provenance: %+v", request)
		}
		json.NewEncoder(w).Encode(quiz.Response{QuizID: "quiz-run", Result: twoQuestions()})
	}))
	defer server.Close()
	m := generatedFilesQuizModel(t, server.URL)
	m.opts.ParticipantID = testParticipantID
	m.Update(m.startQuizGeneration()())
	if m.quiz.quizID != "quiz-run" || m.quiz.phase != "question" {
		t.Fatal("quiz storage ID was lost")
	}
}

func TestRegistrationFailurePreservesPromptWithoutEvaluating(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/participants" {
			t.Errorf("registration failure allowed %s", r.URL.Path)
		}
		w.WriteHeader(503)
		w.Write([]byte(`{"error":{"code":"migration_required","message":"Participant storage migration is required"}}`))
	}))
	defer server.Close()
	m := newModel(nil, t.TempDir(), uiOptions{EvaluationServer: server.URL, ParticipantID: testParticipantID})
	m.connected = true
	m.draft.SetValue("build a search")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_, send := m.Update(cmd())
	if send != nil || m.busy || m.draft.Value() != "build a search" || !strings.Contains(m.status, "migration") {
		t.Fatalf("lost prompt or registration reason: %s", m.status)
	}
}
