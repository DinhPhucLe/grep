package seed

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// VERIFY COUNTS, STABLE IDS, PARENT ORDER, AND FEATURED DEMO PATTERNS
func TestDemoRecords(t *testing.T) {
	records := demoRecords()
	if len(records) == 0 {
		t.Fatal("expected seed records")
	}
	second := demoRecords()
	if len(second) != len(records) {
		t.Fatal("seed generation must be deterministic")
	}
	for i := range records {
		if records[i].collection != second[i].collection {
			t.Fatal("seed generation must be deterministic")
		}
		a := records[i].document["_id"].(bson.ObjectID)
		b := second[i].document["_id"].(bson.ObjectID)
		if a != b {
			t.Fatal("seed generation must be deterministic")
		}
	}

	seen := map[bson.ObjectID]bool{}
	users := map[bson.ObjectID]bool{}
	projects := map[bson.ObjectID]bool{}
	orgs := map[bson.ObjectID]bool{}
	counts := map[string]int{}
	eventsByUser := map[bson.ObjectID]int{}
	eventsByOrg := map[bson.ObjectID]int{}
	orgMonths := map[bson.ObjectID]map[time.Month]bool{}
	orgOutcomes := map[bson.ObjectID]map[string]int{}

	for _, record := range records {
		id := record.document["_id"].(bson.ObjectID)
		if seen[id] {
			t.Fatalf("duplicate seed ID %s in %s", id.Hex(), record.collection)
		}
		seen[id] = true
		counts[record.collection]++
		switch record.collection {
		case "users":
			users[id] = true
		case "projects":
			parent := record.document["user_id"].(bson.ObjectID)
			if !users[parent] {
				t.Fatal("project owner must precede project")
			}
			projects[id] = true
		case "sessions":
			parent := record.document["project_id"].(bson.ObjectID)
			if !projects[parent] {
				t.Fatal("project must precede session")
			}
			if !record.document["ended_at"].(time.Time).After(record.document["started_at"].(time.Time)) {
				t.Fatal("session must end after it starts")
			}
		case "organizations":
			orgs[id] = true
		case "organization_members":
			if !users[record.document["user_id"].(bson.ObjectID)] {
				t.Fatal("member user must precede membership")
			}
			if !orgs[record.document["organization_id"].(bson.ObjectID)] {
				t.Fatal("organization must precede membership")
			}
		case "practice_events":
			if !record.document["ended_at"].(time.Time).After(record.document["started_at"].(time.Time)) {
				t.Fatal("practice event must end after it starts")
			}
			userID := record.document["user_id"].(bson.ObjectID)
			orgID := record.document["organization_id"].(bson.ObjectID)
			eventsByUser[userID]++
			eventsByOrg[orgID]++
			started := record.document["started_at"].(time.Time)
			if orgMonths[orgID] == nil {
				orgMonths[orgID] = map[time.Month]bool{}
			}
			orgMonths[orgID][started.Month()] = true
			if orgOutcomes[orgID] == nil {
				orgOutcomes[orgID] = map[string]int{}
			}
			orgOutcomes[orgID][record.document["outcome"].(string)]++
			if record.document["practice"].(string) != "lead_and_reveal" {
				t.Fatal("practice must be lead_and_reveal")
			}
			attempts := record.document["attempts"].(int)
			if attempts < 1 || attempts > 2 {
				t.Fatal("attempts must be 1 or 2")
			}
		}
		data, err := bson.Marshal(record.document)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"created_at", "started_at", "ended_at", "joined_at"} {
			if _, ok := record.document[field]; ok && bson.Raw(data).Lookup(field).Type != bson.TypeDateTime {
				t.Fatalf("%s must be a BSON date", field)
			}
		}
	}

	if counts["organizations"] != 2 {
		t.Fatalf("expected 2 organizations, got %d", counts["organizations"])
	}
	if counts["users"] != 50 {
		t.Fatalf("expected 50 users, got %d", counts["users"])
	}
	if counts["organization_members"] != 50 {
		t.Fatalf("expected 50 members, got %d", counts["organization_members"])
	}
	if counts["projects"] != 6 {
		t.Fatalf("expected 6 shared projects, got %d", counts["projects"])
	}
	if counts["sessions"] != 6 {
		t.Fatalf("expected 6 sessions, got %d", counts["sessions"])
	}
	if counts["practice_events"] < 800 {
		t.Fatalf("expected rich practice volume (>=800), got %d", counts["practice_events"])
	}

	cast := DemoCastIDs()
	if !users[cast.AlexRiveraID] || !users[cast.JordanKimID] || !users[cast.SamOkonkwoID] {
		t.Fatal("featured employees must exist")
	}
	if eventsByUser[cast.AlexRiveraID] < 70 {
		t.Fatalf("Alex should have dense calendar (>=70), got %d", eventsByUser[cast.AlexRiveraID])
	}
	if eventsByUser[cast.JordanKimID] < 40 {
		t.Fatalf("Jordan should have mid volume (>=40), got %d", eventsByUser[cast.JordanKimID])
	}
	if eventsByUser[cast.SamOkonkwoID] < 80 {
		t.Fatalf("Sam should have high volume (>=80), got %d", eventsByUser[cast.SamOkonkwoID])
	}

	for _, orgID := range []bson.ObjectID{cast.NovaPayOrgID, cast.AtlasHealthOrgID} {
		if eventsByOrg[orgID] < 400 {
			t.Fatalf("org %s should have substantial events (>=400), got %d", orgID.Hex(), eventsByOrg[orgID])
		}
		if len(orgMonths[orgID]) < 6 {
			t.Fatalf("org %s events should span many months, got %d", orgID.Hex(), len(orgMonths[orgID]))
		}
		if orgOutcomes[orgID]["correct"] == 0 || orgOutcomes[orgID]["failed_reveal"] == 0 {
			t.Fatalf("org %s must include both outcomes", orgID.Hex())
		}
	}
}

func TestDemoCastIDsStable(t *testing.T) {
	a := DemoCastIDs()
	b := DemoCastIDs()
	if a != b {
		t.Fatal("DemoCastIDs must be stable")
	}
	if a.NovaPayOrgID == a.AtlasHealthOrgID {
		t.Fatal("orgs must differ")
	}
	if a.AlexRiveraID == a.JordanKimID || a.AlexRiveraID == a.SamOkonkwoID {
		t.Fatal("featured employees must differ")
	}
}
