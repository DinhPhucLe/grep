package quiz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"cortisol-server/internal/cortex"
	"cortisol-server/internal/evaluation"
	"cortisol-server/internal/timing"
)

type Response struct {
	QuizID        string `json:"quiz_id,omitempty"`
	Model         string `json:"model"`
	PromptVersion string `json:"prompt_version"`
	Result
}

type Service struct {
	client evaluation.Completer
	model  string
}

// Reasons are fixed validation messages, never raw model output or source code.
type invalidQuizResponse struct{ reason string }

func (e *invalidQuizResponse) Error() string { return "Cortex returned an invalid quiz: " + e.reason }
func (e *invalidQuizResponse) Unwrap() error { return cortex.ErrInvalidResponse }

func NewService(client evaluation.Completer, model string) *Service {
	return &Service{client: client, model: model}
}

func (s *Service) Generate(ctx context.Context, request Request) (Response, error) {
	if err := request.Validate(); err != nil {
		return Response{}, err
	}
	request.MaxQuestions = request.limit()
	input, err := generationInput(request)
	if err != nil {
		return Response{}, err
	}
	raw, err := s.client.Complete(ctx, systemPrompt, string(input), responseSchema)
	if err != nil {
		return Response{}, err
	}
	var required struct {
		Reason *string `json:"no_questions_reason"`
	}
	if json.Unmarshal(raw, &required) != nil || required.Reason == nil {
		return Response{}, &invalidQuizResponse{reason: "missing or invalid no_questions_reason"}
	}
	var result Result
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF {
		return Response{}, &invalidQuizResponse{reason: "invalid quiz JSON structure"}
	}
	if err := result.validate(request, false); err != nil {
		return Response{}, &invalidQuizResponse{reason: err.Error()}
	}
	started := time.Now()
	generated := len(result.Questions)
	result = result.independentQuestions()
	err = result.Validate(request)
	timing.Record(ctx, "quiz.independent_questions", started, err, map[string]int{"generated": generated, "retained": len(result.Questions)})
	if err != nil {
		return Response{}, &invalidQuizResponse{reason: err.Error()}
	}
	return Response{Model: s.model, PromptVersion: PromptVersion, Result: result}, nil
}

// Number only the provider's copy. The original snapshot remains unchanged for
// validation and rendering. A final newline terminates the last
// line; it does not add an empty line, matching Result.Validate.
func generationInput(request Request) ([]byte, error) {
	request.UserID, request.ThreadID, request.TurnID = "", "", ""
	type numberedFile struct {
		Path      string `json:"path"`
		Content   string `json:"content"`
		LineCount int    `json:"line_count"`
	}
	files := make([]numberedFile, 0, len(request.Files))
	for _, file := range request.Files {
		lines := strings.Split(strings.TrimSuffix(file.Content, "\n"), "\n")
		var content strings.Builder
		for i, line := range lines {
			if i > 0 {
				content.WriteByte('\n')
			}
			fmt.Fprintf(&content, "%d | %s", i+1, line)
		}
		files = append(files, numberedFile{Path: file.Path, Content: content.String(), LineCount: len(lines)})
	}
	return json.Marshal(struct {
		Request
		Files []numberedFile `json:"files"`
	}{Request: request, Files: files})
}
