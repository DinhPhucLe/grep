package orgknowledge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// EnsureSchema creates knowledge_documents with validator and indexes if missing.
// Safe to call when golang-migrate cannot advance past the duplicate v5 baseline.
func EnsureSchema(ctx context.Context, database *mongo.Database) error {
	names, err := database.ListCollectionNames(ctx, bson.D{{Key: "name", Value: CollectionName}})
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}
	if len(names) == 0 {
		validator := bson.M{
			"$jsonSchema": bson.M{
				"bsonType":             "object",
				"required":             []string{"_id", "content", "topics", "properties", "authors", "created_at", "updated_at", "organization_id"},
				"additionalProperties": false,
				"properties": bson.M{
					"_id":     bson.M{"bsonType": "objectId"},
					"content": bson.M{"bsonType": "string", "minLength": 1, "maxLength": 65536},
					"topics": bson.M{
						"bsonType": "array", "minItems": 0, "maxItems": 32,
						"items": bson.M{"bsonType": "string", "minLength": 1, "maxLength": 64},
					},
					"properties": bson.M{"bsonType": "object"},
					"authors": bson.M{
						"bsonType": "array", "minItems": 1, "maxItems": 8,
						"items": bson.M{
							"bsonType": "object", "required": []string{"user_id"}, "additionalProperties": false,
							"properties": bson.M{
								"user_id": bson.M{"bsonType": "string", "minLength": 1, "maxLength": 64},
								"name":    bson.M{"bsonType": "string", "maxLength": 256},
							},
						},
					},
					"created_at":      bson.M{"bsonType": "date"},
					"updated_at":      bson.M{"bsonType": "date"},
					"organization_id": bson.M{"bsonType": "string", "minLength": 1, "maxLength": 64},
				},
			},
		}
		err := database.CreateCollection(ctx, CollectionName, options.CreateCollection().
			SetValidator(validator).SetValidationLevel("strict").SetValidationAction("error"))
		if err != nil {
			return fmt.Errorf("create %s: %w", CollectionName, err)
		}
	}
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "organization_id", Value: 1}, {Key: "created_at", Value: -1}, {Key: "_id", Value: -1}},
			Options: options.Index().SetName("knowledge_documents_by_org_created"),
		},
		{
			Keys:    bson.D{{Key: "organization_id", Value: 1}, {Key: "topics", Value: 1}},
			Options: options.Index().SetName("knowledge_documents_by_org_topics"),
		},
	}
	_, err = database.Collection(CollectionName).Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return fmt.Errorf("create indexes: %w", err)
	}
	return nil
}

// SeedResult counts upsert outcomes for knowledge documents.
type SeedResult struct {
	Inserted int
	Existing int
}

// SeedDocuments upserts documents by _id ($setOnInsert).
func SeedDocuments(ctx context.Context, database *mongo.Database, docs []Document) (SeedResult, error) {
	var result SeedResult
	coll := database.Collection(CollectionName)
	for _, doc := range docs {
		if doc.Properties == nil {
			doc.Properties = map[string]string{}
		}
		update, err := coll.UpdateOne(ctx,
			bson.M{"_id": doc.ID},
			bson.M{"$setOnInsert": doc},
			options.UpdateOne().SetUpsert(true))
		if err != nil {
			return result, fmt.Errorf("seed knowledge %s: %w", doc.ID.Hex(), err)
		}
		if update.UpsertedCount > 0 {
			result.Inserted++
		} else {
			result.Existing++
		}
	}
	return result, nil
}

// MongoStore implements Store against knowledge_documents.
type MongoStore struct {
	coll *mongo.Collection
}

// NewMongoStore returns a Store backed by the application database.
func NewMongoStore(database *mongo.Database) *MongoStore {
	return &MongoStore{coll: database.Collection(CollectionName)}
}

// Insert validates and persists a knowledge document.
func (s *MongoStore) Insert(ctx context.Context, doc Document) (Document, error) {
	if err := doc.ValidateForCreate(); err != nil {
		return Document{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	if doc.ID.IsZero() {
		doc.ID = bson.NewObjectID()
	}
	doc.Content = strings.TrimSpace(doc.Content)
	if doc.Properties == nil {
		doc.Properties = map[string]string{}
	}
	doc.CreatedAt = now
	doc.UpdatedAt = now
	if _, err := s.coll.InsertOne(ctx, doc); err != nil {
		return Document{}, fmt.Errorf("insert knowledge: %w", err)
	}
	return doc, nil
}

type scoredDoc struct {
	Document `bson:",inline"`
	Score    float64 `bson:"score"`
}

// Search runs Atlas $vectorSearch (Automated Embedding on content).
func (s *MongoStore) Search(ctx context.Context, params SearchParams) (SearchResult, error) {
	if strings.TrimSpace(params.OrganizationID) == "" {
		return SearchResult{}, fmt.Errorf("organizationId: %w", ErrInvalid)
	}
	k := params.K
	if k <= 0 {
		k = 5
	}
	if k > 50 {
		k = 50
	}
	query := strings.TrimSpace(params.Query)
	if query == "" {
		query = "engineering knowledge"
	}

	filter := bson.M{"organization_id": params.OrganizationID}
	if len(params.Topics) > 0 {
		filter["topics"] = bson.M{"$in": params.Topics}
	}

	limit := int64(k)
	fetchLimit := limit
	authorNeedle := strings.ToLower(strings.TrimSpace(params.Author))
	hasPostFilter := authorNeedle != "" || len(params.Properties) > 0 || params.From != nil || params.To != nil
	if hasPostFilter {
		// Over-fetch before post-filters so author/property/time do not starve k.
		fetchLimit = min(int64(50), max(limit*10, limit))
	}

	pipeline := mongo.Pipeline{
		{{Key: "$vectorSearch", Value: bson.M{
			"index":         VectorIndexName,
			"path":          "content",
			"query":         query,
			"numCandidates": fetchLimit * 20,
			"limit":         fetchLimit,
			"filter":        filter,
		}}},
		{{Key: "$project", Value: bson.M{
			"content": 1, "topics": 1, "properties": 1, "authors": 1,
			"created_at": 1, "updated_at": 1, "organization_id": 1,
			"score": bson.M{"$meta": "vectorSearchScore"},
		}}},
	}
	cursor, err := s.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return SearchResult{}, fmt.Errorf("vector search: %w", err)
	}
	defer cursor.Close(ctx)

	var rows []scoredDoc
	if err := cursor.All(ctx, &rows); err != nil {
		return SearchResult{}, fmt.Errorf("decode vector search: %w", err)
	}

	items := make([]Document, 0, len(rows))
	scores := make([]float64, 0, len(rows))
	for _, row := range rows {
		if !matchAuthor(row.Document, authorNeedle) {
			continue
		}
		if !matchProperties(row.Document, params.Properties) {
			continue
		}
		if !matchTimeRange(row.Document, params.From, params.To) {
			continue
		}
		items = append(items, row.Document)
		scores = append(scores, row.Score)
		if int64(len(items)) >= limit {
			break
		}
	}
	return SearchResult{Items: items, Scores: scores}, nil
}

func matchAuthor(doc Document, needle string) bool {
	if needle == "" {
		return true
	}
	for _, a := range doc.Authors {
		if strings.Contains(strings.ToLower(a.UserID), needle) || strings.Contains(strings.ToLower(a.Name), needle) {
			return true
		}
	}
	return false
}

func matchProperties(doc Document, want map[string]string) bool {
	if len(want) == 0 {
		return true
	}
	for k, v := range want {
		if doc.Properties[k] != v {
			return false
		}
	}
	return true
}

func matchTimeRange(doc Document, from, to *time.Time) bool {
	if from != nil && doc.CreatedAt.Before(*from) {
		return false
	}
	if to != nil && doc.CreatedAt.After(*to) {
		return false
	}
	return true
}
