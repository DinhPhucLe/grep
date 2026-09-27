// Package knowledge stores project-scoped text with optional supplied embeddings.
package knowledge

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	ErrInvalid  = errors.New("invalid knowledge record")
	ErrTooLarge = errors.New("knowledge record exceeds size limit")
	ErrNotFound = errors.New("project or record not found")
)

// Input contains caller-supplied text and optional embedding metadata. Source
// is a label or locator, not an authorization credential or a unique key.
type Input struct {
	Source         string    `json:"source"`
	Topic          string    `json:"topic"`
	Content        string    `json:"content"`
	Embedding      []float64 `json:"embedding,omitempty"`
	EmbeddingModel string    `json:"embedding_model,omitempty"`
}

type Record struct {
	ID             bson.ObjectID `json:"id" bson:"_id"`
	ProjectID      bson.ObjectID `json:"project_id" bson:"project_id"`
	Source         string        `json:"source" bson:"source"`
	Topic          string        `json:"topic" bson:"topic"`
	Content        string        `json:"content" bson:"content"`
	Embedding      []float64     `json:"embedding,omitempty" bson:"embedding,omitempty"`
	EmbeddingModel string        `json:"embedding_model,omitempty" bson:"embedding_model,omitempty"`
	CreatedAt      time.Time     `json:"created_at" bson:"created_at"`
}

// Validate checks storage limits without changing the original source text.
func (in Input) Validate() error {
	for _, f := range []struct {
		name, value string
		limit       int
	}{
		{"source", in.Source, 1024}, {"topic", in.Topic, 256}, {"content", in.Content, 1024 * 1024},
	} {
		if err := validateText(f.name, f.value, f.limit); err != nil {
			return err
		}
	}
	if in.Embedding == nil && in.EmbeddingModel == "" {
		return nil
	}
	if len(in.Embedding) == 0 {
		return fmt.Errorf("embedding and model must be supplied together: %w", ErrInvalid)
	}
	if len(in.Embedding) > 4096 {
		return fmt.Errorf("embedding: %w", ErrTooLarge)
	}
	if err := validateText("embedding_model", in.EmbeddingModel, 256); err != nil {
		return err
	}
	nonzero := false
	for _, v := range in.Embedding {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("embedding must be finite: %w", ErrInvalid)
		}
		nonzero = nonzero || v != 0
	}
	if !nonzero {
		return fmt.Errorf("embedding must be nonzero: %w", ErrInvalid)
	}
	return nil
}

func validateText(name, value string, limit int) error {
	if len(value) > limit {
		return fmt.Errorf("%s: %w", name, ErrTooLarge)
	}
	if !utf8.ValidString(value) || strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must be nonblank UTF-8: %w", name, ErrInvalid)
	}
	return nil
}
