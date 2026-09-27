package practice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestClientPostEvent(t *testing.T) {
	org := bson.NewObjectID()
	user := bson.NewObjectID()
	session := bson.NewObjectID()
	project := bson.NewObjectID()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/practice-events" || r.Method != http.MethodPost {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		var payload IngestPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Practice != PracticeLeadAndReveal || payload.Attempts != 2 {
			t.Fatalf("%+v", payload)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Event{
			ID:       bson.NewObjectID(),
			Practice: payload.Practice,
			Outcome:  payload.Outcome,
			Attempts: payload.Attempts,
		})
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, HTTPClient: server.Client()}
	start := time.Date(2026, 9, 26, 14, 0, 0, 0, time.UTC)
	event, err := client.PostEvent(context.Background(), IngestPayload{
		Practice:           PracticeLeadAndReveal,
		OrganizationID:     org.Hex(),
		UserID:             user.Hex(),
		SessionID:          session.Hex(),
		ProjectID:          project.Hex(),
		StartedAt:          start.Format(time.RFC3339),
		EndedAt:            start.Add(time.Minute).Format(time.RFC3339),
		TotalDurationMs:    60000,
		ActiveAnswerTimeMs: 20000,
		Attempts:           2,
		Outcome:            OutcomeFailedReveal,
		RepoName:           "cortisol-cli",
		RepoOrg:            "DinhPhucLe",
		FilePath:           "a.go",
		Module:             "server",
		StartLine:          1,
		EndLine:            2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.Practice != PracticeLeadAndReveal || event.Outcome != OutcomeFailedReveal {
		t.Fatalf("%+v", event)
	}
}
