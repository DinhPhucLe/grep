package orgknowledge

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestMapExportRowsDemoAttribution(t *testing.T) {
	nova := "660100000000000004000000"
	atlas := "660200000000000004000000"
	authors := []DemoAuthor{
		{UserID: "u1", Name: "Alex Rivera", OrgID: nova},
		{UserID: "u2", Name: "Sam Okonkwo", OrgID: atlas},
	}
	rows := []GitHubExportRow{
		{
			EventID: "1", EventType: "IssuesEvent", RepoName: "novapay/novapay-api",
			ActorLogin: "x", Title: "Pay", Body: "payment retry stripe checkout",
			CreatedAt: "2026-01-02T00:00:00Z", Labels: []string{"payments"},
		},
		{
			EventID: "2", EventType: "IssuesEvent", RepoName: "atlashealth/atlas-core",
			ActorLogin: "y", Title: "FHIR", Body: "patient search organizationId",
			CreatedAt: "2026-01-03T00:00:00Z", Labels: []string{"fhir"},
		},
	}
	docs, err := MapExportRows(rows, authors, nova, atlas)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatalf("got %d docs", len(docs))
	}
	if docs[0].OrganizationID != nova {
		t.Fatalf("nova org: got %s", docs[0].OrganizationID)
	}
	if docs[1].OrganizationID != atlas {
		t.Fatalf("atlas org: got %s", docs[1].OrganizationID)
	}
	if docs[0].Properties["source"] != "snowflake_github" {
		t.Fatal("expected source property")
	}
	if docs[0].Authors[0].Name == "" {
		t.Fatal("expected author name")
	}
	second, err := MapExportRows(rows, authors, nova, atlas)
	if err != nil {
		t.Fatal(err)
	}
	if docs[0].ID != second[0].ID {
		t.Fatal("mapping must be deterministic by event id")
	}
}

func TestGenerateSyntheticExportCount(t *testing.T) {
	rows := GenerateSyntheticExport(25)
	if len(rows) != 25 {
		t.Fatalf("got %d", len(rows))
	}
	if rows[0].EventID != "synth-00001" {
		t.Fatalf("unexpected id %s", rows[0].EventID)
	}
	again := GenerateSyntheticExport(25)
	if rows[10].Title != again[10].Title {
		t.Fatal("synthetic export must be deterministic")
	}
}

func TestVectorIndexDefinition(t *testing.T) {
	def := VectorIndexDefinition()
	fields, ok := def["fields"].(bson.A)
	if !ok || len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %#v", def["fields"])
	}
	first, ok := fields[0].(bson.M)
	if !ok {
		t.Fatal("first field")
	}
	if first["type"] != "autoEmbed" || first["model"] != "voyage-code-4" || first["path"] != "content" {
		t.Fatalf("unexpected autoEmbed field: %#v", first)
	}
}
