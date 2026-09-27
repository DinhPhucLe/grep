package quiz

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"cortisol-server/internal/cortex"
)

// Reproduces the captured Snowflake response: q2/q3 overlap q1;
// q4 refers to a separate file and can be revealed independently.
func overlappingQuiz() (Request, Result) {
	request := exampleRequest()
	request.Files = []File{{Path: "main.go", Content: strings.Repeat("code\n", 65)}, {Path: "main_test.go", Content: strings.Repeat("test\n", 42)}}
	result := Result{Questions: []Question{
		{ID: "q1", Topic: "Scope", Question: "Is the limit shared?", GapIndices: []int{0}, Evidence: []Evidence{{"main.go", 17, 32}}},
		{ID: "q2", Topic: "Threshold", Question: "What rate is allowed?", GapIndices: []int{0}, Evidence: []Evidence{{"main.go", 22, 30}}},
		{ID: "q3", Topic: "Response", Question: "What response is sent?", GapIndices: []int{0}, Evidence: []Evidence{{"main.go", 26, 29}}},
		{ID: "q4", Topic: "Methods", Question: "Do methods share a limit?", GapIndices: []int{0}, Evidence: []Evidence{{"main_test.go", 29, 32}}},
	}}
	return request, result
}

func TestGenerationKeepsIndependentlyRevealableQuestions(t *testing.T) {
	request, result := overlappingQuiz()
	if result.Validate(request) == nil {
		t.Fatal("overlapping ranges must remain invalid for rendering and grading")
	}
	raw, _ := json.Marshal(result)
	client := &fakeCompleter{raw: string(raw)}
	got, err := NewService(client, "test").Generate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Questions) != 2 || got.Questions[0].Topic != "Scope" || got.Questions[1].Topic != "Methods" || got.Questions[1].ID != "q2" {
		t.Fatalf("wrong independent questions: %+v", got.Questions)
	}
	if err := got.Result.Validate(request); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 {
		t.Fatal("overlap recovery must not make another billable call")
	}
}

func TestOverlapRecoveryStillRejectsInvalidReferences(t *testing.T) {
	request, result := overlappingQuiz()
	// Even a question that would be dropped must be structurally valid.
	result.Questions[1].Evidence = append(result.Questions[1].Evidence, Evidence{"invented.go", 1, 2})
	raw, _ := json.Marshal(result)
	_, err := NewService(&fakeCompleter{raw: string(raw)}, "test").Generate(context.Background(), request)
	if !errors.Is(err, cortex.ErrInvalidResponse) {
		t.Fatalf("invalid references accepted: %v", err)
	}
}
