package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestKnowledgeConnectsMigrationIsolated(t *testing.T) {
	for _, suffix := range []string{"up", "down"} {
		data, err := os.ReadFile("migrations/000014_knowledge_connects." + suffix + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var commands []map[string]json.RawMessage
		if err := json.Unmarshal(data, &commands); err != nil {
			t.Fatal(err)
		}
		if len(commands) == 0 {
			t.Fatal("empty migration")
		}
	}
	dir := t.TempDir()
	for _, suffix := range []string{"up", "down"} {
		contents, err := os.ReadFile("migrations/000014_knowledge_connects." + suffix + ".json")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "000014_knowledge_connects."+suffix+".json"), contents, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := CheckMigrations(dir); err != nil {
		t.Fatal(err)
	}
	have := []string{
		"users", "projects", "sessions", "evaluations", "organizations",
		"organization_members", "practice_events", "knowledge_records",
		"knowledge_documents", "auth_sessions",
	}
	if missing := MissingCollectionsForVersion(14, have); len(missing) != 1 || missing[0] != "knowledge_connects" {
		t.Fatalf("expected knowledge_connects missing, got %v", missing)
	}
	have = append(have, "knowledge_connects")
	if missing := MissingCollectionsForVersion(14, have); len(missing) != 0 {
		t.Fatalf("unexpected missing %v", missing)
	}
}
