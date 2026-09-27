package db

import (
	"strings"
	"testing"
)

func TestMissingCollectionsForVersion(t *testing.T) {
	have := []string{"users", "projects", "sessions", "evaluations", "schema_migrations"}
	missing := MissingCollectionsForVersion(6, have)
	if strings.Join(missing, ",") != "organization_members,organizations,practice_events" {
		t.Fatalf("got %v", missing)
	}
	complete := append(append([]string{}, have...), "organizations", "organization_members", "practice_events")
	if got := MissingCollectionsForVersion(6, complete); len(got) != 0 {
		t.Fatalf("expected complete schema, got %v", got)
	}
	if got := MissingCollectionsForVersion(5, have); len(got) != 0 {
		t.Fatalf("v5 should not require practice collections: %v", got)
	}
	if got := MissingCollectionsForVersion(4, have); len(got) != 0 {
		t.Fatalf("v4 should not require practice collections: %v", got)
	}
}

func TestSchemaIntegrityErrorMessage(t *testing.T) {
	err := SchemaIntegrityError(6, []string{"organizations"})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, part := range []string{"organizations", "version 6", "force", "5"} {
		if !strings.Contains(msg, part) {
			t.Fatalf("missing %q in %s", part, msg)
		}
	}
	if SchemaIntegrityError(6, nil) != nil {
		t.Fatal("empty missing should be nil")
	}
}
