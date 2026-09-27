package practice

import (
	"context"
	"regexp"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Directory interface {
	RecommendPeople(ctx context.Context, query string, limit int) ([]PersonRecommendation, error)
	RecommendOrganizations(ctx context.Context, query string, limit int) ([]OrgRecommendation, error)
}

type MongoDirectory struct {
	users *mongo.Collection
	orgs  *mongo.Collection
}

func NewMongoDirectory(database *mongo.Database) *MongoDirectory {
	return &MongoDirectory{
		users: database.Collection("users"),
		orgs:  database.Collection("organizations"),
	}
}

type userDoc struct {
	ID   bson.ObjectID `bson:"_id"`
	Name string        `bson:"name"`
	Mail string        `bson:"mail"`
}

type orgDoc struct {
	ID   bson.ObjectID `bson:"_id"`
	Name string        `bson:"name"`
}

func (d *MongoDirectory) RecommendPeople(ctx context.Context, query string, limit int) ([]PersonRecommendation, error) {
	if limit <= 0 {
		limit = DefaultRecommendationLimit
	}
	filter := bson.M{}
	if q := strings.TrimSpace(query); q != "" {
		pattern := regexp.QuoteMeta(q)
		filter = bson.M{
			"$or": []bson.M{
				{"name": bson.M{"$regex": pattern, "$options": "i"}},
				{"mail": bson.M{"$regex": pattern, "$options": "i"}},
			},
		}
	}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit))
	cursor, err := d.users.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var docs []userDoc
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	items := make([]PersonRecommendation, 0, len(docs))
	for _, doc := range docs {
		items = append(items, PersonRecommendation{ID: doc.ID.Hex(), Name: doc.Name, Mail: doc.Mail})
	}
	return items, nil
}

func (d *MongoDirectory) RecommendOrganizations(ctx context.Context, query string, limit int) ([]OrgRecommendation, error) {
	if limit <= 0 {
		limit = DefaultRecommendationLimit
	}
	filter := bson.M{}
	if q := strings.TrimSpace(query); q != "" {
		pattern := regexp.QuoteMeta(q)
		filter = bson.M{"name": bson.M{"$regex": pattern, "$options": "i"}}
	}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit))
	cursor, err := d.orgs.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var docs []orgDoc
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	items := make([]OrgRecommendation, 0, len(docs))
	for _, doc := range docs {
		items = append(items, OrgRecommendation{ID: doc.ID.Hex(), Name: doc.Name})
	}
	return items, nil
}
