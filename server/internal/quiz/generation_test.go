package quiz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGenerationAcceptsSupportedQuestionCounts(t *testing.T) {
	for _, count := range []int{0, 1, 2, 4, 5} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			request := exampleRequest()
			request.Files[0].Content = "one\ntwo\nthree\nfour\nfive\nsix\n"
			result := Result{Questions: []Question{}}
			for i := 1; i <= count; i++ {
				result.Questions = append(result.Questions, Question{ID: fmt.Sprintf("q%d", i), Topic: fmt.Sprintf("Choice %d", i), Question: fmt.Sprintf("What happens in case %d?", i), GapIndices: []int{0}, Evidence: []Evidence{{FilePath: "login.go", StartLine: i, EndLine: i}}})
			}
			if count == 0 {
				result.NoQuestionsReason = "No grounded choices"
			}
			raw, _ := json.Marshal(result)
			client := &fakeCompleter{raw: string(raw)}
			input, _ := json.Marshal(request)
			r := httptest.NewRequest("POST", "/quizzes", strings.NewReader(string(input)))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			NewHandler(NewService(client, "test"), time.Second)(w, r)
			if count > 4 {
				if w.Code != 502 || client.calls != 1 {
					t.Fatalf("over-limit quiz: status=%d calls=%d", w.Code, client.calls)
				}
				return
			}
			var response Response
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Result.Validate(request) != nil || len(response.Questions) != count || client.calls != 1 {
				t.Fatalf("status=%d count=%d calls=%d", w.Code, len(response.Questions), client.calls)
			}
		})
	}
}

type completionFunc func(context.Context, string, string, json.RawMessage) (json.RawMessage, error)

func (f completionFunc) Complete(ctx context.Context, prompt, input string, schema json.RawMessage) (json.RawMessage, error) {
	return f(ctx, prompt, input, schema)
}

func TestSingleCompletionTimeoutDoesNotRetry(t *testing.T) {
	calls := 0
	client := completionFunc(func(ctx context.Context, _, _ string, _ json.RawMessage) (json.RawMessage, error) {
		calls++
		<-ctx.Done()
		return nil, ctx.Err()
	})
	input, _ := json.Marshal(exampleRequest())
	r := httptest.NewRequest("POST", "/quizzes", strings.NewReader(string(input)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	NewHandler(NewService(client, "test"), 10*time.Millisecond)(w, r)
	if w.Code != 504 || calls != 1 {
		t.Fatalf("status=%d calls=%d", w.Code, calls)
	}
}
