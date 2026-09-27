package db

import (
	"strings"
	"testing"
)

func TestMissingCollectionsForVersion(t *testing.T) {
	have := []string{"users", "projects", "sessions", "evaluations", "schema_migrations"}
	missing := MissingCollectionsForVersion(5, have)
	if strings.Join(missing, ",") != "organization_members,organizations,practice_events" {
		t.Fatalf("got %v", missing)
	}
	complete := append(append([]string{}, have...), "organizations", "organization_members", "practice_events")
	if got := MissingCollectionsForVersion(5, complete); len(got) != 0 {
		t.Fatalf("expected complete schema, got %v", got)
	}
	if got := MissingCollectionsForVersion(4, have); len(got) != 0 {
		t.Fatalf("v4 should not require practice collections: %v", got)
	}
}

func TestSchemaIntegrityErrorMessage(t *testing.T) {
	err := SchemaIntegrityError(5, []string{"organizations"})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, part := range []string{"organizations", "version 5", "reconcile"} {
		if !strings.Contains(msg, part) {
			t.Fatalf("missing %q in %s", part, msg)
		}
	}
	if strings.Contains(msg, "force schema_migrations") {
		t.Fatal("must not recommend forcing migration ledger")
	}
	if SchemaIntegrityError(5, nil) != nil {
		t.Fatal("empty missing should be nil")
	}
}
