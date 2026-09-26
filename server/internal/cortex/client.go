// Package cortex handles Snowflake's HTTP protocol independently of evaluation rules.
package cortex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var ErrUpstream = errors.New("Cortex request failed")
var ErrInvalidResponse = errors.New("Cortex returned an invalid response")

// UpstreamError retains only safe diagnostic metadata, never provider messages,
// which may contain submitted content or connection details.
type UpstreamError struct {
	Status int
	Code   string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("Cortex request failed (HTTP %d, Snowflake code %s)", e.Status, e.Code)
}
func (e *UpstreamError) Unwrap() error { return ErrUpstream }

func upstreamError(status int, body io.Reader) *UpstreamError {
	e := &UpstreamError{Status: status, Code: "unknown"}
	var response struct {
		Code string `json:"code"`
	}
	if json.NewDecoder(io.LimitReader(body, 16<<10)).Decode(&response) == nil {
		// Snowflake's numeric codes are safe to log; discard arbitrary strings.
		if len(response.Code) == 6 && strings.Trim(response.Code, "0123456789") == "" {
			e.Code = response.Code
		}
	}
	return e
}

const maxCompletionTokens = 4096

type Client struct {
	config Config
	http   *http.Client
}

func NewClient(config Config) (*Client, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &Client{config: config, http: &http.Client{Timeout: config.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *Client) Complete(ctx context.Context, system, input string, schema json.RawMessage) (json.RawMessage, error) {
	payload := struct {
		Model               string              `json:"model"`
		Messages            []map[string]string `json:"messages"`
		Stream              bool                `json:"stream"`
		Temperature         float64             `json:"temperature"`
		MaxCompletionTokens int                 `json:"max_completion_tokens"`
		ResponseFormat      any                 `json:"response_format"`
	}{c.config.Model, []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": input}}, false, 0, maxCompletionTokens,
		map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "prompt_evaluation", "schema": schema}}}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode Cortex request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.config.AccountURL, "/")+"/api/v2/cortex/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, ErrUpstream
	}
	req.Header.Set("Authorization", "Bearer "+c.config.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		return nil, ErrUpstream // Never expose URL, token, or payload in transport errors.
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, upstreamError(resp.StatusCode, resp.Body)
	}
	const maxResponse = 2 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrUpstream
	}
	if len(data) > maxResponse {
		return nil, ErrInvalidResponse
	}
	var envelope struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content json.RawMessage `json:"content"`
				Refusal string          `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(data, &envelope) != nil || len(envelope.Choices) != 1 {
		return nil, ErrInvalidResponse
	}
	choice := envelope.Choices[0]
	// Snowflake can return an empty finish_reason for complete non-streaming
	// Claude responses. Accept it only with a JSON object string; the evaluation
	// service still validates required fields and verdict consistency. Explicit
	// length, refusal, tool-call, and other finish reasons remain rejected.
	content, ok := jsonObjectContent(choice.Message.Content)
	if (choice.FinishReason != "stop" && choice.FinishReason != "") || choice.Message.Refusal != "" || !ok {
		return nil, ErrInvalidResponse
	}
	return content, nil
}

// jsonObjectContent accepts OpenAI-style string content that decodes to a JSON
// object. Non-string content (arrays/objects) and non-object JSON values fail.
func jsonObjectContent(raw json.RawMessage) (json.RawMessage, bool) {
	var content string
	if json.Unmarshal(raw, &content) != nil {
		return nil, false
	}
	content = strings.TrimSpace(content)
	if content == "" || content[0] != '{' || !json.Valid([]byte(content)) {
		return nil, false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal([]byte(content), &object) != nil {
		return nil, false
	}
	return json.RawMessage(content), true
}
