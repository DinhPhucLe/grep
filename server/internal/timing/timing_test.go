package timing

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPTraceCorrelationAndNoPayloadLogging(t *testing.T) {
	var tui, server bytes.Buffer
	ctx := ForRequest(New(context.Background(), JSONSink(&tui)), "quizzes/start")
	req := httptest.NewRequest("POST", "/quizzes/start", strings.NewReader("PRIVATE_PROMPT"))
	Headers(ctx, req)
	original := log.Writer()
	log.SetOutput(&server)
	defer log.SetOutput(original)
	w := httptest.NewRecorder()
	HTTP("quizzes/start", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Record(r.Context(), "test.stage", time.Now(), nil, nil)
		w.WriteHeader(502)
	}))(w, req)
	Record(ctx, "http.client", time.Now(), nil, nil)
	var client Event
	if json.Unmarshal(tui.Bytes(), &client) != nil || client.TraceID == "" || client.RequestID == "" {
		t.Fatal("missing client trace")
	}
	if w.Header().Get("X-Cortisol-Trace") != client.TraceID || w.Header().Get("X-Cortisol-Request") != client.RequestID {
		t.Fatal("IDs changed at server")
	}
	if !strings.Contains(server.String(), client.RequestID) || !strings.Contains(server.String(), `"outcome":"error"`) || strings.Contains(server.String(), "PRIVATE_PROMPT") {
		t.Fatal("invalid server trace")
	}
}

func TestSeparateRequestIDsAndCancellation(t *testing.T) {
	var output bytes.Buffer
	ctx := New(context.Background(), JSONSink(&output))
	first, second := ForRequest(ctx, "evaluations"), ForRequest(ctx, "quizzes")
	a, b := first.Value(key{}).(trace), second.Value(key{}).(trace)
	if a.id != b.id || a.request == b.request {
		t.Fatal("concurrent requests cannot be distinguished")
	}
	Record(first, "http.client", time.Now(), context.DeadlineExceeded, nil)
	if !strings.Contains(output.String(), `"outcome":"timeout"`) {
		t.Fatal(output.String())
	}
	if validID("arbitrary untrusted input") {
		t.Fatal("accepted invalid ID")
	}
}
