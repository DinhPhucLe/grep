package knowledge

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type MongoRepository struct {
	projects *mongo.Collection
	records  *mongo.Collection
}

func NewMongoRepository(database *mongo.Database) *MongoRepository {
	return &MongoRepository{projects: database.Collection("projects"), records: database.Collection("knowledge_records")}
}

// Create saves a new record. The caller ID must come from an authenticated or
// explicit administrative context. This repository does not authenticate it.
// Each call inserts a new record; source is not an idempotency key.
func (r *MongoRepository) Create(ctx context.Context, callerID, projectID bson.ObjectID, input Input) (Record, error) {
	if err := validateScope(callerID, projectID); err != nil {
		return Record{}, err
	}
	if err := input.Validate(); err != nil {
		return Record{}, err
	}
	if err := r.requireOwner(ctx, callerID, projectID); err != nil {
		return Record{}, err
	}
	record := Record{ID: bson.NewObjectID(), ProjectID: projectID, Source: input.Source, Topic: input.Topic, Content: input.Content, Embedding: append([]float64(nil), input.Embedding...), EmbeddingModel: input.EmbeddingModel, CreatedAt: time.Now().UTC().Truncate(time.Millisecond)}
	if _, err := r.records.InsertOne(ctx, record); err != nil {
		return Record{}, fmt.Errorf("create knowledge record: %w", err)
	}
	return record, nil
}

func (r *MongoRepository) Get(ctx context.Context, callerID, projectID, id bson.ObjectID) (Record, error) {
	if err := validateScope(callerID, projectID); err != nil {
		return Record{}, err
	}
	if id.IsZero() {
		return Record{}, fmt.Errorf("record id: %w", ErrInvalid)
	}
	if err := r.requireOwner(ctx, callerID, projectID); err != nil {
		return Record{}, err
	}
	var record Record
	err := r.records.FindOne(ctx, bson.D{{Key: "_id", Value: id}, {Key: "project_id", Value: projectID}}).Decode(&record)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("get knowledge record: %w", err)
	}
	return record, nil
}

// List returns at most limit records, newest first with an _id tie-breaker.
// Pagination and semantic search are deliberately outside this storage API.
func (r *MongoRepository) List(ctx context.Context, callerID, projectID bson.ObjectID, limit int) ([]Record, error) {
	if err := validateScope(callerID, projectID); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("limit must be 1..100: %w", ErrInvalid)
	}
	if err := r.requireOwner(ctx, callerID, projectID); err != nil {
		return nil, err
	}
	cursor, err := r.records.Find(ctx, bson.D{{Key: "project_id", Value: projectID}}, options.Find().SetLimit(int64(limit)).SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}))
	if err != nil {
		return nil, fmt.Errorf("list knowledge records: %w", err)
	}
	defer cursor.Close(ctx)
	records := make([]Record, 0)
	if err = cursor.All(ctx, &records); err != nil {
		return nil, fmt.Errorf("decode knowledge records: %w", err)
	}
	return records, nil
}

func validateScope(callerID, projectID bson.ObjectID) error {
	if callerID.IsZero() || projectID.IsZero() {
		return fmt.Errorf("caller/project id: %w", ErrInvalid)
	}
	return nil
}

// Resolve current project ownership on every operation; organization membership
// does not grant access. Foreign and nonexistent projects share the same error.
func (r *MongoRepository) requireOwner(ctx context.Context, callerID, projectID bson.ObjectID) error {
	err := r.projects.FindOne(ctx, bson.D{{Key: "_id", Value: projectID}, {Key: "user_id", Value: callerID}}, options.FindOne().SetProjection(bson.D{{Key: "_id", Value: 1}})).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("resolve project owner: %w", err)
	}
	return nil
}
