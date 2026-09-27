package seed

import (
	"path/filepath"
	"runtime"
	"testing"

	"cortisol-server/internal/orgknowledge"
)

func TestLoadKnowledgeRowsFromSampleExport(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	path := filepath.Join(filepath.Dir(file), "testdata", "github_export_sample.json")
	rows, err := loadKnowledgeRows(KnowledgeSeedOptions{ExportPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 5 {
		t.Fatalf("expected sample rows, got %d", len(rows))
	}
	cast := DemoCastIDs()
	docs, err := orgknowledge.MapExportRows(rows, DemoKnowledgeAuthors(), cast.NovaPayOrgID.Hex(), cast.AtlasHealthOrgID.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != len(rows) {
		t.Fatal("doc count mismatch")
	}
	nova, atlas := 0, 0
	for _, d := range docs {
		switch d.OrganizationID {
		case cast.NovaPayOrgID.Hex():
			nova++
		case cast.AtlasHealthOrgID.Hex():
			atlas++
		default:
			t.Fatalf("unexpected org %s", d.OrganizationID)
		}
	}
	if nova == 0 || atlas == 0 {
		t.Fatalf("expected both orgs, nova=%d atlas=%d", nova, atlas)
	}
}

func TestDemoKnowledgeAuthorsCount(t *testing.T) {
	authors := DemoKnowledgeAuthors()
	if len(authors) != employeesPerOrg*2 {
		t.Fatalf("got %d authors", len(authors))
	}
}
