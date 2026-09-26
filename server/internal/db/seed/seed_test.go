package seed

import (
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// VERIFY COUNTS, STABLE IDS, PARENT ORDER, AND RELATIONSHIP CARDINALITY
func TestDemoRecords(t *testing.T) {
	records := demoRecords()
	if !reflect.DeepEqual(records, demoRecords()) {
		t.Fatal("seed generation must be deterministic")
	}
	seen := map[bson.ObjectID]bool{}
	users := map[bson.ObjectID]int{}
	projects := map[bson.ObjectID]int{}
	counts := map[string]int{}
	for _, record := range records {
		id := record.document["_id"].(bson.ObjectID)
		if seen[id] {
			t.Fatal("duplicate seed ID")
		}
		seen[id] = true
		counts[record.collection]++
		switch record.collection {
		case "users":
			users[id] = 0
		case "projects":
			parent := record.document["user_id"].(bson.ObjectID)
			if _, ok := users[parent]; !ok {
				t.Fatal("project owner must precede project")
			}
			users[parent]++
			projects[id] = 0
		case "sessions":
			parent := record.document["project_id"].(bson.ObjectID)
			if _, ok := projects[parent]; !ok {
				t.Fatal("project must precede session")
			}
			projects[parent]++
			if !record.document["ended_at"].(time.Time).After(record.document["started_at"].(time.Time)) {
				t.Fatal("session must end after it starts")
			}
		}
		data, err := bson.Marshal(record.document)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"created_at", "started_at", "ended_at"} {
			if _, ok := record.document[field]; ok && bson.Raw(data).Lookup(field).Type != bson.TypeDateTime {
				t.Fatalf("%s must be a BSON date", field)
			}
		}
	}
	if !reflect.DeepEqual(counts, map[string]int{"users": 4, "projects": 6, "sessions": 16}) {
		t.Fatalf("unexpected counts: %v", counts)
	}
	for _, count := range users {
		if count < 1 || count > 2 {
			t.Fatal("each user must own 1–2 projects")
		}
	}
	for _, count := range projects {
		if count < 1 || count > 5 {
			t.Fatal("each project must have 1–5 sessions")
		}
	}
}
