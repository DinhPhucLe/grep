package quiz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"cortisol-server/internal/cortex"
)

type concurrentCompleter func(context.Context, string, string, json.RawMessage) (json.RawMessage, error)

func (f concurrentCompleter) Complete(ctx context.Context, prompt, input string, schema json.RawMessage) (json.RawMessage, error) {
	return f(ctx, prompt, input, schema)
}

func concurrentFixture(n int) (Request, []Scope, json.RawMessage) {
	request := exampleRequest()
	request.Files[0].Content = "one\ntwo\nthree\nfour\nfive\nsix\n"
	plan := make([]Scope, 0, n)
	for i := 0; i < n; i++ {
		plan = append(plan, Scope{ID: fmt.Sprintf("q%d", i+1), Topic: fmt.Sprintf("Choice %d", i+1), GapIndices: []int{0}, Evidence: []Evidence{{FilePath: "login.go", StartLine: i + 1, EndLine: i + 1}}})
	}
	reason := ""
	if n == 0 {
		reason = "No consequential decisions"
	}
	raw, _ := json.Marshal(map[string]any{"plan": plan, "no_questions_reason": reason})
	return request, plan, raw
}

func TestConcurrentQuizSixJobsThreeWorkersAndTermination(t *testing.T) {
	request, _, plan := concurrentFixture(6)
	started := make(chan string, 6)
	releases := map[string]chan struct{}{}
	for i := 1; i <= 6; i++ {
		releases[fmt.Sprintf("q%d", i)] = make(chan struct{})
	}
	var calls, active, peak atomic.Int32
	client := concurrentCompleter(func(ctx context.Context, _, input string, _ json.RawMessage) (json.RawMessage, error) {
		calls.Add(1)
		var job struct {
			Scope Scope `json:"scope"`
		}
		if err := json.Unmarshal([]byte(input), &job); err != nil {
			return nil, err
		}
		if job.Scope.ID == "" {
			return plan, nil
		}
		now := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); now > old; old = peak.Load() {
			if peak.CompareAndSwap(old, now) {
				break
			}
		}
		started <- job.Scope.ID
		select {
		case <-releases[job.Scope.ID]:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		raw, _ := json.Marshal(map[string]string{"question": "Explain " + job.Scope.ID + "?"})
		return raw, nil
	})
	generator := NewGenerator(NewService(client, "test"))
	defer generator.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	type outcome struct {
		response Response
		err      error
	}
	done := make(chan outcome, 1)
	go func() { r, e := generator.Submit(ctx, request); done <- outcome{r, e} }()
	take := func() string {
		t.Helper()
		select {
		case id := <-started:
			return id
		case <-ctx.Done():
			t.Fatal("question jobs stalled")
			return ""
		}
	}
	first, second, third := take(), take(), take()
	if active.Load() != 3 {
		t.Fatalf("active=%d; want three", active.Load())
	}
	// Let later work finish while the first two jobs remain blocked.
	close(releases[third])
	for i := 0; i < 3; i++ {
		id := take()
		close(releases[id])
	}
	close(releases[second])
	close(releases[first])
	select {
	case result := <-done:
		if result.err != nil || len(result.response.Questions) != 6 || result.response.Result.Validate(request) != nil {
			t.Fatalf("invalid batch: %+v %v", result.response, result.err)
		}
	case <-ctx.Done():
		t.Fatal("successful jobs did not terminate generation")
	}
	if calls.Load() != 7 || peak.Load() != 3 {
		t.Fatalf("calls=%d peak=%d; want one plan + six jobs and three workers", calls.Load(), peak.Load())
	}
}

func TestConcurrentQuizNoPaddingAndInvalidPlan(t *testing.T) {
	for _, n := range []int{0, 2, 6} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			request, _, plan := concurrentFixture(n)
			var calls atomic.Int32
			generator := NewGenerator(NewService(concurrentCompleter(func(_ context.Context, _, input string, _ json.RawMessage) (json.RawMessage, error) {
				calls.Add(1)
				var job struct {
					Scope Scope `json:"scope"`
				}
				json.Unmarshal([]byte(input), &job)
				if job.Scope.ID == "" {
					return plan, nil
				}
				raw, _ := json.Marshal(map[string]string{"question": "Explain " + job.Scope.ID + "?"})
				return raw, nil
			}), "test"))
			defer generator.Close()
			result, err := generator.Submit(context.Background(), request)
			if err != nil || result.Result.Validate(request) != nil || len(result.Questions) != n || calls.Load() != int32(n+1) {
				t.Fatalf("result=%+v calls=%d err=%v", result, calls.Load(), err)
			}
		})
	}
	request, _, _ := concurrentFixture(6)
	client := &fakeCompleter{raw: `{"plan":null,"no_questions_reason":""}`}
	generator := NewGenerator(NewService(client, "test"))
	defer generator.Close()
	if _, err := generator.Submit(context.Background(), request); !errors.Is(err, cortex.ErrInvalidResponse) || client.calls != 1 {
		t.Fatalf("invalid plan reached workers: %v calls=%d", err, client.calls)
	}
}

func TestConcurrentQuizFailureCancelsOtherWorkers(t *testing.T) {
	request, _, plan := concurrentFixture(6)
	started := make(chan struct{}, 6)
	release := make(chan struct{})
	exited := make(chan struct{}, 6)
	var sequence atomic.Int32
	client := concurrentCompleter(func(ctx context.Context, _, input string, _ json.RawMessage) (json.RawMessage, error) {
		var job struct {
			Scope Scope `json:"scope"`
		}
		json.Unmarshal([]byte(input), &job)
		if job.Scope.ID == "" {
			return plan, nil
		}
		id := sequence.Add(1)
		started <- struct{}{}
		defer func() { exited <- struct{}{} }()
		if id == 1 {
			select {
			case <-release:
				return nil, cortex.ErrUpstream
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		<-ctx.Done()
		return nil, ctx.Err()
	})
	generator := NewGenerator(NewService(client, "test"))
	defer generator.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := generator.Submit(ctx, request); done <- err }()
	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("workers did not start")
		}
	}
	close(release)
	select {
	case err := <-done:
		if !errors.Is(err, cortex.ErrUpstream) {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("failure did not terminate batch")
	}
	for i := 0; i < 3; i++ {
		select {
		case <-exited:
		case <-ctx.Done():
			t.Fatal("failed batch leaked active workers")
		}
	}
}

func TestConcurrentQuizCancellationAndInvalidQuestions(t *testing.T) {
	for _, mode := range []string{"cancel", "invalid", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			request, _, plan := concurrentFixture(6)
			started := make(chan struct{}, 6)
			generator := NewGenerator(NewService(concurrentCompleter(func(ctx context.Context, _, input string, _ json.RawMessage) (json.RawMessage, error) {
				var job struct {
					Scope Scope `json:"scope"`
				}
				json.Unmarshal([]byte(input), &job)
				if job.Scope.ID == "" {
					return plan, nil
				}
				started <- struct{}{}
				if mode == "cancel" {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				if mode == "duplicate" {
					return json.RawMessage(`{"question":"Same question?"}`), nil
				}
				return json.RawMessage(`{"question":""}`), nil
			}), "test"))
			defer generator.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := generator.Submit(ctx, request); done <- err }()
			if mode == "cancel" {
				select {
				case <-started:
					cancel()
				case <-time.After(3 * time.Second):
					t.Fatal("no work started")
				}
			}
			select {
			case err := <-done:
				if mode == "cancel" && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				if mode != "cancel" && !errors.Is(err, cortex.ErrInvalidResponse) {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("batch hung or retried indefinitely")
			}
		})
	}
}

func TestConcurrentQuizBatchesShareWorkerLimit(t *testing.T) {
	request, _, plan := concurrentFixture(3)
	started := make(chan struct{}, 6)
	release := make(chan struct{})
	var active, peak atomic.Int32
	client := concurrentCompleter(func(ctx context.Context, _, input string, _ json.RawMessage) (json.RawMessage, error) {
		var job struct {
			Scope Scope `json:"scope"`
		}
		json.Unmarshal([]byte(input), &job)
		if job.Scope.ID == "" {
			return plan, nil
		}
		now := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); now > old; old = peak.Load() {
			if peak.CompareAndSwap(old, now) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		raw, _ := json.Marshal(map[string]string{"question": "Explain " + job.Scope.ID + "?"})
		return raw, nil
	})
	generator := NewGenerator(NewService(client, "test"))
	defer generator.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := generator.Submit(ctx, request); done <- err }()
	}
	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("workers stalled")
		}
	}
	close(release)
	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("batches did not finish")
		}
	}
	if peak.Load() != 3 {
		t.Fatalf("worker limit is per request instead of shared: peak=%d", peak.Load())
	}
}

func TestConcurrentQuizDeadlineAndShutdown(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(fmt.Sprint(shutdown), func(t *testing.T) {
			request, _, plan := concurrentFixture(6)
			started := make(chan struct{}, 6)
			generator := NewGenerator(NewService(concurrentCompleter(func(ctx context.Context, _, input string, _ json.RawMessage) (json.RawMessage, error) {
				var job struct {
					Scope Scope `json:"scope"`
				}
				json.Unmarshal([]byte(input), &job)
				if job.Scope.ID == "" {
					return plan, nil
				}
				started <- struct{}{}
				<-ctx.Done()
				return nil, ctx.Err()
			}), "test"))
			defer generator.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := generator.Submit(ctx, request); done <- err }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("work did not start")
			}
			if shutdown {
				generator.Close()
			}
			select {
			case err := <-done:
				if err == nil || (!shutdown && !errors.Is(err, context.DeadlineExceeded)) {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("deadline/shutdown left batch waiting")
			}
		})
	}
}
