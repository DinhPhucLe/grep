package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestKnowledgeMigration(t *testing.T) {
	data, err := os.ReadFile("migrations/000007_knowledge_records.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []json.RawMessage
	if err = json.Unmarshal(data, &commands); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 {
		t.Fatal("expected create and index commands")
	}
	var create struct {
		Create           string
		ValidationLevel  string
		ValidationAction string
		Validator        struct {
			Schema struct {
				Required             []string
				AdditionalProperties bool
				Properties           map[string]json.RawMessage
				OneOf                []json.RawMessage
			} `json:"$jsonSchema"`
		}
	}
	if err = json.Unmarshal(commands[0], &create); err != nil {
		t.Fatal(err)
	}
	if create.Create != "knowledge_records" || create.ValidationLevel != "strict" || create.ValidationAction != "error" || create.Validator.Schema.AdditionalProperties {
		t.Fatal("missing strict collection validator")
	}
	if !reflect.DeepEqual(create.Validator.Schema.Required, []string{"_id", "project_id", "source", "topic", "content", "created_at"}) {
		t.Fatal("required fields changed")
	}
	if len(create.Validator.Schema.Properties) != 8 || len(create.Validator.Schema.OneOf) != 2 {
		t.Fatal("expected eight fields and paired embedding alternatives")
	}
	var index struct {
		CreateIndexes string
		Indexes       []struct {
			Name   string
			Key    map[string]int
			Unique bool
		}
	}
	if err = json.Unmarshal(commands[1], &index); err != nil {
		t.Fatal(err)
	}
	if index.CreateIndexes != "knowledge_records" || len(index.Indexes) != 1 || !reflect.DeepEqual(index.Indexes[0].Key, map[string]int{"project_id": 1, "created_at": -1, "_id": -1}) || index.Indexes[0].Unique {
		t.Fatal("missing project ordering index")
	}
	down, err := os.ReadFile("migrations/000007_knowledge_records.down.json")
	if err != nil {
		t.Fatal(err)
	}
	var drops []map[string]string
	if err = json.Unmarshal(down, &drops); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(drops, []map[string]string{{"drop": "knowledge_records"}}) {
		t.Fatal("rollback affects existing collections")
	}
	// Check this pair independently of the pre-existing duplicate version 5.
	dir := t.TempDir()
	for suffix, contents := range map[string][]byte{"up": data, "down": down} {
		if err := os.WriteFile(filepath.Join(dir, "000007_knowledge_records."+suffix+".json"), contents, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := CheckMigrations(dir); err != nil {
		t.Fatal(err)
	}
}

func TestKnowledgeRequiredFromVersion7(t *testing.T) {
	old := []string{"users", "projects", "sessions", "evaluations", "organizations", "organization_members", "practice_events"}
	if got := MissingCollectionsForVersion(6, old); len(got) != 0 {
		t.Fatalf("old baseline broken: %v", got)
	}
	if got := MissingCollectionsForVersion(7, old); !reflect.DeepEqual(got, []string{"knowledge_records"}) {
		t.Fatalf("new collection not required: %v", got)
	}
	if got := MissingCollectionsForVersion(7, append(old, "knowledge_records")); len(got) != 0 {
		t.Fatal(got)
	}
}
