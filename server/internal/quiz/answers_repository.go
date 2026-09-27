package quiz

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type AnswerRecord struct {
	ID           bson.ObjectID `json:"id" bson:"_id"`
	UserID       bson.ObjectID `json:"user_id" bson:"user_id"`
	QuizID       bson.ObjectID `json:"quiz_id" bson:"quiz_id"`
	QuestionID   string        `json:"question_id" bson:"question_id"`
	ProjectID    bson.ObjectID `json:"project_id" bson:"project_id"`
	Answer       string        `json:"answer" bson:"answer"`
	QuizQuestion string        `json:"quiz_question" bson:"quiz_question"`
	Graded       float64       `json:"graded" bson:"graded"`
	Reasoning    string        `json:"reasoning" bson:"reasoning"`
	Prompt       string        `json:"prompt" bson:"prompt"`
	CreatedAt    time.Time     `json:"created_at" bson:"created_at"`
}

// Insert must enforce uniqueness of (user_id, quiz_id, question_id).
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
func (r *MongoAnswerRepository) Find(ctx context.Context, userID, quizID, questionID string) (AnswerRecord, error) {
	user, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return AnswerRecord{}, err
	}
	quiz, err := bson.ObjectIDFromHex(quizID)
	if err != nil {
		return AnswerRecord{}, err
	}
	var record AnswerRecord
	err = r.collection.FindOne(ctx, bson.M{"user_id": user, "quiz_id": quiz, "question_id": questionID}).Decode(&record)
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
	cursor, err := r.collection.Indexes().List(ctx)
	if err != nil {
		return AnswerRecord{}, err
	}
	defer cursor.Close(ctx)
	ready := false
	for cursor.Next(ctx) {
		var index struct {
			Name    string `bson:"name"`
			Unique  bool   `bson:"unique"`
			Key     bson.D `bson:"key"`
			Sparse  bool   `bson:"sparse"`
			Partial bson.M `bson:"partialFilterExpression"`
		}
		if err := cursor.Decode(&index); err != nil {
			return AnswerRecord{}, err
		}
		if index.Name != "one_answer_per_user_question" || !index.Unique || index.Sparse || len(index.Partial) != 0 || len(index.Key) != 3 {
			continue
		}
		valid := true
		for i, field := range []string{"user_id", "quiz_id", "question_id"} {
			if index.Key[i].Key != field || (index.Key[i].Value != int32(1) && index.Key[i].Value != int64(1)) {
				valid = false
			}
		}
		ready = ready || valid
	}
	if err := cursor.Err(); err != nil {
		return AnswerRecord{}, err
	}
	if !ready {
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
