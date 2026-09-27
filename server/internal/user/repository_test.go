package user

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"
)

func TestExistingUserLookupIsReadOnly(t *testing.T) {
	u := bson.NewObjectID()
	for _, found := range []bool{true, false} {
		batch := bson.A{}
		if found {
			batch = append(batch, bson.D{{Key: "_id", Value: u}})
		}
		response := bson.D{{Key: "ok", Value: 1}, {Key: "cursor", Value: bson.D{{Key: "id", Value: int64(0)}, {Key: "ns", Value: "test.users"}, {Key: "firstBatch", Value: batch}}}}
		var calls []bson.Raw
		opts := options.Client().SetMonitor(&event.CommandMonitor{Started: func(_ context.Context, e *event.CommandStartedEvent) {
			if e.CommandName == "endSessions" {
				return
			}
			if e.CommandName != "find" {
				t.Errorf("unexpected write/command: %s", e.CommandName)
			}
			calls = append(calls, append(bson.Raw(nil), e.Command...))
		}})
		opts.Deployment = drivertest.NewMockDeployment(response)
		client, err := mongo.Connect(opts)
		if err != nil {
			t.Fatal(err)
		}
		repo := NewMongoRepository(client.Database("test"))
		if ok, err := repo.Exists(context.Background(), u.Hex()); err != nil || ok != found {
			t.Fatalf("user lookup: %v %v", ok, err)
		}
		client.Disconnect(context.Background())
		if len(calls) != 1 {
			t.Fatalf("calls: %d", len(calls))
		}
		if calls[0].Lookup("find").StringValue() != "users" || calls[0].Lookup("filter").Document().Lookup("_id").ObjectID() != u {
			t.Fatal("wrong users lookup")
		}
	}
}

func TestInvalidIDsNeverReachDatabase(t *testing.T) {
	repo := NewMongoRepository(nil)
	for _, id := range []string{"", "not-an-id", "000000000000000000000000", "66F600000000000000000001"} {
		if ok, err := repo.Exists(context.Background(), id); ok || err != nil {
			t.Fatalf("accepted invalid ID %q", id)
		}
	}
}
