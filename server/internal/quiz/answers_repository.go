package quiz

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type AnswerRecord struct {
	ID            bson.ObjectID `json:"id" bson:"_id"`
	ParticipantID string        `json:"participant_id" bson:"participant_id"`
	QuizID        string        `json:"quiz_id" bson:"quiz_id"`
	QuestionID    string        `json:"question_id" bson:"question_id"`
	ProjectID     string        `json:"project_id" bson:"project_id"`
	ThreadID      string        `json:"thread_id" bson:"thread_id"`
	TurnID        string        `json:"turn_id" bson:"turn_id"`
	Answer        string        `json:"answer" bson:"answer"`
	Status        string        `json:"status" bson:"status"`
	CreatedAt     time.Time     `json:"created_at" bson:"created_at"`
	Model         string        `json:"model" bson:"model"`
	PromptVersion string        `json:"prompt_version" bson:"prompt_version"`
	Question      Question      `json:"question" bson:"question"`
	Request       Request       `json:"request" bson:"request"`
}

// Insert must enforce uniqueness of (participant_id, quiz_id, question_id).
// It must never overwrite an existing record, and reports ErrAnswerConflict
// on duplicate keys. Find reports ErrAnswerNotFound for absent records.
type AnswerRepository interface {
	Find(context.Context, string, string, string) (AnswerRecord, error)
	Insert(context.Context, AnswerRecord) (AnswerRecord, error)
}
type MongoAnswerRepository struct {
	database   *mongo.Database
	collection *mongo.Collection
}

func NewMongoAnswerRepository(database *mongo.Database) *MongoAnswerRepository {
	return &MongoAnswerRepository{database: database, collection: database.Collection("quiz_answers")}
}
func (r *MongoAnswerRepository) Find(ctx context.Context, participantID, quizID, questionID string) (AnswerRecord, error) {
	var record AnswerRecord
	err := r.collection.FindOne(ctx, bson.M{"participant_id": participantID, "quiz_id": quizID, "question_id": questionID}).Decode(&record)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return AnswerRecord{}, ErrAnswerNotFound
	}
	return record, err
}
func (r *MongoAnswerRepository) Insert(ctx context.Context, record AnswerRecord) (AnswerRecord, error) {
	// This is deliberately read-only: collection/index creation belongs to the
	// separately authorized migration, never an incoming answer request.
	names, err := r.database.ListCollectionNames(ctx, bson.M{"name": "quiz_answers"})
	if err != nil {
		return AnswerRecord{}, err
	}
	if len(names) == 0 {
		return AnswerRecord{}, ErrMigrationRequired
	}
	_, err = r.collection.InsertOne(ctx, record)
	if mongo.IsDuplicateKeyError(err) {
		return AnswerRecord{}, ErrAnswerConflict
	}
	if err != nil {
		return AnswerRecord{}, err
	}
	return record, nil
}
