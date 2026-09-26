package cortex

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestComplete(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"success", 200, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"verdict\":\"clear\"}"}}]}`, nil},
		{"snowflake empty finish reason", 200, `{"choices":[{"finish_reason":"","message":{"content":"{\"verdict\":\"clear\"}"}}]}`, nil},
		{"empty finish reason incomplete JSON", 200, `{"choices":[{"finish_reason":"","message":{"content":"{\"verdict\":"}}]}`, ErrInvalidResponse},
		{"auth", 401, `secret provider details`, ErrUpstream},
		{"rate limit", 429, `secret provider details`, ErrUpstream},
		{"truncated", 200, `{"choices":[{"finish_reason":"length","message":{"content":"{}"}}]}`, ErrInvalidResponse},
		{"bad content", 200, `{"choices":[{"finish_reason":"stop","message":{"content":"not json"}}]}`, ErrInvalidResponse},
		{"no choices", 200, `{"choices":[]}`, ErrInvalidResponse},
		{"bad envelope", 200, `not json`, ErrInvalidResponse},
		{"refusal", 200, `{"choices":[{"finish_reason":"stop","message":{"content":"{}","refusal":"no"}}]}`, ErrInvalidResponse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v2/cortex/v1/chat/completions" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("incorrect upstream request")
				}
				var payload struct {
					Model               string          `json:"model"`
					MaxCompletionTokens int             `json:"max_completion_tokens"`
					DeprecatedMaxTokens json.RawMessage `json:"max_tokens"`
					ResponseFormat      struct {
						Type string `json:"type"`
					} `json:"response_format"`
				}
				if json.NewDecoder(r.Body).Decode(&payload) != nil || payload.Model != "test-model" || payload.ResponseFormat.Type != "json_schema" {
					t.Error("missing model/schema")
				}
				if payload.MaxCompletionTokens != 4096 || payload.DeprecatedMaxTokens != nil {
					t.Error("expected max_completion_tokens only; Snowflake rejects max_tokens")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := NewClient(Config{server.URL, "test-token", "test-model", time.Second})
			if err != nil {
				t.Fatal(err)
			}
			client.http.Transport = server.Client().Transport
			got, err := client.Complete(context.Background(), "rubric", "input", json.RawMessage(`{"type":"object"}`))
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if tc.want == nil && string(got) != `{"verdict":"clear"}` {
				t.Fatalf("unexpected content %s", got)
			}
		})
	}
}
func TestCanceledRequest(t *testing.T) {
	client, _ := NewClient(Config{"https://example.snowflakecomputing.com", "token", "model", time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Complete(ctx, "rubric", "input", json.RawMessage(`{}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestConfig(t *testing.T) {
	t.Setenv("SNOWFLAKE_ACCOUNT_URL", "https://org-account.snowflakecomputing.com/")
	t.Setenv("SNOWFLAKE_PAT", "token")
	t.Setenv("SNOWFLAKE_MODEL", "model")
	t.Setenv("EVALUATION_TIMEOUT", "45s")
	if c, err := ConfigFromEnv(); err != nil || c.Timeout != 45*time.Second {
		t.Fatalf("config: %v", err)
	}
	for _, key := range []string{"SNOWFLAKE_ACCOUNT_URL", "SNOWFLAKE_PAT", "SNOWFLAKE_MODEL", "EVALUATION_TIMEOUT"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "\n")
			if _, err := ConfigFromEnv(); err == nil {
				t.Fatal("accepted invalid configuration")
			}
		})
	}
}

func TestUpstreamDiagnostics(t *testing.T) {
	for _, tc := range []struct{ body, code string }{
		{`{"code":"390432","message":"private token and payload"}`, "390432"},
		{`{"code":"secret-token","message":"private token and payload"}`, "unknown"},
		{`not json with private details`, "unknown"},
	} {
		err := upstreamError(401, strings.NewReader(tc.body))
		if !errors.Is(err, ErrUpstream) || err.Code != tc.code || err.Status != 401 || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsafe or missing diagnostics: %v", err)
		}
	}
}
