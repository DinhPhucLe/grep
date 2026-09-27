package practice

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Directory interface {
	RecommendPeople(ctx context.Context, query string, limit int) ([]PersonRecommendation, error)
	RecommendOrganizations(ctx context.Context, query string, limit int) ([]OrgRecommendation, error)
	LookupUserName(ctx context.Context, userID bson.ObjectID) (string, error)
	LookupOrganizationName(ctx context.Context, orgID bson.ObjectID) (string, error)
}

type MongoDirectory struct {
	users    *mongo.Collection
	orgs     *mongo.Collection
	connects *mongo.Collection
	events   *mongo.Collection
}

func NewMongoDirectory(database *mongo.Database) *MongoDirectory {
	return &MongoDirectory{
		users:    database.Collection("users"),
		orgs:     database.Collection("organizations"),
		connects: database.Collection("knowledge_connects"),
		events:   database.Collection("practice_events"),
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
	if q := strings.TrimSpace(query); q != "" {
		return d.searchPeople(ctx, q, limit)
	}
	return d.recommendActivePeople(ctx, limit)
}

func (d *MongoDirectory) searchPeople(ctx context.Context, query string, limit int) ([]PersonRecommendation, error) {
	pattern := regexp.QuoteMeta(query)
	filter := bson.M{
		"$or": []bson.M{
			{"name": bson.M{"$regex": pattern, "$options": "i"}},
			{"mail": bson.M{"$regex": pattern, "$options": "i"}},
		},
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
	return peopleFromDocs(docs), nil
}

// recommendActivePeople ranks users with learning edges first, then practice
// events. Newest created_at alone put inert legacy/github accounts on top.
func (d *MongoDirectory) recommendActivePeople(ctx context.Context, limit int) ([]PersonRecommendation, error) {
	scores := map[string]int64{}
	bumpHex := func(hex string, score int64) {
		hex = strings.TrimSpace(hex)
		if hex == "" {
			return
		}
		if cur, ok := scores[hex]; !ok || score > cur {
			scores[hex] = score
		}
	}
	bumpOID := func(id bson.ObjectID, score int64) {
		if id.IsZero() {
			return
		}
		bumpHex(id.Hex(), score)
	}

	if err := d.scanConnectUserScores(ctx, bumpHex); err != nil {
		return nil, err
	}
	if err := d.scanPracticeUserScores(ctx, bumpOID); err != nil {
		return nil, err
	}

	ranked := rankedHexes(scores)
	items := make([]PersonRecommendation, 0, limit)
	seen := map[string]struct{}{}
	for _, hex := range ranked {
		if len(items) >= limit {
			break
		}
		oid, err := bson.ObjectIDFromHex(hex)
		if err != nil {
			continue
		}
		var doc userDoc
		err = d.users.FindOne(ctx, bson.M{"_id": oid}).Decode(&doc)
		if err == mongo.ErrNoDocuments {
			continue
		}
		if err != nil {
			return nil, err
		}
		seen[hex] = struct{}{}
		items = append(items, PersonRecommendation{ID: doc.ID.Hex(), Name: doc.Name, Mail: doc.Mail})
	}
	if len(items) >= limit {
		return items, nil
	}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit * 2))
	cursor, err := d.users.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var docs []userDoc
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	for _, doc := range docs {
		if len(items) >= limit {
			break
		}
		hex := doc.ID.Hex()
		if _, ok := seen[hex]; ok {
			continue
		}
		seen[hex] = struct{}{}
		items = append(items, PersonRecommendation{ID: doc.ID.Hex(), Name: doc.Name, Mail: doc.Mail})
	}
	return items, nil
}

func (d *MongoDirectory) scanConnectUserScores(ctx context.Context, bump func(string, int64)) error {
	cursor, err := d.connects.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{
		"seeker_user_id": 1, "author_user_id": 1, "created_at": 1,
	}).SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(500))
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	const connectBoost int64 = 1_000_000_000_000
	for cursor.Next(ctx) {
		var row bson.M
		if err := cursor.Decode(&row); err != nil {
			return err
		}
		score := mongoTimeUnix(row["created_at"]) + connectBoost
		if seeker, ok := row["seeker_user_id"].(string); ok {
			bump(seeker, score)
		}
		if author, ok := row["author_user_id"].(string); ok {
			bump(author, score)
		}
	}
	return cursor.Err()
}

func (d *MongoDirectory) scanPracticeUserScores(ctx context.Context, bump func(bson.ObjectID, int64)) error {
	cursor, err := d.events.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{
		"user_id": 1, "started_at": 1,
	}).SetSort(bson.D{{Key: "started_at", Value: -1}}).SetLimit(500))
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var row bson.M
		if err := cursor.Decode(&row); err != nil {
			return err
		}
		uid, ok := row["user_id"].(bson.ObjectID)
		if !ok {
			continue
		}
		bump(uid, mongoTimeUnix(row["started_at"]))
	}
	return cursor.Err()
}

func (d *MongoDirectory) RecommendOrganizations(ctx context.Context, query string, limit int) ([]OrgRecommendation, error) {
	if limit <= 0 {
		limit = DefaultRecommendationLimit
	}
	if q := strings.TrimSpace(query); q != "" {
		pattern := regexp.QuoteMeta(q)
		filter := bson.M{"name": bson.M{"$regex": pattern, "$options": "i"}}
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
	return d.recommendActiveOrganizations(ctx, limit)
}

func (d *MongoDirectory) recommendActiveOrganizations(ctx context.Context, limit int) ([]OrgRecommendation, error) {
	scores := map[string]int64{}
	bump := func(hex string, score int64) {
		hex = strings.TrimSpace(hex)
		if hex == "" {
			return
		}
		if cur, ok := scores[hex]; !ok || score > cur {
			scores[hex] = score
		}
	}
	const connectBoost int64 = 1_000_000_000_000

	cursor, err := d.connects.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{
		"organization_id": 1, "created_at": 1,
	}).SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(500))
	if err != nil {
		return nil, err
	}
	for cursor.Next(ctx) {
		var row bson.M
		if err := cursor.Decode(&row); err != nil {
			_ = cursor.Close(ctx)
			return nil, err
		}
		org, _ := row["organization_id"].(string)
		bump(org, mongoTimeUnix(row["created_at"])+connectBoost)
	}
	if err := cursor.Close(ctx); err != nil {
		return nil, err
	}

	eventCursor, err := d.events.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{
		"organization_id": 1, "started_at": 1,
	}).SetSort(bson.D{{Key: "started_at", Value: -1}}).SetLimit(500))
	if err != nil {
		return nil, err
	}
	for eventCursor.Next(ctx) {
		var row bson.M
		if err := eventCursor.Decode(&row); err != nil {
			_ = eventCursor.Close(ctx)
			return nil, err
		}
		switch v := row["organization_id"].(type) {
		case bson.ObjectID:
			bump(v.Hex(), mongoTimeUnix(row["started_at"]))
		case string:
			bump(v, mongoTimeUnix(row["started_at"]))
		}
	}
	if err := eventCursor.Close(ctx); err != nil {
		return nil, err
	}

	ranked := rankedHexes(scores)
	items := make([]OrgRecommendation, 0, limit)
	seen := map[string]struct{}{}
	for _, hex := range ranked {
		if len(items) >= limit {
			break
		}
		oid, err := bson.ObjectIDFromHex(hex)
		if err != nil {
			continue
		}
		var doc orgDoc
		err = d.orgs.FindOne(ctx, bson.M{"_id": oid}).Decode(&doc)
		if err == mongo.ErrNoDocuments {
			continue
		}
		if err != nil {
			return nil, err
		}
		seen[doc.ID.Hex()] = struct{}{}
		items = append(items, OrgRecommendation{ID: doc.ID.Hex(), Name: doc.Name})
	}
	if len(items) >= limit {
		return items, nil
	}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit * 2))
	fill, err := d.orgs.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer fill.Close(ctx)
	var docs []orgDoc
	if err := fill.All(ctx, &docs); err != nil {
		return nil, err
	}
	for _, doc := range docs {
		if len(items) >= limit {
			break
		}
		if _, ok := seen[doc.ID.Hex()]; ok {
			continue
		}
		items = append(items, OrgRecommendation{ID: doc.ID.Hex(), Name: doc.Name})
	}
	return items, nil
}

func peopleFromDocs(docs []userDoc) []PersonRecommendation {
	items := make([]PersonRecommendation, 0, len(docs))
	for _, doc := range docs {
		items = append(items, PersonRecommendation{ID: doc.ID.Hex(), Name: doc.Name, Mail: doc.Mail})
	}
	return items
}

func rankedHexes(scores map[string]int64) []string {
	type pair struct {
		hex   string
		score int64
	}
	ranked := make([]pair, 0, len(scores))
	for hex, score := range scores {
		ranked = append(ranked, pair{hex, score})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].hex < ranked[j].hex
		}
		return ranked[i].score > ranked[j].score
	})
	out := make([]string, 0, len(ranked))
	for _, p := range ranked {
		out = append(out, p.hex)
	}
	return out
}

func mongoTimeUnix(value any) int64 {
	switch v := value.(type) {
	case time.Time:
		return v.Unix()
	case bson.DateTime:
		return int64(v) / 1000
	default:
		return 0
	}
}

func (d *MongoDirectory) LookupUserName(ctx context.Context, userID bson.ObjectID) (string, error) {
	var doc userDoc
	err := d.users.FindOne(ctx, bson.M{"_id": userID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return doc.Name, nil
}

func (d *MongoDirectory) LookupOrganizationName(ctx context.Context, orgID bson.ObjectID) (string, error) {
	var doc orgDoc
	err := d.orgs.FindOne(ctx, bson.M{"_id": orgID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return doc.Name, nil
}
