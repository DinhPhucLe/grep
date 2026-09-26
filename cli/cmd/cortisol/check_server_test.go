package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCheckServerRoundTrip(t *testing.T) {
	input := "connection \"quoted\" 日本語\nnext line"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/jobs" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %s %v", r.Method, r.URL.Path, r.Header)
		}
		var request struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Input != input {
			t.Errorf("input changed: %q", request.Input)
		}
		json.NewEncoder(w).Encode(map[string]string{"output": strings.ToUpper(request.Input)})
	}))
	defer server.Close()
	var stdout, stderr bytes.Buffer
	code := runCheckServer([]string{"--server", server.URL, "--input", input}, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit %d, stderr: %s", code, &stderr)
	}
	if !strings.Contains(stdout.String(), "HTTP 200 OK") || !strings.Contains(stdout.String(), "Connection OK") || !strings.Contains(stdout.String(), `\nNEXT LINE`) {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}

func TestCheckServerErrors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body, want string
	}{
		{"rejected", 400, "input must not be empty", "HTTP 400"},
		{"busy", 503, "job queue is full", "job queue is full"},
		{"redirect", 307, "redirected", "HTTP 307"},
		{"not_json", 200, "not JSON", "expected a JSON job response"},
		{"missing", 200, `{}`, "missing an output string"},
		{"null", 200, `{"output":null}`, "missing an output string"},
		{"wrong_type", 200, `{"output":1}`, "expected a JSON job response"},
		{"extra_json", 200, `{"output":"ok"}{}`, "expected a JSON job response"},
		{"oversized", 200, strings.Repeat("x", maxCheckResponseBytes+1), "exceeds 1 MiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "/unexpected")
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer server.Close()
			var stdout, stderr bytes.Buffer
			code := runCheckServer([]string{"--server", server.URL}, &stdout, &stderr)
			if code != 1 || !strings.Contains(stderr.String(), tc.want) || strings.Contains(stdout.String(), "Connection OK") {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, &stdout, &stderr)
			}
		})
	}
}

func TestCheckServerTimeoutAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer server.Close()
	var stdout, stderr bytes.Buffer
	if code := runCheckServer([]string{"--server", server.URL, "--timeout", "50ms"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "timed out") {
		t.Fatalf("exit %d, error %q", code, &stderr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := checkServer(ctx, &http.Client{Timeout: time.Second}, server.URL+"/jobs", "test", io.Discard); err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestCheckServerUnreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()
	var stdout, stderr bytes.Buffer
	if code := runCheckServer([]string{"--server", server.URL}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "could not reach") {
		t.Fatalf("exit %d, stderr %q", code, &stderr)
	}
}

func TestCheckServerOptions(t *testing.T) {
	for _, args := range [][]string{
		{"--server", "localhost:8080"}, {"--server", "file:///tmp/file"},
		{"--server", "http://localhost:8080?foo=bar"}, {"--server", "http://user:pass@localhost:8080"},
		{"--input", " "}, {"--timeout", "0"}, {"--timeout", "-1s"}, {"--timeout", "oops"},
		{"--unknown"}, {"unexpected"},
	} {
		if code := runCheckServer(args, io.Discard, io.Discard); code != 2 {
			t.Errorf("args %v: exit %d", args, code)
		}
	}
	if code := runCheckServer([]string{"--help"}, io.Discard, io.Discard); code != 0 {
		t.Errorf("help exit %d", code)
	}
	if endpoint, err := jobsEndpoint("http://localhost:8080/prefix/"); err != nil || endpoint != "http://localhost:8080/prefix/jobs" {
		t.Fatalf("endpoint %q, error %v", endpoint, err)
	}
}

func TestCheckServerSubcommandDispatch(t *testing.T) {
	// --help succeeds even when Codex is absent, proving this is a CLI command.
	t.Setenv("PATH", "")
	if code := run([]string{"check-server", "--help"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	parsed, err := parse([]string{"--", "check-server", "--help"}, io.Discard)
	if err != nil || len(parsed.args) != 2 || parsed.args[0] != "check-server" {
		t.Fatalf("Codex args changed: %#v, %v", parsed, err)
	}
}
