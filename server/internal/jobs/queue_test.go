package jobs

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestQueueErrorAndCancellation(t *testing.T) {
	want := errors.New("processor failed")
	q := NewQueue(1, 1, func(context.Context, string) (string, error) { return "", want })
	defer q.Close()
	if _, err := q.Submit(context.Background(), "x"); !errors.Is(err, want) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := q.Submit(ctx, "x"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestQueueSaturation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	q := NewQueue(1, 1, func(context.Context, string) (string, error) { close(started); <-release; return "done", nil })
	defer q.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := q.Submit(ctx, "first"); done <- err }()
	<-started
	canceled, stop := context.WithCancel(ctx)
	stop()
	q.jobs <- job[string, string]{ctx: canceled} // Occupy the waiting slot deterministically.
	if _, err := q.Submit(ctx, "overflow"); !errors.Is(err, ErrQueueFull) {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCloseReleasesSubmitters(t *testing.T) {
	started := make(chan struct{})
	finished := make(chan struct{})
	q := NewQueue(1, 1, func(ctx context.Context, input string) (string, error) {
		close(started)
		<-ctx.Done()
		close(finished)
		return "", ctx.Err()
	})
	done := make(chan error, 1)
	go func() { _, err := q.Submit(context.Background(), "work"); done <- err }()
	<-started
	q.Close()
	select {
	case err := <-done:
		if !errors.Is(err, ErrClosed) && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("submit hung after close")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("worker not canceled")
	}
	if _, err := q.Submit(context.Background(), "late"); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}
