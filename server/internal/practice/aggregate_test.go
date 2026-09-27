package practice

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func sampleEvents(user, org, project bson.ObjectID) []Event {
	day1 := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 1, 15, 0, 0, 0, time.UTC)
	day3 := time.Date(2026, 3, 2, 11, 0, 0, 0, time.UTC)
	mk := func(start time.Time, outcome string, active int64, file, module string) Event {
		return Event{
			ID:                 bson.NewObjectID(),
			Practice:           PracticeLeadAndReveal,
			OrganizationID:     org,
			UserID:             user,
			SessionID:          bson.NewObjectID(),
			ProjectID:          project,
			StartedAt:          start,
			EndedAt:            start.Add(time.Minute),
			TotalDurationMs:    60000,
			ActiveAnswerTimeMs: active,
			Attempts:           1,
			Outcome:            outcome,
			RepoName:           "cortisol-cli",
			RepoOrg:            "DinhPhucLe",
			FilePath:           file,
			Module:             module,
			StartLine:          1,
			EndLine:            10,
		}
	}
	return []Event{
		mk(day1, OutcomeCorrect, 10000, "server/a.go", "server"),
		mk(day2, OutcomeFailedReveal, 20000, "server/b.go", "server"),
		mk(day3, OutcomeCorrect, 30000, "cli/c.go", "cli"),
	}
}

func TestBuildEmployeeViewCalendarAndPie(t *testing.T) {
	user := bson.NewObjectID()
	org := bson.NewObjectID()
	project := bson.NewObjectID()
	view := BuildEmployeeView(EmployeeSubject{
		UserID: user.Hex(), OrganizationID: org.Hex(), Practice: PracticeLeadAndReveal, Year: 2026,
	}, sampleEvents(user, org, project))
	if view.SchemaVersion != "employee_practice.v1" {
		t.Fatal(view.SchemaVersion)
	}
	if view.ActivityCalendar.Status != "available" {
		t.Fatal(view.ActivityCalendar.Status)
	}
	var march1, march2 *CalendarDay
	for i := range view.ActivityCalendar.Days {
		d := &view.ActivityCalendar.Days[i]
		if d.Date == "2026-03-01" {
			march1 = d
		}
		if d.Date == "2026-03-02" {
			march2 = d
		}
	}
	if march1 == nil || march1.Count != 2 || march1.Intensity == 0 {
		t.Fatalf("march1 %+v", march1)
	}
	if march2 == nil || march2.Count != 1 {
		t.Fatalf("march2 %+v", march2)
	}
	if view.OutcomePie.Status != "available" || len(view.OutcomePie.Segments) != 2 {
		t.Fatalf("%+v", view.OutcomePie)
	}
	seg := map[string]int{}
	for _, s := range view.OutcomePie.Segments {
		seg[s.Label] = s.Value
	}
	if seg["correct"] != 2 || seg["failed_reveal"] != 1 {
		t.Fatalf("%v", seg)
	}
	if len(view.Metrics) == 0 {
		t.Fatal("expected metrics")
	}
}

func TestBuildOrgViewTreemapAndTimeseries(t *testing.T) {
	user := bson.NewObjectID()
	org := bson.NewObjectID()
	project := bson.NewObjectID()
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 31, 23, 59, 59, 0, time.UTC)
	view := BuildOrgView(OrgSubject{
		OrganizationID: org.Hex(),
		Practice:       PracticeLeadAndReveal,
		From:           from.Format(time.RFC3339),
		To:             to.Format(time.RFC3339),
	}, sampleEvents(user, org, project))
	if view.SchemaVersion != "org_practice.v1" {
		t.Fatal(view.SchemaVersion)
	}
	if len(view.CodebaseTreemaps) != 1 {
		t.Fatalf("%d", len(view.CodebaseTreemaps))
	}
	tree := view.CodebaseTreemaps[0]
	if tree.RepoName != "cortisol-cli" || tree.Status != "available" || tree.Root.Value != 3 {
		t.Fatalf("%+v", tree)
	}
	if tree.Root.Intensity != 0 {
		t.Fatalf("root intensity want 0 got %d", tree.Root.Intensity)
	}
	moduleIntensity := map[string]int{}
	for _, child := range tree.Root.Children {
		moduleIntensity[child.Name] = child.Intensity
	}
	// server has 2 events, cli has 1 → server should outrank cli when max is 2
	if moduleIntensity["server"] <= moduleIntensity["cli"] {
		t.Fatalf("expected server intensity > cli, got %v", moduleIntensity)
	}
	if view.Timeseries.Status != "available" || len(view.Timeseries.Points) == 0 {
		t.Fatalf("%+v", view.Timeseries)
	}
}
