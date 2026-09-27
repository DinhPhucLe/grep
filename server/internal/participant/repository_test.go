package participant

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type fakeDatabase struct {
	names  []string
	err    error
	filter any
}

func (f *fakeDatabase) ListCollectionNames(_ context.Context, filter any, _ ...options.Lister[options.ListCollectionsOptions]) ([]string, error) {
	f.filter = filter
	return f.names, f.err
}

type fakeCollection struct {
	filter, update any
	calls          int
	upsert         bool
	after          bool
	err            error
}

func (f *fakeCollection) FindOneAndUpdate(_ context.Context, filter, update any, opts ...options.Lister[options.FindOneAndUpdateOptions]) *mongo.SingleResult {
	f.filter = filter
	f.update = update
	f.calls++
	var o options.FindOneAndUpdateOptions
	for _, builder := range opts {
		for _, apply := range builder.List() {
			_ = apply(&o)
		}
	}
	f.upsert = o.Upsert != nil && *o.Upsert
	f.after = o.ReturnDocument != nil && *o.ReturnDocument == options.After
	return mongo.NewSingleResultFromDocument(Profile{ID: testID}, f.err, nil)
}
func (f *fakeCollection) FindOne(_ context.Context, filter any, _ ...options.Lister[options.FindOneOptions]) *mongo.SingleResult {
	f.filter = filter
	f.calls++
	return mongo.NewSingleResultFromDocument(Profile{ID: testID}, f.err, nil)
}

func TestMongoRegisterRequiresExistingCollection(t *testing.T) {
	for _, tc := range []struct {
		name        string
		dbErr, want error
	}{{"not migrated", nil, ErrMigrationRequired}, {"list error", context.DeadlineExceeded, context.DeadlineExceeded}} {
		t.Run(tc.name, func(t *testing.T) {
			db := &fakeDatabase{err: tc.dbErr}
			coll := &fakeCollection{}
			repo := &MongoRepository{database: db, collection: coll}
			_, err := repo.Register(context.Background(), Registration{ID: testID})
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if coll.calls != 0 {
				t.Fatal("wrote before collection was migrated")
			}
			if db.filter.(bson.M)["name"] != "participants" {
				t.Fatalf("wrong collection filter: %v", db.filter)
			}
		})
	}
}

func TestMongoRegisterUsesAtomicUpsertWithoutErasingName(t *testing.T) {
	for _, name := range []string{"", "Ada"} {
		coll := &fakeCollection{}
		repo := &MongoRepository{database: &fakeDatabase{names: []string{"participants"}}, collection: coll}
		before := time.Now().UTC()
		profile, err := repo.Register(context.Background(), Registration{ID: testID, DisplayName: name})
		if err != nil {
			t.Fatal(err)
		}
		if profile.ID != testID || coll.calls != 1 || !coll.upsert || !coll.after {
			t.Fatalf("wrong upsert result/options: %+v %+v", profile, coll)
		}
		if coll.filter.(bson.M)["_id"] != testID {
			t.Fatalf("wrong participant filter: %v", coll.filter)
		}
		update := coll.update.(bson.M)
		set := update["$set"].(bson.M)
		insert := update["$setOnInsert"].(bson.M)
		if _, ok := set["created_at"]; ok {
			t.Fatal("created_at overwritten")
		}
		if insert["created_at"].(time.Time).Before(before) || set["last_seen_at"].(time.Time).Before(before) {
			t.Fatal("timestamps missing")
		}
		if name == "" {
			if _, ok := set["display_name"]; ok {
				t.Fatal("omitted display name erased")
			}
		} else if set["display_name"] != "Ada" {
			t.Fatal("display name not updated")
		}
	}
}

func TestMongoExistsDistinguishesMissingAndFailure(t *testing.T) {
	for _, tc := range []struct {
		err    error
		exists bool
	}{{nil, true}, {mongo.ErrNoDocuments, false}, {context.DeadlineExceeded, false}} {
		coll := &fakeCollection{err: tc.err}
		repo := &MongoRepository{collection: coll}
		exists, err := repo.Exists(context.Background(), testID)
		if exists != tc.exists {
			t.Fatalf("exists=%v want %v", exists, tc.exists)
		}
		if errors.Is(tc.err, mongo.ErrNoDocuments) {
			if err != nil {
				t.Fatal(err)
			}
		} else if !errors.Is(err, tc.err) {
			t.Fatalf("wrong error %v", err)
		}
		if coll.filter.(bson.M)["_id"] != testID {
			t.Fatal("lookup did not scope to participant")
		}
	}
}
