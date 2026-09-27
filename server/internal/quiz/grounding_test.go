package quiz

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerationProvidesExactSourceLineNumbers(t *testing.T) {
	for _, tc := range []struct {
		name, source, numbered string
		lines                  int
	}{
		{"trailing newline", "one\n\nthree\n", "1 | one\n2 | \n3 | three", 3},
		{"no trailing newline", "one\n\nthree", "1 | one\n2 | \n3 | three", 3},
		{"trailing blank line", "one\ntwo\nthree\n\n", "1 | one\n2 | two\n3 | three\n4 | ", 4},
		{"trailing whitespace line", "one\ntwo\nthree\n  \n", "1 | one\n2 | two\n3 | three\n4 |   ", 4},
		{"unicode and CRLF", "α\r\n\tβ\r\nγ\r\n", "1 | α\r\n2 | \tβ\r\n3 | γ\r", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := exampleRequest()
			request.Files[0].Content = tc.source
			raw, _ := json.Marshal(exampleResult())
			client := &fakeCompleter{raw: string(raw)}
			response, err := NewService(client, "test").Generate(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			var sent struct {
				Files []struct {
					Path      string `json:"path"`
					Content   string `json:"content"`
					LineCount int    `json:"line_count"`
				} `json:"files"`
			}
			if err := json.Unmarshal([]byte(client.input), &sent); err != nil {
				t.Fatal(err)
			}
			if len(sent.Files) != 1 || sent.Files[0].Path != "login.go" || sent.Files[0].Content != tc.numbered || sent.Files[0].LineCount != tc.lines {
				t.Fatalf("model did not receive exact source coordinates: %+v", sent.Files)
			}
			if request.Files[0].Content != tc.source || response.Questions[0].Evidence[0].StartLine != 3 || client.calls != 1 {
				t.Fatal("numbering mutated the original snapshot, shifted evidence, or retried")
			}
		})
	}
}

func TestCodeReferenceErrorExplainsBounds(t *testing.T) {
	request := exampleRequest()
	result := exampleResult()
	result.Questions[0].Evidence[0].EndLine = 4
	err := result.Validate(request)
	if err == nil || !strings.Contains(err.Error(), "invalid code reference") || !strings.Contains(err.Error(), "1..3") || !strings.Contains(err.Error(), "3..4") {
		t.Fatalf("missing safe coordinate diagnosis: %v", err)
	}
}
