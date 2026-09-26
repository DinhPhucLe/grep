package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"cortisol-server/internal/cortex"
)

type fakeCompleter struct {
	raw   string
	err   error
	input string
}

func (c *fakeCompleter) Complete(_ context.Context, _ string, input string, _ json.RawMessage) (json.RawMessage, error) {
	c.input = input
	return json.RawMessage(c.raw), c.err
}

type memoryRepository struct {
	records []Record
	err     error
}

func (r *memoryRepository) Insert(_ context.Context, record Record) error {
	if r.err != nil {
		return r.err
	}
	r.records = append(r.records, record)
	return nil
}

func TestServiceValidationAndPersistence(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		valid     bool
	}{
		{"clear", `{"verdict":"clear","summary":"Requirements are established.","gaps":[]}`, true},
		{"ambiguous", `{"verdict":"ambiguous","summary":"Login behavior is unspecified.","gaps":[{"description":"Failure case unspecified","consequence":"Wrong login flow could change"}]}`, true},
		{"missing gaps", `{"verdict":"clear","summary":"Clear"}`, false},
		{"inconsistent", `{"verdict":"ambiguous","summary":"Unclear","gaps":[]}`, false},
		{"extra field", `{"verdict":"clear","summary":"Clear","gaps":[],"score":100}`, false},
		{"null", `null`, false},
		{"trailing", `{"verdict":"clear","summary":"Clear","gaps":[]} {}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeCompleter{raw: tc.raw}
			repo := &memoryRepository{}
			request := Request{Input: "fix login", Context: "Web service", Conversation: []Message{{Role: "user", Content: "Fix expired sessions"}}}
			got, err := NewService(client, repo, "model").Evaluate(context.Background(), request)
			if tc.valid {
				if err != nil || len(repo.records) != 1 || got.ID.IsZero() || got.RubricVersion == "" {
					t.Fatalf("result=%+v err=%v records=%d", got, err, len(repo.records))
				}
				var sent Request
				if json.Unmarshal([]byte(client.input), &sent) != nil || len(sent.Conversation) != 1 || sent.Context != request.Context {
					t.Fatal("context not forwarded")
				}
			} else if !errors.Is(err, cortex.ErrInvalidResponse) || len(repo.records) != 0 {
				t.Fatalf("invalid response saved: %v", err)
			}
		})
	}
}
func TestServiceFailures(t *testing.T) {
	repo := &memoryRepository{}
	client := &fakeCompleter{err: cortex.ErrUpstream}
	service := NewService(client, repo, "model")
	if _, err := service.Evaluate(context.Background(), Request{Input: "test"}); !errors.Is(err, cortex.ErrUpstream) || len(repo.records) != 0 {
		t.Fatal(err)
	}
	client.err = nil
	client.raw = `{"verdict":"clear","summary":"Clear","gaps":[]}`
	repo.err = errors.New("database details")
	if _, err := service.Evaluate(context.Background(), Request{Input: "test"}); !errors.Is(err, ErrStorage) {
		t.Fatal(err)
	}
}
