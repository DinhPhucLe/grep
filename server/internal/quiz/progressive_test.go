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

func planFixture() []Scope {
	return []Scope{
		{ID: "q1", Topic: "First decision", GapIndices: []int{0}, Evidence: []Evidence{{FilePath: "login.go", StartLine: 2, EndLine: 2}}},
		{ID: "q2", Topic: "Second decision", GapIndices: []int{0}, Evidence: []Evidence{{FilePath: "login.go", StartLine: 3, EndLine: 3}}},
	}
}

func TestProgressiveGenerationOneQuestionPerCall(t *testing.T) {
	plan := planFixture()
	start, _ := json.Marshal(map[string]any{"plan": plan, "question": "What happens in the first case?", "no_questions_reason": ""})
	client := &fakeCompleter{raw: string(start)}
	service := NewService(client, "test")
	first, err := service.Start(context.Background(), exampleRequest())
	if err != nil || client.calls != 1 || first.Question == "" || len(first.Plan) != 2 {
		t.Fatalf("%+v %v", first, err)
	}
	if !strings.Contains(client.prompt, "ONLY the first question") || !json.Valid(client.schema) {
		t.Fatal("incorrect start prompt/schema")
	}
	request := NextRequest{Quiz: exampleRequest(), Plan: first.Plan, Previous: []Question{first.Plan[0].question(first.Question)}}
	client.raw = `{"question":"What happens in the second case?"}`
	next, err := service.Next(context.Background(), request)
	if err != nil || client.calls != 2 || next.Question.ID != "q2" || next.Question.Evidence[0].StartLine != 3 {
		t.Fatalf("%+v %v", next, err)
	}
	var submitted NextRequest
	if json.Unmarshal([]byte(client.input), &submitted) != nil || len(submitted.Previous) != 1 || submitted.Previous[0].Question != first.Question {
		t.Fatal("previous question/context lost")
	}
	request.Previous = append(request.Previous, next.Question)
	if _, err := service.Next(context.Background(), request); err == nil || client.calls != 2 {
		t.Fatal("generated beyond plan")
	}
}

func TestProgressiveValidation(t *testing.T) {
	plan := planFixture()
	for _, raw := range []string{`{}`, `{"plan":[],"question":"","no_questions_reason":""}`, `{"plan":null,"question":"","no_questions_reason":"No code"}`} {
		if _, err := NewService(&fakeCompleter{raw: raw}, "test").Start(context.Background(), exampleRequest()); !errors.Is(err, cortex.ErrInvalidResponse) {
			t.Fatalf("accepted %s: %v", raw, err)
		}
	}
	request := NextRequest{Quiz: exampleRequest(), Plan: plan, Previous: []Question{plan[0].question("First question?")}}
	for _, raw := range []string{`{}`, `{"question":null}`, `{"question":"First question?"}`, `{"question":"new","evidence":[]}`, `{"question":""}`} {
		if _, err := NewService(&fakeCompleter{raw: raw}, "test").Next(context.Background(), request); !errors.Is(err, cortex.ErrInvalidResponse) {
			t.Fatalf("accepted %s: %v", raw, err)
		}
	}
	bad := NextResponse{Question: plan[1].question("Next question?")}
	bad.Question.Evidence = []Evidence{{FilePath: "login.go", StartLine: 1, EndLine: 1}}
	if bad.Validate(request) == nil {
		t.Fatal("accepted changed mask boundaries")
	}
	request.Previous[0].Topic = "changed"
	if request.Validate() == nil {
		t.Fatal("accepted prefix not matching plan")
	}
}

func TestProgressiveHTTPQueues(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"plan": planFixture(), "question": "First question?", "no_questions_reason": ""})
	client := &fakeCompleter{raw: string(raw)}
	service := NewService(client, "test")
	startQueue := jobs.NewQueue(1, 1, service.Start)
	defer startQueue.Close()
	input, _ := json.Marshal(exampleRequest())
	req := httptest.NewRequest("POST", "/quizzes/start", strings.NewReader(string(input)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	NewStartHandler(startQueue, time.Second)(w, req)
	var start StartResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &start) != nil || start.Validate(exampleRequest()) != nil {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	client.raw = `{"question":"Next question?"}`
	nextQueue := jobs.NewQueue(1, 1, service.Next)
	defer nextQueue.Close()
	input, _ = json.Marshal(NextRequest{Quiz: exampleRequest(), Plan: start.Plan, Previous: []Question{start.Plan[0].question(start.Question)}})
	req = httptest.NewRequest("POST", "/quizzes/next", strings.NewReader(string(input)))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	NewNextHandler(nextQueue, time.Second)(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"id":"q2"`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
