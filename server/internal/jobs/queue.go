// Package jobs provides bounded in-process work admission and concurrency.
package jobs

import (
	"context"
	"errors"
)

var ErrQueueFull = errors.New("job queue is full")
var ErrClosed = errors.New("job queue is closed")

type outcome[R any] struct {
	result R
	err    error
}
type job[T, R any] struct {
	ctx   context.Context
	input T
	reply chan outcome[R]
}
type Queue[T, R any] struct {
	jobs    chan job[T, R]
	process func(context.Context, T) (R, error)
	cancel  context.CancelFunc
	ctx     context.Context
}

// NewQueue starts workers. Close cancels active work and releases waiters.
// This queue is not durable.
func NewQueue[T, R any](workers, capacity int, process func(context.Context, T) (R, error)) *Queue[T, R] {
	if workers < 1 || capacity < 1 || process == nil {
		panic("workers, capacity, and processor are required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	q := &Queue[T, R]{jobs: make(chan job[T, R], capacity), process: process, cancel: cancel, ctx: ctx}
	for i := 0; i < workers; i++ {
		go q.worker(ctx)
	}
	return q
}
func (q *Queue[T, R]) Close() { q.cancel() }
func (q *Queue[T, R]) Submit(ctx context.Context, input T) (R, error) {
	var zero R
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if q.ctx.Err() != nil {
		return zero, ErrClosed
	}
	j := job[T, R]{ctx: ctx, input: input, reply: make(chan outcome[R], 1)}
	select {
	case q.jobs <- j:
	default:
		return zero, ErrQueueFull
	}
	select {
	case result := <-j.reply:
		return result.result, result.err
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-q.ctx.Done():
		return zero, ErrClosed
	}
}
func (q *Queue[T, R]) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-q.jobs:
			if j.ctx.Err() != nil {
				continue
			}
			workCtx, cancel := context.WithCancel(j.ctx)
			stop := context.AfterFunc(ctx, cancel)
			result, err := q.process(workCtx, j.input)
			stop()
			cancel()
			j.reply <- outcome[R]{result, err}
		}
	}
}
