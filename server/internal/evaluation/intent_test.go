package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestConversationPersistsWithNullAmbiguity(t *testing.T) {
	for _, input := range []string{"yes", "go ahead", "thanks", "what does this mean?", "are you done?", "individually"} {
		t.Run(input, func(t *testing.T) {
			client := &fakeCompleter{raw: `{"verdict":"not_applicable","summary":"Conversation, not a new implementation request.","ambiguity_score":null,"gaps":[]}`}
			repo := &memoryRepository{}
			record, err := NewService(client, repo, "test").Evaluate(context.Background(), Request{Input: input})
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(record.Evaluation)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			value, exists := fields["ambiguity_score"]
			if !exists || value != nil || len(repo.records) != 1 || record.ID.IsZero() {
				t.Fatalf("unrated conversation lost: %s", encoded)
			}
			if repo.records[0].ID != record.ID || repo.records[0].Request.Input != input || repo.records[0].Evaluation.AmbiguityScore != nil {
				t.Fatal("saved evaluation does not match the null-score response")
			}
			stored, err := bson.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			var decoded Record
			if err := bson.Unmarshal(stored, &decoded); err != nil || decoded.Evaluation.Validate() != nil {
				t.Fatalf("BSON round trip failed: %v", err)
			}
			if bson.Raw(stored).Lookup("evaluation", "ambiguity_score").Type != bson.TypeNull {
				t.Fatal("stored score must be explicit BSON null")
			}
		})
	}
}

func TestUnratedHTTPResponseIsCreatedAndSaved(t *testing.T) {
	client := &fakeCompleter{raw: `{"verdict":"not_applicable","summary":"Confirmation","ambiguity_score":null,"gaps":[]}`}
	repo := &memoryRepository{}
	handler := NewHandler(NewService(client, repo, "test"), time.Second)
	r := httptest.NewRequest("POST", "/evaluations", strings.NewReader(`{"input":"yes"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, r)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	if w.Code != 201 || fields["id"] == nil || len(repo.records) != 1 || repo.records[0].ID.IsZero() {
		t.Fatalf("unrated response was not persisted: %d %s", w.Code, w.Body)
	}
}

func TestUnratedStorageFailureIsNotReportedAsSuccess(t *testing.T) {
	client := &fakeCompleter{raw: `{"verdict":"not_applicable","summary":"Confirmation","ambiguity_score":null,"gaps":[]}`}
	repo := &memoryRepository{err: errors.New("database unavailable")}
	service := NewService(client, repo, "test")
	if _, err := service.Evaluate(context.Background(), Request{Input: "yes"}); !errors.Is(err, ErrStorage) {
		t.Fatalf("null-score persistence failure was hidden: %v", err)
	}
	r := httptest.NewRequest("POST", "/evaluations", strings.NewReader(`{"input":"yes"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	NewHandler(service, time.Second)(w, r)
	if w.Code != 500 || strings.Contains(w.Body.String(), "database unavailable") {
		t.Fatalf("unexpected storage failure response: %d %s", w.Code, w.Body)
	}
}
