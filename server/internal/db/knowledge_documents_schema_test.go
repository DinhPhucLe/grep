package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestKnowledgeDocumentsMigrationIsolated(t *testing.T) {
	data, err := os.ReadFile("migrations/000012_knowledge_documents.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []map[string]json.RawMessage
	if err := json.Unmarshal(data, &commands); err != nil {
		t.Fatal(err)
	}
	if len(commands) < 2 {
		t.Fatal("expected create + indexes")
	}
	down, err := os.ReadFile("migrations/000012_knowledge_documents.down.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(down, &commands); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	for _, suffix := range []string{"up", "down"} {
		contents, err := os.ReadFile("migrations/000012_knowledge_documents." + suffix + ".json")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "000012_knowledge_documents."+suffix+".json"), contents, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := CheckMigrations(dir); err != nil {
		t.Fatal(err)
	}
	baseline := []string{"users", "projects", "sessions", "evaluations", "organizations", "organization_members", "practice_events", "knowledge_records"}
	if missing := MissingCollectionsForVersion(12, baseline); len(missing) == 0 || missing[0] != "knowledge_documents" {
		t.Fatalf("expected knowledge_documents missing, got %v", missing)
	}
	if missing := MissingCollectionsForVersion(12, append(baseline, "knowledge_documents")); len(missing) != 0 {
		t.Fatalf("unexpected missing %v", missing)
	}
}
