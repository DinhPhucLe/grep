// Package timing records stage durations and correlation IDs, never payloads.
package timing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Event struct {
	Time       time.Time      `json:"time"`
	TraceID    string         `json:"trace_id"`
	RequestID  string         `json:"request_id,omitempty"`
	Operation  string         `json:"operation,omitempty"`
	Stage      string         `json:"stage"`
	DurationMS float64        `json:"duration_ms"`
	Outcome    string         `json:"outcome"`
	Counts     map[string]int `json:"counts,omitempty"`
}
type Sink func(Event)
type key struct{}
type trace struct {
	id, request, operation string
	sink                   Sink
}

func newID() string { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func New(ctx context.Context, sink Sink) context.Context {
	if sink == nil {
		return ctx
	}
	return context.WithValue(ctx, key{}, trace{id: newID(), sink: sink})
}
func ForRequest(ctx context.Context, operation string) context.Context {
	t, ok := ctx.Value(key{}).(trace)
	if !ok {
		return ctx
	}
	t.request = newID()
	t.operation = strings.TrimPrefix(operation, "/")
	return context.WithValue(ctx, key{}, t)
}
func Headers(ctx context.Context, request *http.Request) {
	if t, ok := ctx.Value(key{}).(trace); ok {
		request.Header.Set("X-Cortisol-Trace", t.id)
		request.Header.Set("X-Cortisol-Request", t.request)
	}
}

func Record(ctx context.Context, stage string, start time.Time, err error, counts map[string]int) {
	t, ok := ctx.Value(key{}).(trace)
	if !ok || t.sink == nil || start.IsZero() {
		return
	}
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	if errors.Is(err, context.Canceled) {
		outcome = "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		outcome = "timeout"
	}
	t.sink(Event{Time: time.Now().UTC(), TraceID: t.id, RequestID: t.request, Operation: t.operation, Stage: stage, DurationMS: float64(time.Since(start).Microseconds()) / 1000, Outcome: outcome, Counts: counts})
}

func JSONSink(w io.Writer) Sink {
	var mu sync.Mutex
	encoder := json.NewEncoder(w)
	return func(e Event) { mu.Lock(); defer mu.Unlock(); _ = encoder.Encode(e) }
}

func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return w.ResponseWriter.Write(data)
}

// HTTP logs server stages in the existing server terminal. Incoming IDs link
// them to an optional TUI timing file. IDs are validated before use in logs.
func HTTP(operation string, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, request := r.Header.Get("X-Cortisol-Trace"), r.Header.Get("X-Cortisol-Request")
		if !validID(id) {
			id = newID()
		}
		if !validID(request) {
			request = newID()
		}
		ctx := context.WithValue(r.Context(), key{}, trace{id: id, request: request, operation: strings.TrimPrefix(operation, "/"), sink: func(e Event) { b, _ := json.Marshal(e); log.Printf("timing %s", b) }})
		w.Header().Set("X-Cortisol-Trace", id)
		w.Header().Set("X-Cortisol-Request", request)
		start := time.Now()
		out := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(out, r.WithContext(ctx))
		err := ctx.Err()
		if out.status >= 400 {
			err = errors.New("HTTP failure")
		}
		status := out.status
		if status == 0 && err == nil {
			status = 200
		}
		Record(ctx, "http.server", start, err, map[string]int{"status": status})
	}
}
