package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"cortisol-server/internal/quiz"
)

func TestCapturedOverlappingQuizRenders(t *testing.T) {
	input, err := os.ReadFile("../../internal/quiz/testdata/overlapping-request.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../internal/quiz/testdata/overlapping-response.json")
	if err != nil {
		t.Fatal(err)
	}
	var request quiz.Request
	if err := json.Unmarshal(input, &request); err != nil {
		t.Fatal(err)
	}
	request.MaxQuestions = 4
	var captured quiz.Result
	if err := json.Unmarshal(raw, &captured); err != nil {
		t.Fatal(err)
	}
	captured.Questions = captured.Questions[:4]
	raw, err = json.Marshal(captured)
	if err != nil {
		t.Fatal(err)
	}
	service := quiz.NewService(quizCompletionFunc(func(context.Context, string, string, json.RawMessage) (json.RawMessage, error) {
		return raw, nil
	}), "test")
	server := httptest.NewServer(quiz.NewHandler(service, time.Second))
	defer server.Close()
	var response quiz.Response
	if err := postQuizJSON(context.Background(), server.URL, "quizzes", request, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Questions) != 2 {
		t.Fatalf("got %d questions", len(response.Questions))
	}
	m := testQuizModel(t, server.URL)
	m.width, m.height = 120, 40
	m.quiz.phase = "generating"
	m.quiz.panel = &conversationItem{kind: "quizPanel", done: true}
	m.Update(quizGeneratedMsg{session: m.quiz, request: request, result: response.Result})
	if m.quiz.phase != "question" || m.busy || !strings.Contains(m.viewport.View(), "Question 1/2") || !strings.Contains(m.viewport.View(), "two different clients") {
		t.Fatalf("quiz did not render: phase=%s view=%s", m.quiz.phase, m.viewport.View())
	}
}

func TestQuizAPIErrorExplainsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"code":"cortex_invalid_response","message":"Cortex returned an invalid quiz: invalid code reference"}}`))
	}))
	defer server.Close()
	var response quiz.Response
	err := postQuizJSON(context.Background(), server.URL, "quizzes", struct{}{}, &response)
	if err == nil || !strings.Contains(err.Error(), "invalid code reference") || !strings.Contains(err.Error(), "502") {
		t.Fatalf("lost rejection reason: %v", err)
	}
}
