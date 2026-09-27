package orgknowledge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

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
