package quiz

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cortisol-server/internal/cortex"
	"cortisol-server/internal/jobs"
)

func TestGradingContract(t *testing.T) {
	request := AnswerRequest{Quiz: exampleRequest(), Questions: exampleResult(), QuestionID: "q1", Answer: "Both use the same generic error"}
	for _, raw := range []string{`{"correct":true}`, `{"correct":false}`, `{}`, `{"correct":null}`, `{"correct":true,"answer":"secret"}`} {
		client := &fakeCompleter{raw: raw}
		_, err := NewService(client, "test").Grade(context.Background(), request)
		valid := raw == `{"correct":true}` || raw == `{"correct":false}`
		if valid && err != nil || !valid && !errors.Is(err, cortex.ErrInvalidResponse) {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	client := &fakeCompleter{raw: `{"correct":true}`}
	service := NewService(client, "test")
	queue := jobs.NewQueue(1, 1, service.Grade)
	defer queue.Close()
	data, _ := json.Marshal(request)
	r := httptest.NewRequest("POST", "/quiz-answers", strings.NewReader(string(data)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	NewAnswerHandler(queue, time.Second)(w, r)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"correct":true}` {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	request.QuestionID = "q999"
	if request.Validate() == nil {
		t.Fatal("unknown question graded")
	}
}

func TestRejectOverlappingQuestionEvidence(t *testing.T) {
	r := exampleResult()
	q := r.Questions[0]
	q.ID = "q2"
	q.Topic = "Other"
	q.Question = "Another question?"
	r.Questions = append(r.Questions, q)
	if r.Validate(exampleRequest()) == nil {
		t.Fatal("overlapping reveal ranges accepted")
	}
}
