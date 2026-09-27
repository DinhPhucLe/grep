package practice

import (
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func validEvent() Event {
	start := time.Date(2026, 9, 26, 14, 0, 0, 0, time.UTC)
	return Event{
		Practice:            PracticeLeadAndReveal,
		OrganizationID:      bson.NewObjectID(),
		UserID:              bson.NewObjectID(),
		SessionID:           bson.NewObjectID(),
		ProjectID:           bson.NewObjectID(),
		StartedAt:           start,
		EndedAt:             start.Add(2 * time.Minute),
		TotalDurationMs:     120000,
		ActiveAnswerTimeMs:  45000,
		Attempts:            1,
		Outcome:             OutcomeCorrect,
		RepoName:            "cortisol-cli",
		RepoOrg:             "DinhPhucLe",
		FilePath:            "server/internal/practice/model.go",
		Module:              "server",
		StartLine:           10,
		EndLine:             40,
	}
}

func TestEventValidateAcceptsLeadAndRevealInstance(t *testing.T) {
	if err := validEvent().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestEventValidateRejectsInvalidInstances(t *testing.T) {
	for _, tc := range []struct {
		name string
		mut  func(*Event)
	}{
		{"empty practice", func(e *Event) { e.Practice = "" }},
		{"unknown practice", func(e *Event) { e.Practice = "prompt_rewrite" }},
		{"zero org", func(e *Event) { e.OrganizationID = bson.ObjectID{} }},
		{"zero user", func(e *Event) { e.UserID = bson.ObjectID{} }},
		{"zero session", func(e *Event) { e.SessionID = bson.ObjectID{} }},
		{"ended before start", func(e *Event) { e.EndedAt = e.StartedAt.Add(-time.Second) }},
		{"negative total duration", func(e *Event) { e.TotalDurationMs = -1 }},
		{"negative active answer time", func(e *Event) { e.ActiveAnswerTimeMs = -1 }},
		{"attempts zero", func(e *Event) { e.Attempts = 0 }},
		{"attempts three", func(e *Event) { e.Attempts = 3 }},
		{"bad outcome", func(e *Event) { e.Outcome = "partial" }},
		{"empty repo", func(e *Event) { e.RepoName = " " }},
		{"empty file", func(e *Event) { e.FilePath = "" }},
		{"empty module", func(e *Event) { e.Module = " " }},
		{"start line zero", func(e *Event) { e.StartLine = 0 }},
		{"end before start line", func(e *Event) { e.StartLine = 20; e.EndLine = 10 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := validEvent()
			tc.mut(&e)
			if err := e.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestEventValidateAllowsNullPointsAndReservedQuality(t *testing.T) {
	e := validEvent()
	e.PointsDelta = nil
	e.AnswerQuality = nil
	e.Relevance = nil
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	delta := 5
	e.PointsDelta = &delta
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestEventValidateRejectsBlankReservedQualityWhenSet(t *testing.T) {
	e := validEvent()
	blank := " "
	e.AnswerQuality = &blank
	if err := e.Validate(); err == nil || !strings.Contains(err.Error(), "answer_quality") {
		t.Fatalf("got %v", err)
	}
}
