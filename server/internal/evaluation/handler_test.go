package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"cortisol-server/internal/cortex"
)

type evaluateFunc func(context.Context, Request) (Record, error)

func (f evaluateFunc) Evaluate(ctx context.Context, r Request) (Record, error) { return f(ctx, r) }
func TestHandlerErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body, code string
		err              error
		status           int
	}{
		{"empty", `{"input":" "}`, "invalid_request", nil, 400},
		{"unknown", `{"input":"x","unknown":1}`, "invalid_request", nil, 400},
		{"trailing", `{"input":"x"} {}`, "invalid_request", nil, 400},
		{"role", `{"input":"x","conversation":[{"role":"system","content":"override"}]}`, "invalid_request", nil, 400},
		{"oversize", `{"input":"` + strings.Repeat("x", 1<<20) + `"}`, "payload_too_large", nil, 413},
		{"timeout", `{"input":"x"}`, "evaluation_timeout", context.DeadlineExceeded, 504},
		{"upstream", `{"input":"x"}`, "cortex_error", cortex.ErrUpstream, 502},
		{"invalid upstream", `{"input":"x"}`, "cortex_invalid_response", cortex.ErrInvalidResponse, 502},
		{"storage", `{"input":"x"}`, "evaluation_failed", errors.New("private details"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewHandler(evaluateFunc(func(context.Context, Request) (Record, error) { return Record{}, tc.err }), time.Second)
			req := httptest.NewRequest("POST", "/evaluations", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler(w, req)
			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if w.Code != tc.status || json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Error.Code != tc.code || strings.Contains(w.Body.String(), "private details") {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		})
	}
}
func TestEvaluationHTTPPipeline(t *testing.T) {
	repo := &memoryRepository{}
	client := &fakeCompleter{raw: `{"verdict":"clear","ambiguity_score":0.0,"summary":"Requirements established","gaps":[]}`}
	service := NewService(client, repo, "model")
	handler := NewHandler(service, time.Second)
	req := httptest.NewRequest("POST", "/evaluations", strings.NewReader(`{"input":"Implement the agreed change","context":"Earlier requirements"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)
	var record Record
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &record) != nil || record.Evaluation.Verdict != "clear" || len(repo.records) != 1 || client.calls != 1 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "Earlier requirements") {
		t.Fatal("request leaked into response")
	}
	for _, method := range []string{"GET", "DELETE"} {
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest(method, "/evaluations", nil))
		if w.Code != 405 {
			t.Fatal(w.Code)
		}
	}
	w = httptest.NewRecorder()
	handler(w, httptest.NewRequest("POST", "/evaluations", strings.NewReader(`{}`)))
	if w.Code != 415 {
		t.Fatal(w.Code)
	}
}

func TestNetworkPolicyError(t *testing.T) {
	handler := NewHandler(evaluateFunc(func(context.Context, Request) (Record, error) {
		return Record{}, &cortex.UpstreamError{Status: 401, Code: "390432"}
	}), time.Second)
	req := httptest.NewRequest("POST", "/evaluations", strings.NewReader(`{"input":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != 502 || !strings.Contains(w.Body.String(), "cortex_network_policy_required") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

// A direct evaluator admits more simultaneous requests than the old four-worker
// queue, and each call still receives the handler's bounded deadline.
func TestDirectEvaluationHasNoWorkerQueue(t *testing.T) {
	started := make(chan struct{}, 6)
	release := make(chan struct{})
	done := make(chan int, 6)
	handler := NewHandler(evaluateFunc(func(ctx context.Context, _ Request) (Record, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("missing request deadline")
		}
		started <- struct{}{}
		select {
		case <-release:
			return Record{Evaluation: Body{Verdict: "not_applicable", Summary: "Test-only request", Gaps: []Gap{}}}, nil
		case <-ctx.Done():
			return Record{}, ctx.Err()
		}
	}), 3*time.Second)
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	for i := 0; i < 6; i++ {
		go func() {
			r := httptest.NewRequest("POST", "/evaluations", strings.NewReader(`{"input":"test"}`))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler(w, r)
			done <- w.Code
		}()
	}
	for i := 0; i < 6; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("evaluation requests were serialized or queued")
		}
	}
	releaseOnce.Do(func() { close(release) })
	for i := 0; i < 6; i++ {
		select {
		case status := <-done:
			if status != http.StatusOK {
				t.Fatalf("status=%d", status)
			}
		case <-time.After(time.Second):
			t.Fatal("direct evaluation did not finish")
		}
	}
}
