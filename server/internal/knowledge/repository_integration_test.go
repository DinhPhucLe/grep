package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Explicit opt-in only: never load .env or use the application's database.
func TestMongoRepositoryIntegration(t *testing.T) {
	uri, prefix := os.Getenv("KNOWLEDGE_TEST_MONGODB_URI"), os.Getenv("KNOWLEDGE_TEST_DATABASE")
	if uri == "" && prefix == "" {
		t.Skip("set KNOWLEDGE_TEST_MONGODB_URI and KNOWLEDGE_TEST_DATABASE=cortisol_test_<name> for disposable MongoDB tests")
	}
	if uri == "" || !strings.HasPrefix(prefix, "cortisol_test_") || len(prefix) > 35 {
		t.Fatal("require test URI and database prefix cortisol_test_ (at most 35 characters)")
	}
	for _, c := range prefix {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			t.Fatal("test database prefix must contain only lowercase letters, digits, underscores")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(uri).SetServerSelectionTimeout(5 * time.Second))
	if err != nil {
		t.Fatal("cannot initialize test MongoDB client")
	}
	defer func() {
		cleanup, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		_ = client.Disconnect(cleanup)
	}()
	if err = client.Ping(ctx, nil); err != nil {
		t.Fatal("cannot connect to explicitly selected test MongoDB")
	}
	// A unique suffix prevents test cleanup from touching a pre-existing database.
	database := client.Database(prefix + "_" + bson.NewObjectID().Hex())
	if err = database.CreateCollection(ctx, "projects"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		if err := database.Drop(cleanup); err != nil {
			t.Errorf("drop disposable database: %v", err)
		}
	}()
	data, err := os.ReadFile("../db/migrations/000011_knowledge_records.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []json.RawMessage
	if err = json.Unmarshal(data, &commands); err != nil {
		t.Fatal(err)
	}
	for _, raw := range commands {
		var command bson.D
		if err = bson.UnmarshalExtJSON(raw, false, &command); err != nil {
			t.Fatal(err)
		}
		if err = database.RunCommand(ctx, command).Err(); err != nil {
			t.Fatal(err)
		}
	}
	owner, foreign, project, otherProject := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	_, err = database.Collection("projects").InsertMany(ctx, []any{bson.M{"_id": project, "user_id": owner}, bson.M{"_id": otherProject, "user_id": owner}})
	if err != nil {
		t.Fatal(err)
	}
	repo := NewMongoRepository(database)
	input := validInput()
	plain, err := repo.Create(ctx, owner, project, input)
	if err != nil {
		t.Fatal(err)
	}
	if plain.ID.IsZero() || plain.ProjectID != project || plain.CreatedAt.IsZero() || plain.CreatedAt.Location() != time.UTC {
		t.Fatal("missing server fields")
	}
	input.Embedding = []float64{1, -0.5}
	input.EmbeddingModel = "synthetic/test-only"
	embedded, err := repo.Create(ctx, owner, project, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, original := range []Record{plain, embedded} {
		got, err := repo.Get(ctx, owner, project, original.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Content != input.Content || got.Source != input.Source || got.Topic != input.Topic || got.EmbeddingModel != original.EmbeddingModel || len(got.Embedding) != len(original.Embedding) {
			t.Fatal("round trip changed record")
		}
		if len(got.Embedding) > 0 && got.Embedding[1] != -0.5 {
			t.Fatal("vector changed")
		}
	}
	for _, who := range []bson.ObjectID{foreign, bson.NewObjectID()} {
		if _, err = repo.Create(ctx, who, project, input); !errors.Is(err, ErrNotFound) {
			t.Fatalf("foreign create: %v", err)
		}
		if _, err = repo.Get(ctx, who, project, plain.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("foreign get: %v", err)
		}
		if _, err = repo.List(ctx, who, project, 10); !errors.Is(err, ErrNotFound) {
			t.Fatalf("foreign list: %v", err)
		}
	}
	if _, err = repo.Create(ctx, owner, bson.NewObjectID(), input); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing project: %v", err)
	}
	if _, err = repo.Get(ctx, owner, otherProject, plain.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-project record: %v", err)
	}
	if _, err = repo.Get(ctx, owner, project, bson.NewObjectID()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing record: %v", err)
	}
	records, err := repo.List(ctx, owner, otherProject, 10)
	if err != nil || len(records) != 0 || records == nil {
		t.Fatalf("empty project: %v %v", records, err)
	}
	// Force a timestamp tie to exercise deterministic _id ordering.
	_, err = database.Collection("knowledge_records").UpdateMany(ctx, bson.M{"project_id": project}, bson.M{"$set": bson.M{"created_at": plain.CreatedAt}})
	if err != nil {
		t.Fatal(err)
	}
	records, err = repo.List(ctx, owner, project, 1)
	if err != nil || len(records) != 1 || records[0].ID != embedded.ID {
		t.Fatalf("limited newest-first list: %v %v", records, err)
	}
	records, err = repo.List(ctx, owner, project, 100)
	if err != nil || len(records) != 2 || records[1].ID != plain.ID {
		t.Fatalf("ordered list: %v %v", records, err)
	}
	// Exercise the actual MongoDB validator, including the paired optional fields.
	for _, kind := range []string{"missing source", "unknown field", "model only", "vector only", "empty vector", "wrong vector type"} {
		t.Run(kind, func(t *testing.T) {
			doc := bson.M{"_id": bson.NewObjectID(), "project_id": project, "source": "test", "topic": "test", "content": "text", "created_at": time.Now().UTC()}
			switch kind {
			case "missing source":
				delete(doc, "source")
			case "unknown field":
				doc["extra"] = true
			case "model only":
				doc["embedding_model"] = "synthetic"
			case "vector only":
				doc["embedding"] = []float64{1}
			case "empty vector":
				doc["embedding"] = []float64{}
				doc["embedding_model"] = "synthetic"
			case "wrong vector type":
				doc["embedding"] = []string{"1"}
				doc["embedding_model"] = "synthetic"
			}
			_, err := database.Collection("knowledge_records").InsertOne(ctx, doc)
			var writeErr mongo.WriteException
			if !errors.As(err, &writeErr) || !writeErr.HasErrorCode(121) {
				t.Fatalf("expected document validation error: %v", err)
			}
		})
	}
	// Ownership is resolved on every operation, rather than cached in records.
	_, err = database.Collection("projects").UpdateOne(ctx, bson.M{"_id": project}, bson.M{"$set": bson.M{"user_id": foreign}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Get(ctx, owner, project, plain.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("former owner still authorized: %v", err)
	}
	if _, err = repo.Get(ctx, foreign, project, plain.ID); err != nil {
		t.Fatalf("current owner denied: %v", err)
	}
}
