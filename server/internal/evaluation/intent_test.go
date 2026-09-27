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

func TestConversationHasNoAmbiguity(t *testing.T) {
	for _, input := range []string{"yes", "go ahead", "thanks", "what does this mean?", "are you done?", "individually"} {
		t.Run(input, func(t *testing.T) {
			client := &fakeCompleter{raw: `{"verdict":"not_applicable","summary":"Conversation, not a new implementation request.","ambiguity_score":null,"gaps":[]}`}
			repo := &memoryRepository{err: errors.New("unrated results must not reach storage")}
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
			if !exists || value != nil || len(repo.records) != 0 {
				t.Fatalf("unrated conversation lost: %s", encoded)
			}
			stored, err := bson.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			var decoded Record
			if err := bson.Unmarshal(stored, &decoded); err != nil || decoded.Evaluation.Validate() != nil {
				t.Fatalf("BSON round trip failed: %v", err)
			}
		})
	}
}

func TestUnratedHTTPResponseIsNotCreatedOrSaved(t *testing.T) {
	client := &fakeCompleter{raw: `{"verdict":"not_applicable","summary":"Confirmation","ambiguity_score":null,"gaps":[]}`}
	repo := &memoryRepository{err: errors.New("storage should not be reached")}
	handler := NewHandler(NewService(client, repo, "test"), time.Second)
	r := httptest.NewRequest("POST", "/evaluations", strings.NewReader(`{"input":"yes"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, r)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || fields["id"] != nil || len(repo.records) != 0 {
		t.Fatalf("unrated response pretends to be persisted: %d %s", w.Code, w.Body)
	}
}
