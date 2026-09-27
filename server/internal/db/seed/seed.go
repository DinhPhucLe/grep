package seed

import (
	"context"
	"errors"
	"fmt"

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
// Layout: [0x66][org][0..][kind][user][project][session]
func seedID(kind, org, user, project, session byte) bson.ObjectID {
	return bson.ObjectID{0x66, org, 0, 0, 0, 0, 0, 0, kind, user, project, session}
}

// PRACTICE EVENTS NEED A 16-BIT SEQUENCE BEYOND THE 4-BYTE KIND/USER/PROJECT/SESSION SLOT
// Layout: [0x66][org][seqHi][seqLo][0..][kind=6][user][0][0]
func seedEventID(org, user byte, seq uint16) bson.ObjectID {
	return bson.ObjectID{0x66, org, byte(seq >> 8), byte(seq), 0, 0, 0, 0, 6, user, 0, 0}
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
	if migration.Dirty || migration.Version < 5 {
		return result, errors.New("seeding requires a clean migration version of at least 5; run or repair migrations first")
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
			return result, fmt.Errorf("required collection %s is missing; migration version claims %d but schema objects are incomplete; reconcile deployed schema and migration history before repairing or seeding; do not automatically force or reset the migration ledger", record.collection, migration.Version)
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
