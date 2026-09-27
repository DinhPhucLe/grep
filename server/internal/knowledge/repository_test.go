package knowledge

import (
	"context"
	"errors"
	"go.mongodb.org/mongo-driver/v2/bson"
	"testing"
)

func TestRepositoryRejectsInvalidCallsBeforeDatabaseAccess(t *testing.T) {
	// Nil collections deliberately prove validation happens before any DB access.
	repo := &MongoRepository{}
	ctx := context.Background()
	owner, project, id := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"zero caller", func() error { _, e := repo.Create(ctx, bson.NilObjectID, project, validInput()); return e }},
		{"zero project", func() error { _, e := repo.Create(ctx, owner, bson.NilObjectID, validInput()); return e }},
		{"invalid content", func() error { _, e := repo.Create(ctx, owner, project, Input{}); return e }},
		{"zero record", func() error { _, e := repo.Get(ctx, owner, project, bson.NilObjectID); return e }},
		{"get zero caller", func() error { _, e := repo.Get(ctx, bson.NilObjectID, project, id); return e }},
		{"get zero project", func() error { _, e := repo.Get(ctx, owner, bson.NilObjectID, id); return e }},
		{"list zero caller", func() error { _, e := repo.List(ctx, bson.NilObjectID, project, 10); return e }},
		{"list zero project", func() error { _, e := repo.List(ctx, owner, bson.NilObjectID, 10); return e }},
		{"zero limit", func() error { _, e := repo.List(ctx, owner, project, 0); return e }},
		{"negative limit", func() error { _, e := repo.List(ctx, owner, project, -1); return e }},
		{"excessive limit", func() error { _, e := repo.List(ctx, owner, project, 101); return e }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("got %v", err)
			}
		})
	}
}
