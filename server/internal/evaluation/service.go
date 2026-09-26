package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"cortisol-server/internal/cortex"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var ErrStorage = errors.New("evaluation could not be saved")

type Completer interface {
	Complete(context.Context, string, string, json.RawMessage) (json.RawMessage, error)
}
type Repository interface {
	Insert(context.Context, Record) error
}
type Service struct {
	client     Completer
	repository Repository
	model      string
}

func NewService(client Completer, repository Repository, model string) *Service {
	return &Service{client: client, repository: repository, model: model}
}

func (s *Service) Evaluate(ctx context.Context, request Request) (Record, error) {
	if err := request.Validate(); err != nil {
		return Record{}, err
	}
	input, err := json.Marshal(request)
	if err != nil {
		return Record{}, err
	}
	raw, err := s.client.Complete(ctx, systemPrompt, string(input), responseSchema)
	if err != nil {
		return Record{}, err
	}
	var body Body
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF || body.Validate() != nil {
		return Record{}, cortex.ErrInvalidResponse
	}
	record := Record{ID: bson.NewObjectID(), CreatedAt: time.Now().UTC(), Model: s.model, RubricVersion: rubricVersion, Request: request, Evaluation: body}
	if err := s.repository.Insert(ctx, record); err != nil {
		if ctx.Err() != nil {
			return Record{}, ctx.Err()
		}
		return Record{}, ErrStorage
	}
	return record, nil
}
