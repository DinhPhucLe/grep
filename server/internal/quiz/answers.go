package quiz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"cortisol-server/internal/participant"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	ErrInvalidQuizMetadata = errors.New("quiz metadata requires a valid participant_id and nonblank project_id, thread_id, and turn_id of at most 256 bytes")
	ErrParticipantNotFound = errors.New("participant is not registered")
	ErrQuizNotFound        = errors.New("quiz was not found or has expired")
	ErrQuestionNotFound    = errors.New("question was not found in this quiz")
	ErrQuizCapacity        = errors.New("quiz storage is at capacity; retry later")
	ErrAnswerNotFound      = errors.New("answer not found")
	ErrAnswerConflict      = errors.New("an answer has already been saved for this question")
	ErrMigrationRequired   = errors.New("answer storage requires a database migration")
)

type AnswerRequest struct {
	ParticipantID string `json:"participant_id"`
	QuizID        string `json:"quiz_id"`
	QuestionID    string `json:"question_id"`
	Answer        string `json:"answer"`
}

func (r AnswerRequest) Validate() error {
	if !participant.ValidID(r.ParticipantID) {
		return errors.New("participant_id must be a canonical UUID")
	}
	if _, err := bson.ObjectIDFromHex(r.QuizID); err != nil || r.QuizID != strings.ToLower(r.QuizID) {
		return errors.New("quiz_id must be a server-issued quiz ID")
	}
	if len(r.QuestionID) != 2 || r.QuestionID[0] != 'q' || r.QuestionID[1] < '1' || r.QuestionID[1] > '4' {
		return errors.New("question_id must identify a quiz question")
	}
	if strings.TrimSpace(r.Answer) == "" || len(r.Answer) > 8000 {
		return errors.New("answer must contain 1 to 8000 bytes of nonblank text")
	}
	return nil
}

type AnswerReceipt struct {
	ID            string    `json:"id"`
	ParticipantID string    `json:"participant_id"`
	QuizID        string    `json:"quiz_id"`
	QuestionID    string    `json:"question_id"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

type participantLookup interface {
	Exists(context.Context, string) (bool, error)
}
type quizSnapshot struct {
	request  Request
	response Response
	expires  time.Time
}

// AnswerStore keeps short-lived, server-authoritative quiz context. Durable
// answers remain idempotently retryable when a quiz expires or the server restarts.
type AnswerStore struct {
	repo      AnswerRepository
	profiles  participantLookup
	generator Generator
	mu        sync.Mutex
	quizzes   map[string]quizSnapshot
	pending   int
	capacity  int
	now       func() time.Time
}

func NewAnswerStore(repo AnswerRepository, profiles participantLookup, generator Generator) *AnswerStore {
	return &AnswerStore{repo: repo, profiles: profiles, generator: generator, quizzes: make(map[string]quizSnapshot), capacity: 128, now: time.Now}
}
func validateQuizMetadata(r Request) error {
	if r.ParticipantID == "" && r.ProjectID == "" && r.ThreadID == "" && r.TurnID == "" {
		return nil
	}
	if !participant.ValidID(r.ParticipantID) {
		return ErrInvalidQuizMetadata
	}
	for _, value := range []string{r.ProjectID, r.ThreadID, r.TurnID} {
		if strings.TrimSpace(value) == "" || len(value) > 256 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return ErrInvalidQuizMetadata
		}
	}
	return nil
}
func (s *AnswerStore) requireParticipant(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	exists, err := s.profiles.Exists(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return ErrParticipantNotFound
	}
	return ctx.Err()
}
func (s *AnswerStore) purgeExpired(now time.Time) {
	for id, snapshot := range s.quizzes {
		if !now.Before(snapshot.expires) {
			delete(s.quizzes, id)
		}
	}
}
func (s *AnswerStore) Generate(ctx context.Context, r Request) (Response, error) {
	if err := r.Validate(); err != nil {
		return Response{}, err
	}
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	if r.ParticipantID == "" {
		return s.generator.Generate(ctx, r)
	}
	if err := s.requireParticipant(ctx, r.ParticipantID); err != nil {
		return Response{}, err
	}
	s.mu.Lock()
	s.purgeExpired(s.now())
	if len(s.quizzes)+s.pending >= s.capacity {
		s.mu.Unlock()
		return Response{}, ErrQuizCapacity
	}
	s.pending++
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.pending--; s.mu.Unlock() }()
	snapshotRequest, err := cloneQuizValue(r)
	if err != nil {
		return Response{}, err
	}
	res, err := s.generator.Generate(ctx, r)
	if err != nil {
		return Response{}, err
	}
	if err = ctx.Err(); err != nil {
		return Response{}, err
	}
	if len(res.Questions) == 0 {
		res.QuizID = ""
		return res, nil
	}
	res.QuizID = bson.NewObjectID().Hex()
	snapshotResponse, err := cloneQuizValue(res)
	if err != nil {
		return Response{}, err
	}
	s.mu.Lock()
	s.quizzes[res.QuizID] = quizSnapshot{request: snapshotRequest, response: snapshotResponse, expires: s.now().Add(24 * time.Hour)}
	s.mu.Unlock()
	return res, nil
}
func cloneQuizValue[T any](value T) (T, error) {
	var clone T
	raw, err := json.Marshal(value)
	if err != nil {
		return clone, err
	}
	err = json.Unmarshal(raw, &clone)
	return clone, err
}
func answerReceipt(record AnswerRecord) AnswerReceipt {
	return AnswerReceipt{ID: record.ID.Hex(), ParticipantID: record.ParticipantID, QuizID: record.QuizID, QuestionID: record.QuestionID, Status: record.Status, CreatedAt: record.CreatedAt}
}
func sameAnswer(record AnswerRecord, answer string) (AnswerReceipt, error) {
	if record.Answer != answer {
		return AnswerReceipt{}, ErrAnswerConflict
	}
	return answerReceipt(record), nil
}
func (s *AnswerStore) Submit(ctx context.Context, input AnswerRequest) (AnswerReceipt, error) {
	if err := input.Validate(); err != nil {
		return AnswerReceipt{}, err
	}
	if err := s.requireParticipant(ctx, input.ParticipantID); err != nil {
		return AnswerReceipt{}, err
	}
	existing, err := s.repo.Find(ctx, input.ParticipantID, input.QuizID, input.QuestionID)
	if err == nil {
		return sameAnswer(existing, input.Answer)
	}
	if !errors.Is(err, ErrAnswerNotFound) {
		return AnswerReceipt{}, err
	}
	s.mu.Lock()
	s.purgeExpired(s.now())
	snapshot, ok := s.quizzes[input.QuizID]
	s.mu.Unlock()
	if !ok || snapshot.request.ParticipantID != input.ParticipantID {
		return AnswerReceipt{}, ErrQuizNotFound
	}
	var selected *Question
	for _, question := range snapshot.response.Questions {
		if question.ID == input.QuestionID {
			selected = &question
			break
		}
	}
	if selected == nil {
		return AnswerReceipt{}, ErrQuestionNotFound
	}
	record := AnswerRecord{ID: bson.NewObjectID(), ParticipantID: input.ParticipantID, QuizID: input.QuizID, QuestionID: input.QuestionID, ProjectID: snapshot.request.ProjectID, ThreadID: snapshot.request.ThreadID, TurnID: snapshot.request.TurnID, Answer: input.Answer, Status: "ungraded", CreatedAt: s.now().UTC().Truncate(time.Millisecond), Model: snapshot.response.Model, PromptVersion: snapshot.response.PromptVersion, Question: *selected, Request: snapshot.request}
	// Repository implementations must not receive aliases to the cached snapshot.
	record, err = cloneQuizValue(record)
	if err != nil {
		return AnswerReceipt{}, err
	}
	if err = ctx.Err(); err != nil {
		return AnswerReceipt{}, err
	}
	record, err = s.repo.Insert(ctx, record)
	if errors.Is(err, ErrAnswerConflict) {
		existing, findErr := s.repo.Find(ctx, input.ParticipantID, input.QuizID, input.QuestionID)
		if findErr != nil {
			return AnswerReceipt{}, fmt.Errorf("read concurrent answer: %w", findErr)
		}
		return sameAnswer(existing, input.Answer)
	}
	if err != nil {
		return AnswerReceipt{}, err
	}
	return answerReceipt(record), nil
}
func NewAnswerHandler(store *AnswerStore, timeout time.Duration) http.HandlerFunc {
	return newHandler(store.Submit, timeout, AnswerRequest.Validate)
}
