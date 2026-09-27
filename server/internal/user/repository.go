// Package user reads existing users and projects. It never creates profiles.
package user

import (
	"context"
	"errors"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func ValidID(value string) bool {
	id, err := bson.ObjectIDFromHex(value)
	return err == nil && !id.IsZero() && value == strings.ToLower(value)
}

type MongoRepository struct{ database *mongo.Database }

func NewMongoRepository(database *mongo.Database) *MongoRepository {
	return &MongoRepository{database: database}
}

func (r *MongoRepository) Exists(ctx context.Context, value string) (bool, error) {
	if !ValidID(value) {
		return false, nil
	}
	id, _ := bson.ObjectIDFromHex(value)
	return r.exists(ctx, "users", bson.M{"_id": id})
}

func (r *MongoRepository) OwnsProject(ctx context.Context, userID, projectID string) (bool, error) {
	if !ValidID(userID) || !ValidID(projectID) {
		return false, nil
	}
	owner, _ := bson.ObjectIDFromHex(userID)
	project, _ := bson.ObjectIDFromHex(projectID)
	return r.exists(ctx, "projects", bson.M{"_id": project, "user_id": owner})
}

func (r *MongoRepository) exists(ctx context.Context, collection string, filter bson.M) (bool, error) {
	err := r.database.Collection(collection).FindOne(ctx, filter, options.FindOne().SetProjection(bson.M{"_id": 1})).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	return err == nil, err
}
