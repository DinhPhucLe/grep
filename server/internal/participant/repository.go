package participant

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type collectionLister interface {
	ListCollectionNames(context.Context, any, ...options.Lister[options.ListCollectionsOptions]) ([]string, error)
}
type profileCollection interface {
	FindOneAndUpdate(context.Context, any, any, ...options.Lister[options.FindOneAndUpdateOptions]) *mongo.SingleResult
	FindOne(context.Context, any, ...options.Lister[options.FindOneOptions]) *mongo.SingleResult
}

type MongoRepository struct {
	database   collectionLister
	collection profileCollection
}

func NewMongoRepository(database *mongo.Database) *MongoRepository {
	return &MongoRepository{database: database, collection: database.Collection("participants")}
}

func (r *MongoRepository) Register(ctx context.Context, input Registration) (Profile, error) {
	if err := input.Validate(); err != nil {
		return Profile{}, err
	}
	// Upsert would implicitly create a collection. Require the separately
	// managed schema before writing; registration never runs migrations.
	names, err := r.database.ListCollectionNames(ctx, bson.M{"name": "participants"})
	if err != nil {
		return Profile{}, err
	}
	if len(names) == 0 {
		return Profile{}, ErrMigrationRequired
	}
	now := time.Now().UTC()
	set := bson.M{"last_seen_at": now}
	if input.DisplayName != "" {
		set["display_name"] = input.DisplayName
	}
	update := bson.M{"$set": set, "$setOnInsert": bson.M{"created_at": now}}
	var profile Profile
	err = r.collection.FindOneAndUpdate(ctx, bson.M{"_id": input.ID}, update, options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)).Decode(&profile)
	return profile, err
}

func (r *MongoRepository) Exists(ctx context.Context, id string) (bool, error) {
	if !ValidID(id) {
		return false, nil
	}
	err := r.collection.FindOne(ctx, bson.M{"_id": id}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	return err == nil, err
}
