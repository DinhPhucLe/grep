package quiz

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"cortisol-server/internal/cortex"
	"cortisol-server/internal/evaluation"
)

type Response struct {
	Model         string `json:"model"`
	PromptVersion string `json:"prompt_version"`
	Result
}

type Service struct {
	client evaluation.Completer
	model  string
}

func NewService(client evaluation.Completer, model string) *Service {
	return &Service{client: client, model: model}
}

func (s *Service) Generate(ctx context.Context, request Request) (Response, error) {
	if err := request.Validate(); err != nil {
		return Response{}, err
	}
	request.MaxQuestions = request.limit()
	input, err := json.Marshal(request)
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
		return Response{}, cortex.ErrInvalidResponse
	}
	var result Result
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || result.Validate(request) != nil {
		return Response{}, cortex.ErrInvalidResponse
	}
	return Response{Model: s.model, PromptVersion: PromptVersion, Result: result}, nil
}
