package practice

import (
	"context"
	"testing"
	"time"

	"cortisol-server/internal/quiz"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"
)

func projectionCursor(collection string, docs ...bson.D) bson.D {
	batch := bson.A{}
	for _, doc := range docs {
		batch = append(batch, doc)
	}
	return bson.D{{Key: "ok", Value: 1}, {Key: "cursor", Value: bson.D{
		{Key: "id", Value: int64(0)}, {Key: "ns", Value: "test." + collection}, {Key: "firstBatch", Value: batch},
	}}}
}

func TestQuizAnswerProjectionUsesImmutableIdempotentUpsert(t *testing.T) {
	org := bson.NewObjectID()
	answer := quiz.AnswerRecord{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), QuizID: bson.NewObjectID(), QuestionID: "q2", Graded: 0.5, CreatedAt: time.Now().UTC().Truncate(time.Millisecond)}
	var updates []bson.Raw
	opts := options.Client().SetMonitor(&event.CommandMonitor{Started: func(_ context.Context, e *event.CommandStartedEvent) {
		if e.CommandName == "update" {
			updates = append(updates, append(bson.Raw(nil), e.Command...))
		}
	}})
	membership := projectionCursor("organization_members", bson.D{{Key: "organization_id", Value: org}})
	ok := bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}, {Key: "nModified", Value: 0}}
	opts.Deployment = drivertest.NewMockDeployment(membership, ok, membership, ok)
	client, err := mongo.Connect(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	projector := NewQuizAnswerProjector(client.Database("test"), org)
	for i := 0; i < 2; i++ {
		if err := projector.SyncAnswer(context.Background(), answer); err != nil {
			t.Fatal(err)
		}
	}
	if len(updates) != 2 {
		t.Fatalf("want two retry attempts, got %d", len(updates))
	}
	for _, command := range updates {
		var decoded struct {
			Updates []struct {
				Query  bson.M `bson:"q"`
				Update bson.M `bson:"u"`
				Upsert bool   `bson:"upsert"`
			} `bson:"updates"`
		}
		if err := bson.Unmarshal(command, &decoded); err != nil {
			t.Fatal(err)
		}
		update := decoded.Updates[0]
		if !update.Upsert || update.Query["_id"] != answer.ID || update.Query["user_id"] != answer.UserID || update.Query["source"] != SourceQuizAnswer {
			t.Fatalf("unsafe upsert: %+v", update)
		}
		doc := bson.M{}
		for _, field := range update.Update["$setOnInsert"].(bson.D) {
			doc[field.Key] = field.Value
		}
		if len(update.Update) != 1 || doc["organization_id"] != org || doc["grade"] != 0.5 || doc["outcome"] != OutcomeFailedReveal {
			t.Fatalf("incorrect event: %+v", doc)
		}
		for _, unknown := range []string{"session_id", "project_id", "active_answer_time_ms", "attempts", "file_path"} {
			if _, exists := doc[unknown]; exists {
				t.Fatalf("invented %s", unknown)
			}
		}
	}
}

func TestQuizAnswerOrganizationAttribution(t *testing.T) {
	defaultOrg, otherOrg, thirdOrg := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	for _, tc := range []struct {
		name    string
		members []bson.ObjectID
		want    bson.ObjectID
	}{
		{"login organization", []bson.ObjectID{otherOrg, defaultOrg}, defaultOrg},
		{"sole membership", []bson.ObjectID{otherOrg}, otherOrg},
		{"ambiguous", []bson.ObjectID{otherOrg, thirdOrg}, bson.NilObjectID},
		{"no membership", nil, bson.NilObjectID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			docs := []bson.D{}
			for _, id := range tc.members {
				docs = append(docs, bson.D{{Key: "organization_id", Value: id}})
			}
			opts := options.Client()
			opts.Deployment = drivertest.NewMockDeployment(projectionCursor("organization_members", docs...))
			client, err := mongo.Connect(opts)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Disconnect(context.Background())
			got, err := NewQuizAnswerProjector(client.Database("test"), defaultOrg).organization(context.Background(), bson.NewObjectID())
			if err != nil || got != tc.want {
				t.Fatalf("organization=%s want=%s err=%v", got.Hex(), tc.want.Hex(), err)
			}
		})
	}
}
