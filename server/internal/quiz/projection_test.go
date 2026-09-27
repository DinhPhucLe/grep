package quiz

import (
	"context"
	"errors"
	"testing"
)

type recordingProjection struct {
	err     error
	records []AnswerRecord
}

func (p *recordingProjection) SyncAnswer(_ context.Context, record AnswerRecord) error {
	if p.err != nil {
		return p.err
	}
	p.records = append(p.records, record)
	return nil
}

func TestAnswerRetryRepairsFailedPracticeProjection(t *testing.T) {
	store, repo, _, response := answerFixture(t)
	projection := &recordingProjection{err: errors.New("projection unavailable")}
	store.projection = projection
	input := gradedRequest(response.QuizID, "answer")
	if _, err := store.Submit(context.Background(), input); !errors.Is(err, projection.err) {
		t.Fatalf("projection failure must be retryable: %v", err)
	}
	if len(repo.records) != 1 {
		t.Fatal("original answer should remain saved")
	}
	projection.err = nil
	// A retry after restart must repair the event without an in-memory quiz.
	restarted := NewAnswerStore(repo, answerUsers{exists: true}, nil, projection)
	receipt, err := restarted.Submit(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.records) != 1 || len(projection.records) != 1 || projection.records[0].ID.Hex() != receipt.ID {
		t.Fatal("retry did not project the original answer exactly once")
	}
	input.Answer = "different answer"
	if _, err := restarted.Submit(context.Background(), input); !errors.Is(err, ErrAnswerConflict) {
		t.Fatal(err)
	}
	if len(projection.records) != 1 {
		t.Fatal("conflicting answer was projected")
	}
}
