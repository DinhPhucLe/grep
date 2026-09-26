package evaluation

import (
	"context"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type MongoRepository struct{ collection *mongo.Collection }

func NewMongoRepository(database *mongo.Database) *MongoRepository {
	return &MongoRepository{collection: database.Collection("evaluations")}
}
func (r *MongoRepository) Insert(ctx context.Context, record Record) error {
	_, err := r.collection.InsertOne(ctx, record)
	return err
}
