package jobs

import (
	"context"
	"errors"
	"strings"
)

var ErrQueueFull = errors.New("job queue is full")

type Result struct {
	Output string `json:"output"`
}

type job struct {
	ctx   context.Context
	input string
	reply chan Result
}

type Queue struct {
	jobs chan job
}

// NewQueue starts workers that live for the lifetime of the server process.
func NewQueue(workers, capacity int) *Queue {
	if workers < 1 || capacity < 1 {
		panic("workers and capacity must be positive")
	}
	q := &Queue{jobs: make(chan job, capacity)}
	for i := 0; i < workers; i++ {
		go q.worker()
	}
	return q
}

// Submit queues work without waiting for space, then waits for its result.
func (q *Queue) Submit(ctx context.Context, input string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	j := job{ctx: ctx, input: input, reply: make(chan Result, 1)}

	select {
	case q.jobs <- j:
		// Job successfully queued.
	default:
		// Job queue is full.
		return Result{}, ErrQueueFull
	}

	select {
	case result := <-j.reply:
		// Job completed successfully.
		return result, nil
	case <-ctx.Done():
		// Context was done before job completed.
		return Result{}, ctx.Err()
	}
}

func (q *Queue) worker() {
	for j := range q.jobs {
		if j.ctx.Err() != nil {
			// Skip jobs with canceled context.
			continue
		}
		// Demo workload: replace this with the actual job processing.
		j.reply <- Result{Output: strings.ToUpper(j.input)}
	}
}
