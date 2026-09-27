package orgknowledge

import (
	"context"
	"fmt"
	"sort"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TopicCount is a pitch dashboard bucket.
type TopicCount struct {
	Topic string `json:"topic"`
	Count int    `json:"count"`
}

// AuthorCount is a pitch dashboard bucket.
type AuthorCount struct {
	UserID string `json:"userId"`
	Name   string `json:"name,omitempty"`
	Count  int    `json:"count"`
}

// RepoCount is a pitch dashboard bucket.
type RepoCount struct {
	Repo  string `json:"repo"`
	Count int    `json:"count"`
}

// RecentItem is a short preview card for the pitch UI.
type RecentItem struct {
	ID        string   `json:"id"`
	Preview   string   `json:"preview"`
	Topics    []string `json:"topics"`
	Author    string   `json:"author,omitempty"`
	CreatedAt string   `json:"createdAt"`
}

// OrgSummary is the dashboard pitch aggregate for one organization.
type OrgSummary struct {
	OrganizationID   string        `json:"organizationId"`
	OrganizationName string        `json:"organizationName,omitempty"`
	TotalDocuments   int           `json:"totalDocuments"`
	TopicCounts      []TopicCount  `json:"topicCounts"`
	AuthorCounts     []AuthorCount `json:"authorCounts"`
	RepoCounts       []RepoCount   `json:"repoCounts"`
	Recent           []RecentItem  `json:"recent"`
}

// Summarizer loads org knowledge aggregates for the dashboard.
type Summarizer struct {
	database *mongo.Database
}

func NewSummarizer(database *mongo.Database) *Summarizer {
	return &Summarizer{database: database}
}

// OrgSummary builds pitch metadata for organizationID (hex org id string).
func (s *Summarizer) OrgSummary(ctx context.Context, organizationID string) (OrgSummary, error) {
	if organizationID == "" {
		return OrgSummary{}, fmt.Errorf("organization id required: %w", ErrInvalid)
	}
	coll := s.database.Collection(CollectionName)
	filter := bson.M{"organization_id": organizationID}
	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return OrgSummary{}, fmt.Errorf("count knowledge: %w", err)
	}

	name := ""
	if oid, err := bson.ObjectIDFromHex(organizationID); err == nil {
		var org struct {
			Name string `bson:"name"`
		}
		if err := s.database.Collection("organizations").FindOne(ctx, bson.M{"_id": oid}).Decode(&org); err == nil {
			name = org.Name
		}
	}

	summary := OrgSummary{
		OrganizationID:   organizationID,
		OrganizationName: name,
		TotalDocuments:   int(total),
		TopicCounts:      []TopicCount{},
		AuthorCounts:     []AuthorCount{},
		RepoCounts:       []RepoCount{},
		Recent:           []RecentItem{},
	}
	if total == 0 {
		return summary, nil
	}

	topics, err := aggregateTopics(ctx, coll, organizationID)
	if err != nil {
		return OrgSummary{}, err
	}
	summary.TopicCounts = topics

	authors, err := aggregateAuthors(ctx, coll, organizationID)
	if err != nil {
		return OrgSummary{}, err
	}
	summary.AuthorCounts = authors

	repos, err := aggregateRepos(ctx, coll, organizationID)
	if err != nil {
		return OrgSummary{}, err
	}
	summary.RepoCounts = repos

	recent, err := recentItems(ctx, coll, organizationID, 8)
	if err != nil {
		return OrgSummary{}, err
	}
	summary.Recent = recent
	return summary, nil
}

func aggregateTopics(ctx context.Context, coll *mongo.Collection, orgID string) ([]TopicCount, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"organization_id": orgID}}},
		{{Key: "$unwind", Value: "$topics"}},
		{{Key: "$group", Value: bson.M{"_id": "$topics", "count": bson.M{"$sum": 1}}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}, {Key: "_id", Value: 1}}}},
		{{Key: "$limit", Value: 40}},
	}
	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("aggregate topics: %w", err)
	}
	defer cursor.Close(ctx)
	var rows []struct {
		ID    string `bson:"_id"`
		Count int    `bson:"count"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	out := make([]TopicCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, TopicCount{Topic: r.ID, Count: r.Count})
	}
	return out, nil
}

func aggregateAuthors(ctx context.Context, coll *mongo.Collection, orgID string) ([]AuthorCount, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"organization_id": orgID}}},
		{{Key: "$unwind", Value: "$authors"}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$authors.user_id",
			"name":  bson.M{"$first": "$authors.name"},
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}, {Key: "_id", Value: 1}}}},
		{{Key: "$limit", Value: 25}},
	}
	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("aggregate authors: %w", err)
	}
	defer cursor.Close(ctx)
	var rows []struct {
		ID    string `bson:"_id"`
		Name  string `bson:"name"`
		Count int    `bson:"count"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	out := make([]AuthorCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, AuthorCount{UserID: r.ID, Name: r.Name, Count: r.Count})
	}
	return out, nil
}

func aggregateRepos(ctx context.Context, coll *mongo.Collection, orgID string) ([]RepoCount, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"organization_id": orgID}}},
		{{Key: "$group", Value: bson.M{
			"_id":   bson.M{"$ifNull": bson.A{"$properties.repo", "unknown"}},
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}, {Key: "_id", Value: 1}}}},
		{{Key: "$limit", Value: 25}},
	}
	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("aggregate repos: %w", err)
	}
	defer cursor.Close(ctx)
	var rows []struct {
		ID    string `bson:"_id"`
		Count int    `bson:"count"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	out := make([]RepoCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, RepoCount{Repo: r.ID, Count: r.Count})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Repo < out[j].Repo
	})
	return out, nil
}

func recentItems(ctx context.Context, coll *mongo.Collection, orgID string, limit int64) ([]RecentItem, error) {
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit)
	cursor, err := coll.Find(ctx, bson.M{"organization_id": orgID}, opts)
	if err != nil {
		return nil, fmt.Errorf("recent knowledge: %w", err)
	}
	defer cursor.Close(ctx)
	var docs []Document
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]RecentItem, 0, len(docs))
	for _, d := range docs {
		author := ""
		if len(d.Authors) > 0 {
			author = d.Authors[0].Name
			if author == "" {
				author = d.Authors[0].UserID
			}
		}
		out = append(out, RecentItem{
			ID:        d.ID.Hex(),
			Preview:   preview(d.Content, 160),
			Topics:    d.Topics,
			Author:    author,
			CreatedAt: d.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return out, nil
}

func preview(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "…"
}
