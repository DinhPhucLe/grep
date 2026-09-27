package orgknowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ConnectStore persists learning-edge connect events.
type ConnectStore interface {
	InsertConnects(ctx context.Context, edges []Connect) (inserted int, err error)
	ListByUserYear(ctx context.Context, userID string, year int) ([]Connect, error)
	ListByOrgRange(ctx context.Context, orgID string, from, to time.Time) ([]Connect, error)
}

// DocumentByID loads one knowledge document (connect expansion).
type DocumentByID interface {
	FindByID(ctx context.Context, id string) (Document, error)
}

// DocumentLookup loads knowledge documents by id for connect expansion and org dots.
type DocumentLookup interface {
	DocumentByID
	FindByIDs(ctx context.Context, ids []string) ([]Document, error)
}

// MongoConnectStore implements ConnectStore against knowledge_connects.
type MongoConnectStore struct {
	coll *mongo.Collection
}

// NewMongoConnectStore returns a ConnectStore backed by the application database.
func NewMongoConnectStore(database *mongo.Database) *MongoConnectStore {
	return &MongoConnectStore{coll: database.Collection(ConnectCollectionName)}
}

// InsertConnects upserts edges; duplicate unique-key rows are skipped.
func (s *MongoConnectStore) InsertConnects(ctx context.Context, edges []Connect) (int, error) {
	if len(edges) == 0 {
		return 0, nil
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	inserted := 0
	for i := range edges {
		edge := edges[i]
		if edge.ID.IsZero() {
			edge.ID = bson.NewObjectID()
		}
		if edge.CreatedAt.IsZero() {
			edge.CreatedAt = now
		}
		if edge.Topics == nil {
			edge.Topics = []string{}
		}
		filter := bson.M{
			"seeker_user_id": edge.SeekerUserID,
			"document_id":    edge.DocumentID,
			"author_user_id": edge.AuthorUserID,
			"session_id":     edge.SessionID,
		}
		update := bson.M{"$setOnInsert": edge}
		res, err := s.coll.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))
		if err != nil {
			return inserted, fmt.Errorf("insert connect: %w", err)
		}
		if res.UpsertedCount > 0 {
			inserted++
		}
	}
	return inserted, nil
}

// ListByUserYear returns edges where the user is seeker or author in the UTC year.
func (s *MongoConnectStore) ListByUserYear(ctx context.Context, userID string, year int) ([]Connect, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("userId: %w", ErrInvalid)
	}
	from := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(1, 0, 0)
	filter := bson.M{
		"created_at": bson.M{"$gte": from, "$lt": to},
		"$or": []bson.M{
			{"seeker_user_id": userID},
			{"author_user_id": userID},
		},
	}
	cursor, err := s.coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list connects by user: %w", err)
	}
	defer cursor.Close(ctx)
	var rows []Connect
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("decode connects: %w", err)
	}
	return rows, nil
}

// ListByOrgRange returns connects for an organization in [from, to].
func (s *MongoConnectStore) ListByOrgRange(ctx context.Context, orgID string, from, to time.Time) ([]Connect, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return nil, fmt.Errorf("organizationId: %w", ErrInvalid)
	}
	filter := bson.M{
		"organization_id": orgID,
		"created_at":      bson.M{"$gte": from.UTC(), "$lte": to.UTC()},
	}
	cursor, err := s.coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list connects by org: %w", err)
	}
	defer cursor.Close(ctx)
	var rows []Connect
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("decode connects: %w", err)
	}
	return rows, nil
}

// FindByID loads one knowledge document by hex ObjectId.
func (s *MongoStore) FindByID(ctx context.Context, id string) (Document, error) {
	oid, err := bson.ObjectIDFromHex(strings.TrimSpace(id))
	if err != nil {
		return Document{}, ErrNotFound
	}
	var doc Document
	err = s.coll.FindOne(ctx, bson.M{"_id": oid}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, fmt.Errorf("find knowledge: %w", err)
	}
	return doc, nil
}

// FindByIDs loads knowledge documents by hex ObjectIds (order not preserved).
func (s *MongoStore) FindByIDs(ctx context.Context, ids []string) ([]Document, error) {
	oids := make([]bson.ObjectID, 0, len(ids))
	for _, id := range ids {
		oid, err := bson.ObjectIDFromHex(strings.TrimSpace(id))
		if err != nil {
			continue
		}
		oids = append(oids, oid)
	}
	if len(oids) == 0 {
		return nil, nil
	}
	cursor, err := s.coll.Find(ctx, bson.M{"_id": bson.M{"$in": oids}})
	if err != nil {
		return nil, fmt.Errorf("find knowledge by ids: %w", err)
	}
	defer cursor.Close(ctx)
	var docs []Document
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("decode knowledge: %w", err)
	}
	return docs, nil
}
