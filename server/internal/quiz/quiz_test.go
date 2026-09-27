package quiz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cortisol-server/internal/cortex"
	"cortisol-server/internal/evaluation"
	"cortisol-server/internal/jobs"
)

func exampleRequest() Request {
	return Request{Input: "fix login", Evaluation: evaluation.Body{
		Verdict: "ambiguous", Summary: "Failure handling is unspecified", AmbiguityScore: 0.7,
		Gaps: []evaluation.Gap{{Description: "Unknown user behavior", Consequence: "Might reveal account existence"}},
	}, Files: []File{{Path: "login.go", Content: "package login\n// private implementation\nfunc message() string { return \"Invalid credentials\" }\n"}}}
}

func exampleResult() Result {
	return Result{Questions: []Question{{ID: "q1", Topic: "Account enumeration", Question: "How would the response for an unknown user compare with a wrong password?", GapIndices: []int{0}, Evidence: []Evidence{{FilePath: "login.go", StartLine: 3, EndLine: 3}}}}}
}

type fakeCompleter struct {
	raw, input, prompt string
	schema             json.RawMessage
	err                error
	calls              int
}

func (f *fakeCompleter) Complete(_ context.Context, prompt, input string, schema json.RawMessage) (json.RawMessage, error) {
	f.calls++
	f.input, f.prompt, f.schema = input, prompt, schema
	return json.RawMessage(f.raw), f.err
}

func TestRequestValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Request)
	}{
		{"threshold", func(r *Request) { r.Evaluation.AmbiguityScore = 0.3 }},
		{"clear", func(r *Request) { r.Evaluation.Verdict = "clear"; r.Evaluation.Gaps = []evaluation.Gap{} }},
		{"missing gaps", func(r *Request) { r.Evaluation.Gaps = nil }},
		{"no files", func(r *Request) { r.Files = nil }},
		{"empty file", func(r *Request) { r.Files[0].Content = " " }},
		{"duplicate files", func(r *Request) { r.Files = append(r.Files, r.Files[0]) }},
		{"parent path", func(r *Request) { r.Files[0].Path = "../secret" }},
		{"absolute path", func(r *Request) { r.Files[0].Path = "/tmp/file" }},
		{"low cap", func(r *Request) { r.MaxQuestions = 3 }},
		{"high cap", func(r *Request) { r.MaxQuestions = 7 }},
		{"file too large", func(r *Request) { r.Files[0].Content = strings.Repeat("x", 128001) }},
		{"aggregate too large", func(r *Request) {
			r.Files = []File{{"a", strings.Repeat("x", 128000)}, {"b", strings.Repeat("x", 128000)}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := exampleRequest()
			tc.change(&r)
			client := &fakeCompleter{}
			if _, err := NewService(client, "test").Generate(context.Background(), r); err == nil || client.calls != 0 {
				t.Fatal("invalid request reached Cortex")
			}
		})
	}
	for _, cap := range []int{0, 4, 5, 6} {
		r := exampleRequest()
		r.MaxQuestions = cap
		if err := r.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestQuestionValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Result)
	}{
		{"unknown file", func(r *Result) { r.Questions[0].Evidence[0].FilePath = "invented.go" }},
		{"out of range line", func(r *Result) { r.Questions[0].Evidence[0].EndLine = 4 }},
		{"zero line", func(r *Result) { r.Questions[0].Evidence[0].StartLine = 0 }},
		{"invalid gap", func(r *Result) { r.Questions[0].GapIndices = []int{1} }},
		{"missing gap", func(r *Result) { r.Questions[0].GapIndices = nil }},
		{"missing evidence", func(r *Result) { r.Questions[0].Evidence = nil }},
		{"duplicate", func(r *Result) { q := r.Questions[0]; q.ID = "q2"; r.Questions = append(r.Questions, q) }},
		{"wrong id", func(r *Result) { r.Questions[0].ID = "q8" }},
		{"empty", func(r *Result) { r.Questions = []Question{} }},
		{"contradictory reason", func(r *Result) { r.NoQuestionsReason = "No evidence" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := exampleResult()
			tc.change(&r)
			if r.Validate(exampleRequest()) == nil {
				t.Fatal("invalid question accepted")
			}
		})
	}
	if err := (Result{Questions: []Question{}, NoQuestionsReason: "No choice is evidenced"}).Validate(exampleRequest()); err != nil {
		t.Fatal(err)
	}
	r := exampleResult()
	for n := 2; n <= 7; n++ {
		q := exampleResult().Questions[0]
		q.ID = fmt.Sprintf("q%d", n)
		q.Topic = q.ID
		q.Question = q.ID
		r.Questions = append(r.Questions, q)
	}
	if r.Validate(exampleRequest()) == nil {
		t.Fatal("seven questions accepted")
	}
	r.Questions = r.Questions[:5]
	input := exampleRequest()
	input.MaxQuestions = 4
	if r.Validate(input) == nil {
		t.Fatal("caller cap ignored")
	}
}

func TestHTTPGenerationPipeline(t *testing.T) {
	raw, _ := json.Marshal(exampleResult())
	client := &fakeCompleter{raw: string(raw)}
	service := NewService(client, "configured-claude-model")
	queue := jobs.NewQueue(1, 1, service.Generate)
	defer queue.Close()
	input := exampleRequest()
	input.Context = "Prior requirements"
	input.Conversation = []evaluation.Message{{Role: "user", Content: "Use generic errors"}}
	encoded, _ := json.Marshal(input)
	req := httptest.NewRequest("POST", "/quizzes", strings.NewReader(string(encoded)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	NewHandler(queue, time.Second)(w, req)
	var got Response
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || len(got.Questions) != 1 || got.Model != "configured-claude-model" || got.PromptVersion != PromptVersion {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var sent Request
	if json.Unmarshal([]byte(client.input), &sent) != nil || sent.MaxQuestions != 6 || sent.Files[0].Content != input.Files[0].Content || sent.Conversation[0].Content != input.Conversation[0].Content || sent.Context != input.Context {
		t.Fatal("Cortex request lost implementation/context")
	}
	if client.prompt != systemPrompt || !json.Valid(client.schema) {
		t.Fatal("quiz prompt/schema not sent")
	}
	if strings.Contains(w.Body.String(), "private implementation") || strings.Contains(w.Body.String(), "Invalid credentials") || strings.Contains(w.Body.String(), "fix login") {
		t.Fatal("implementation/request echoed")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("response can be cached")
	}
}

func TestServiceRejectsMalformedOutput(t *testing.T) {
	valid, _ := json.Marshal(exampleResult())
	for _, raw := range []string{`null`, `{}`, `{"questions":[],"no_questions_reason":null}`, string(valid) + ` {}`, strings.Replace(string(valid), `"topic":`, `"answer":"leak","topic":`, 1), strings.Replace(string(valid), `"start_line":3`, `"start_line":100`, 1)} {
		_, err := NewService(&fakeCompleter{raw: raw}, "test").Generate(context.Background(), exampleRequest())
		if !errors.Is(err, cortex.ErrInvalidResponse) {
			t.Fatalf("accepted malformed output: %s (%v)", raw, err)
		}
	}
	_, err := NewService(&fakeCompleter{err: context.DeadlineExceeded}, "test").Generate(context.Background(), exampleRequest())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

type submitFunc func(context.Context, Request) (Response, error)

func (f submitFunc) Submit(ctx context.Context, r Request) (Response, error) { return f(ctx, r) }

func TestHandlerFailures(t *testing.T) {
	input, _ := json.Marshal(exampleRequest())
	for _, tc := range []struct {
		name, method, media, body string
		err                       error
		status                    int
	}{
		{"method", "GET", "application/json", string(input), nil, 405},
		{"media", "POST", "text/plain", string(input), nil, 415},
		{"invalid", "POST", "application/json", `{}`, nil, 400},
		{"unknown", "POST", "application/json", `{"unknown":true}`, nil, 400},
		{"trailing", "POST", "application/json", string(input) + ` {}`, nil, 400},
		{"oversize", "POST", "application/json", `{"input":"` + strings.Repeat("x", 1<<20) + `"}`, nil, 413},
		{"timeout", "POST", "application/json", string(input), context.DeadlineExceeded, 504},
		{"full", "POST", "application/json", string(input), jobs.ErrQueueFull, 503},
		{"closed", "POST", "application/json", string(input), jobs.ErrClosed, 503},
		{"upstream", "POST", "application/json", string(input), cortex.ErrUpstream, 502},
		{"bad output", "POST", "application/json", string(input), cortex.ErrInvalidResponse, 502},
		{"internal", "POST", "application/json", string(input), errors.New("private token"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			handler := NewHandler(submitFunc(func(context.Context, Request) (Response, error) { called = true; return Response{}, tc.err }), time.Second)
			req := httptest.NewRequest(tc.method, "/quizzes", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.media)
			w := httptest.NewRecorder()
			handler(w, req)
			if w.Code != tc.status || strings.Contains(w.Body.String(), "private token") {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if tc.status < 500 && called {
				t.Fatal("invalid HTTP input reached queue")
			}
		})
	}
}
