package seed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// SEED RESULTS FOR THE COMMAND SUMMARY
type SeedResult struct {
	Inserted int
	Existing int
}

type seedRecord struct {
	collection string
	document   bson.M
}

// DETERMINISTIC IDS KEEP GENERATED RECORDS STABLE ACROSS RUNS
func seedID(kind, user, project, session byte) bson.ObjectID {
	return bson.ObjectID{0x66, 0, 0, 0, 0, 0, 0, 0, kind, user, project, session}
}

// GENERATE FOUR USERS WITH ONE OR TWO PROJECTS AND ONE TO FIVE SESSIONS EACH
func demoRecords() []seedRecord {
	var records []seedRecord
	base := time.Date(2026, time.September, 26, 14, 0, 0, 0, time.UTC)
	projectNumber := 0
	for user := 1; user <= 4; user++ {
		userID := seedID(1, byte(user), 0, 0)
		userName := fmt.Sprintf("user%d", user)
		created := base.Add(time.Duration(user-1) * 24 * time.Hour)
		records = append(records, seedRecord{"users", bson.M{
			"_id": userID, "name": userName, "mail": userName + "@example.com", "created_at": created,
		}})
		for project := 1; project <= 1+user%2; project++ {
			projectNumber++
			projectID := seedID(2, byte(user), byte(project), 0)
			projectName := fmt.Sprintf("project_%d%c", user, 'A'+rune(project-1))
			records = append(records, seedRecord{"projects", bson.M{
				"_id": projectID, "user_id": userID, "name": projectName,
				"root_path": fmt.Sprintf("/demo/%s/%s", userName, projectName),
				"repo_url":  fmt.Sprintf("https://example.com/%s/%s.git", userName, projectName),
			}})
			for session := 1; session <= 1+(projectNumber-1)%5; session++ {
				started := created.Add(time.Duration(project*6+session) * time.Hour)
				records = append(records, seedRecord{"sessions", bson.M{
					"_id": seedID(3, byte(user), byte(project), byte(session)), "project_id": projectID,
					"agent_model": "demo-model", "started_at": started,
					"ended_at": started.Add(time.Duration(15+session*10) * time.Minute),
					"branch":   fmt.Sprintf("demo/session-%d", session), "status": "complete", "shell": "zsh",
				}})
			}
		}
	}
	return records
}

// SEED DEVELOPMENT DATA WITHOUT CHANGING EXISTING DOCUMENTS
func Seed(ctx context.Context, database *mongo.Database) (SeedResult, error) {
	var result SeedResult

	// REQUIRE THE APPLIED SCHEMA BEFORE ANY SEED WRITES
	var migration struct {
		Version int  `bson:"version"`
		Dirty   bool `bson:"dirty"`
	}
	err := database.Collection("schema_migrations").FindOne(ctx, bson.D{}).Decode(&migration)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return result, errors.New("run go run ./cmd/migrate before seeding")
	}
	if err != nil {
		return result, fmt.Errorf("read migration version: %w", err)
	}
	if migration.Dirty || migration.Version < 3 {
		return result, errors.New("seeding requires a clean migration version of at least 3; run or repair migrations first")
	}
	names, err := database.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return result, fmt.Errorf("check seed collections: %w", err)
	}
	collections := make(map[string]bool, len(names))
	for _, name := range names {
		collections[name] = true
	}
	for _, record := range demoRecords() {
		if !collections[record.collection] {
			return result, fmt.Errorf("required collection %s is missing; repair migrations before seeding", record.collection)
		}
	}

	// UPSERT ONLY ON INSERT: RERUNS PRESERVE USER EDITS AND AVOID DUPLICATES
	// Writes are ordered but not transactional; rerun to finish a partial seed.
	for _, record := range demoRecords() {
		update, err := database.Collection(record.collection).UpdateOne(ctx,
			bson.M{"_id": record.document["_id"]},
			bson.M{"$setOnInsert": record.document}, options.UpdateOne().SetUpsert(true))
		if err != nil {
			return result, fmt.Errorf("seed %s: %w", record.collection, err)
		}
		if update.UpsertedCount > 0 {
			result.Inserted++
		} else {
			result.Existing++
		}
	}
	return result, nil
}
