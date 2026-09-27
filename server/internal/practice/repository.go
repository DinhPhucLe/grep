package practice

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type MongoEventRepository struct {
	collection *mongo.Collection
}

func NewMongoEventRepository(database *mongo.Database) *MongoEventRepository {
	return &MongoEventRepository{collection: database.Collection("practice_events")}
}

func (r *MongoEventRepository) Insert(ctx context.Context, event Event) error {
	_, err := r.collection.InsertOne(ctx, event)
	return err
}

func (r *MongoEventRepository) ListByUserPracticeYear(ctx context.Context, userID bson.ObjectID, practice string, year int) ([]Event, error) {
	from := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(year+1, time.January, 1, 0, 0, 0, 0, time.UTC)
	filter := bson.M{
		"user_id":    userID,
		"practice":   practice,
		"started_at": bson.M{"$gte": from, "$lt": to},
	}
	return r.find(ctx, filter)
}

func (r *MongoEventRepository) ListByOrgPracticeRange(ctx context.Context, orgID bson.ObjectID, practice string, from, to time.Time) ([]Event, error) {
	filter := bson.M{
		"organization_id": orgID,
		"practice":        practice,
		"started_at":      bson.M{"$gte": from, "$lte": to},
	}
	return r.find(ctx, filter)
}

func (r *MongoEventRepository) find(ctx context.Context, filter bson.M) ([]Event, error) {
	cursor, err := r.collection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "started_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var events []Event
	if err := cursor.All(ctx, &events); err != nil {
		return nil, err
	}
	if events == nil {
		events = []Event{}
	}
	return events, nil
}

type MongoMemberChecker struct {
	collection *mongo.Collection
}

func NewMongoMemberChecker(database *mongo.Database) *MongoMemberChecker {
	return &MongoMemberChecker{collection: database.Collection("organization_members")}
}

func (c *MongoMemberChecker) IsMember(ctx context.Context, organizationID, userID bson.ObjectID) (bool, error) {
	err := c.collection.FindOne(ctx, bson.M{
		"organization_id": organizationID,
		"user_id":         userID,
	}).Err()
	if err == mongo.ErrNoDocuments {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
