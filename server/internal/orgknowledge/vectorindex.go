package orgknowledge

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// VectorIndexDefinition is the Atlas Vector Search index for Automated Embedding.
func VectorIndexDefinition() bson.M {
	return bson.M{
		"fields": bson.A{
			bson.M{
				"type":           "autoEmbed",
				"modality":       "text",
				"path":           "content",
				"model":          EmbeddingModel,
				"numDimensions":  EmbeddingDims,
				"similarity":     "cosine",
			},
			bson.M{"type": "filter", "path": "organization_id"},
			bson.M{"type": "filter", "path": "topics"},
		},
	}
}

// EnsureVectorIndex creates the voyage-code-4 autoEmbed index if it does not exist.
// Requires an Atlas cluster with Automated Embedding enabled; local Mongo may error.
func EnsureVectorIndex(ctx context.Context, database *mongo.Database) (created bool, err error) {
	coll := database.Collection(CollectionName)
	view := coll.SearchIndexes()
	cursor, err := view.List(ctx, options.SearchIndexes().SetName(VectorIndexName))
	if err != nil {
		return false, fmt.Errorf("list search indexes: %w", err)
	}
	defer cursor.Close(ctx)
	if cursor.Next(ctx) {
		return false, nil
	}
	if err := cursor.Err(); err != nil {
		return false, fmt.Errorf("list search indexes: %w", err)
	}

	model := mongo.SearchIndexModel{
		Definition: VectorIndexDefinition(),
		Options:    options.SearchIndexes().SetName(VectorIndexName).SetType("vectorSearch"),
	}
	name, err := view.CreateOne(ctx, model)
	if err != nil {
		return false, fmt.Errorf("create search index %s: %w", VectorIndexName, err)
	}
	if name == "" {
		name = VectorIndexName
	}
	return true, nil
}

// SmokeVectorSearch runs a natural-language $vectorSearch (Atlas embeds the query).
func SmokeVectorSearch(ctx context.Context, database *mongo.Database, organizationID, query string, limit int64) ([]Document, error) {
	result, err := NewMongoStore(database).Search(ctx, SearchParams{
		OrganizationID: organizationID,
		Query:          query,
		K:              int(limit),
	})
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}
