package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"cortisol-server/internal/cortex"
)

type fakeCompleter struct {
	raw    string
	err    error
	input  string
	schema json.RawMessage
}

func (c *fakeCompleter) Complete(_ context.Context, _ string, input string, schema json.RawMessage) (json.RawMessage, error) {
	c.input = input
	c.schema = schema
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
		{"missing score", `{"verdict":"clear","summary":"Clear","gaps":[]}`, false},
		{"null score", `{"verdict":"clear","summary":"Clear","ambiguity_score":null,"gaps":[]}`, false},
		{"negative score", `{"verdict":"clear","summary":"Clear","ambiguity_score":-0.01,"gaps":[]}`, false},
		{"score above one", `{"verdict":"clear","summary":"Clear","ambiguity_score":1.01,"gaps":[]}`, false},
		{"string score", `{"verdict":"clear","summary":"Clear","ambiguity_score":"0.20","gaps":[]}`, false},
		{"clear", `{"verdict":"clear","ambiguity_score":0.0,"summary":"Requirements are established.","gaps":[]}`, true},
		{"ambiguous", `{"verdict":"ambiguous","ambiguity_score":0.8,"summary":"Login behavior is unspecified.","gaps":[{"description":"Failure case unspecified","consequence":"Wrong login flow could change"}]}`, true},
		{"missing gaps", `{"verdict":"clear","ambiguity_score":0.0,"summary":"Clear"}`, false},
		{"inconsistent", `{"verdict":"ambiguous","ambiguity_score":0.8,"summary":"Unclear","gaps":[]}`, false},
		{"extra field", `{"verdict":"clear","ambiguity_score":0.0,"summary":"Clear","gaps":[],"score":100}`, false},
		{"null", `null`, false},
		{"trailing", `{"verdict":"clear","ambiguity_score":0.0,"summary":"Clear","gaps":[]} {}`, false},
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
				var expected Body
				if err := json.Unmarshal([]byte(tc.raw), &expected); err != nil {
					t.Fatal(err)
				}
				if got.Evaluation.AmbiguityScore != expected.AmbiguityScore || repo.records[0].Evaluation.AmbiguityScore != expected.AmbiguityScore {
					t.Fatal("ambiguity score lost before response or persistence")
				}
				var schema struct {
					Required   []string `json:"required"`
					Properties map[string]struct {
						Type    string   `json:"type"`
						Minimum *float64 `json:"minimum"`
						Maximum *float64 `json:"maximum"`
					} `json:"properties"`
				}
				if err := json.Unmarshal(client.schema, &schema); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, key := range schema.Required {
					found = found || key == "ambiguity_score"
				}
				field := schema.Properties["ambiguity_score"]
				if !found || field.Type != "number" || field.Minimum != nil || field.Maximum != nil {
					t.Fatal("Snowflake schema must require numeric ambiguity_score without unsupported numeric bounds")
				}
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
	client.raw = `{"verdict":"clear","ambiguity_score":0.0,"summary":"Clear","gaps":[]}`
	repo.err = errors.New("database details")
	if _, err := service.Evaluate(context.Background(), Request{Input: "test"}); !errors.Is(err, ErrStorage) {
		t.Fatal(err)
	}
}
