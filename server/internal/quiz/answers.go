package quiz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"cortisol-server/internal/user"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	ErrInvalidQuizMetadata = errors.New("quiz metadata requires a user_id ObjectID plus thread_id and turn_id of at most 256 bytes")
	ErrUserNotFound        = errors.New("user was not found")
	ErrQuizNotFound        = errors.New("quiz was not found or has expired")
	ErrQuestionNotFound    = errors.New("question was not found in this quiz")
	ErrQuizCapacity        = errors.New("quiz storage is at capacity; retry later")
	ErrAnswerNotFound      = errors.New("answer not found")
	ErrAnswerConflict      = errors.New("an answer has already been saved for this question")
	ErrMigrationRequired   = errors.New("answer storage requires a database migration")
)

type AnswerRequest struct {
	UserID     string   `json:"user_id"`
	QuizID     string   `json:"quiz_id"`
	QuestionID string   `json:"question_id"`
	Answer     string   `json:"answer"`
	Graded     *float64 `json:"graded"`
	Reasoning  string   `json:"reasoning"`
}

func (r AnswerRequest) Validate() error {
	if !user.ValidID(r.UserID) {
		return errors.New("user_id must be an existing MongoDB ObjectID")
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
	if r.Graded == nil || math.IsNaN(*r.Graded) || math.IsInf(*r.Graded, 0) || *r.Graded < 0 || *r.Graded > 1 {
		return errors.New("graded must be a number between 0 and 1")
	}
	if len(r.Reasoning) > 1200 || (*r.Graded < 1 && strings.TrimSpace(r.Reasoning) == "") {
		return errors.New("reasoning is required for partial or incorrect answers and must be at most 1200 bytes")
	}
	return nil
}

type AnswerReceipt struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	QuizID     string    `json:"quiz_id"`
	QuestionID string    `json:"question_id"`
	Status     string    `json:"status"`
	Graded     float64   `json:"graded"`
	CreatedAt  time.Time `json:"created_at"`
}

type userLookup interface {
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
	users     userLookup
	generator Generator
	mu        sync.Mutex
	quizzes   map[string]quizSnapshot
	pending   int
	capacity  int
	now       func() time.Time
}

func NewAnswerStore(repo AnswerRepository, users userLookup, generator Generator) *AnswerStore {
	return &AnswerStore{repo: repo, users: users, generator: generator, quizzes: make(map[string]quizSnapshot), capacity: 128, now: time.Now}
}
func validateQuizMetadata(r Request) error {
	if r.UserID == "" && r.ThreadID == "" && r.TurnID == "" {
		return nil
	}
	if !user.ValidID(r.UserID) {
		return ErrInvalidQuizMetadata
	}
	for _, value := range []string{r.ThreadID, r.TurnID} {
		if strings.TrimSpace(value) == "" || len(value) > 256 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return ErrInvalidQuizMetadata
		}
	}
	return nil
}
func (s *AnswerStore) requireUser(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	exists, err := s.users.Exists(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return ErrUserNotFound
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
	if r.UserID == "" {
		return s.generator.Generate(ctx, r)
	}
	if err := s.requireUser(ctx, r.UserID); err != nil {
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
	return AnswerReceipt{ID: record.ID.Hex(), UserID: record.UserID.Hex(), QuizID: record.QuizID.Hex(), QuestionID: record.QuestionID, Status: "graded", Graded: record.Graded, CreatedAt: record.CreatedAt}
}
func sameAnswer(record AnswerRecord, input AnswerRequest) (AnswerReceipt, error) {
	if record.QuizQuestion == "" || record.Answer != input.Answer || record.Graded != *input.Graded || record.Reasoning != strings.TrimSpace(input.Reasoning) {
		return AnswerReceipt{}, ErrAnswerConflict
	}
	return answerReceipt(record), nil
}
func (s *AnswerStore) Submit(ctx context.Context, input AnswerRequest) (AnswerReceipt, error) {
	if err := input.Validate(); err != nil {
		return AnswerReceipt{}, err
	}
	if err := s.requireUser(ctx, input.UserID); err != nil {
		return AnswerReceipt{}, err
	}
	existing, err := s.repo.Find(ctx, input.UserID, input.QuizID, input.QuestionID)
	if err == nil {
		return sameAnswer(existing, input)
	}
	if !errors.Is(err, ErrAnswerNotFound) {
		return AnswerReceipt{}, err
	}
	s.mu.Lock()
	s.purgeExpired(s.now())
	snapshot, ok := s.quizzes[input.QuizID]
	s.mu.Unlock()
	if !ok || snapshot.request.UserID != input.UserID {
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
	userID, _ := bson.ObjectIDFromHex(input.UserID)
	quizID, _ := bson.ObjectIDFromHex(input.QuizID)
	record := AnswerRecord{ID: bson.NewObjectID(), UserID: userID, QuizID: quizID, QuestionID: input.QuestionID, Answer: input.Answer, Graded: *input.Graded, Reasoning: strings.TrimSpace(input.Reasoning), QuizQuestion: selected.Question, Prompt: snapshot.request.Input, CreatedAt: s.now().UTC().Truncate(time.Millisecond)}
	if err = ctx.Err(); err != nil {
		return AnswerReceipt{}, err
	}
	record, err = s.repo.Insert(ctx, record)
	if errors.Is(err, ErrAnswerConflict) {
		existing, findErr := s.repo.Find(ctx, input.UserID, input.QuizID, input.QuestionID)
		if findErr != nil {
			return AnswerReceipt{}, fmt.Errorf("read concurrent answer: %w", findErr)
		}
		return sameAnswer(existing, input)
	}
	if err != nil {
		return AnswerReceipt{}, err
	}
	return answerReceipt(record), nil
}
func NewAnswerHandler(store *AnswerStore, timeout time.Duration) http.HandlerFunc {
	return newHandler(store.Submit, timeout, AnswerRequest.Validate)
}
