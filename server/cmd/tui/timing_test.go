package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cortisol-server/internal/evaluation"
	"cortisol-server/internal/timing"
)

func TestTUITimingCorrelatesHTTPAndCodex(t *testing.T) {
	var events []timing.Event
	m := newModel(nil, t.TempDir(), uiOptions{})
	m.timingSink = func(e timing.Event) { events = append(events, e) }
	m.promptTrace = timing.New(context.Background(), m.timingSink)
	var trace, request string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trace, request = r.Header.Get("X-Cortisol-Trace"), r.Header.Get("X-Cortisol-Request")
		w.WriteHeader(503)
	}))
	defer server.Close()
	if _, err := fetchEvaluation(m.traceContext(), server.URL, evaluation.Request{Input: "private prompt"}); err == nil {
		t.Fatal("expected failure")
	}
	m.codexStarted = time.Now()
	m.recordCodexEnd(nil)
	if len(events) != 2 || trace == "" || request == "" || events[0].TraceID != trace || events[0].RequestID != request || events[0].Outcome != "error" || events[1].TraceID != trace || events[1].Stage != "codex.turn" {
		t.Fatalf("incorrect correlated timings: %+v", events)
	}
	m.recordCodexEnd(nil)
	if len(events) != 2 {
		t.Fatal("duplicate turn completion timing")
	}
}
