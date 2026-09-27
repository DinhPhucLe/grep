package orgknowledge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type memoryStore struct {
	docs []Document
}

func (m *memoryStore) Insert(_ context.Context, doc Document) (Document, error) {
	if doc.ID.IsZero() {
		doc.ID = bson.NewObjectID()
	}
	now := time.Now().UTC()
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = now
	}
	if doc.UpdatedAt.IsZero() {
		doc.UpdatedAt = now
	}
	m.docs = append(m.docs, doc)
	return doc, nil
}

func (m *memoryStore) Search(_ context.Context, params SearchParams) (SearchResult, error) {
	k := params.K
	if k <= 0 {
		k = 5
	}
	var items []Document
	var scores []float64
	for i, doc := range m.docs {
		if params.OrganizationID != "" && doc.OrganizationID != params.OrganizationID {
			continue
		}
		items = append(items, doc)
		scores = append(scores, 1.0-float64(i)*0.01)
		if len(items) >= k {
			break
		}
	}
	return SearchResult{Items: items, Scores: scores}, nil
}

func TestKnowledgeHandlerGetDefaultsKToFive(t *testing.T) {
	store := &memoryStore{}
	for i := 0; i < 8; i++ {
		store.docs = append(store.docs, Document{
			ID:             bson.NewObjectID(),
			Content:        "payment retry backoff note",
			Topics:         []string{"payments"},
			Properties:     map[string]string{},
			Authors:        []Author{{UserID: "u"}},
			OrganizationID: "org-novapay",
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		})
	}
	handler := NewHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge?organizationId=org-novapay&query=payment+retry", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var got SearchResult
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 5 {
		t.Fatalf("want 5 items got %d", len(got.Items))
	}
	if len(got.Scores) != 5 {
		t.Fatalf("want 5 scores got %d", len(got.Scores))
	}
	for _, item := range got.Items {
		if item.OrganizationID != "org-novapay" {
			t.Fatalf("leaked org %q", item.OrganizationID)
		}
	}
}

func TestKnowledgeHandlerMethodNotAllowed(t *testing.T) {
	handler := NewHandler(&memoryStore{})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/knowledge", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", w.Code)
	}
}

func TestKnowledgeHandlerPostCreatesDocument(t *testing.T) {
	store := &memoryStore{}
	handler := NewHandler(store)
	body := `{
		"content": "Use exponential backoff with jitter on 429/503.",
		"topics": ["payments", "retries"],
		"properties": {"repo": "novapay/novapay-api"},
		"authors": [{"userId": "alexr", "name": "Alex Rivera"}],
		"organizationId": "org-novapay"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var got Document
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID.IsZero() {
		t.Fatal("expected id")
	}
	if got.Content != "Use exponential backoff with jitter on 429/503." {
		t.Fatalf("content %q", got.Content)
	}
	if got.OrganizationID != "org-novapay" {
		t.Fatalf("organizationId %q", got.OrganizationID)
	}
	if len(got.Topics) != 2 || got.Topics[0] != "payments" {
		t.Fatalf("topics %#v", got.Topics)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps %#v %#v", got.CreatedAt, got.UpdatedAt)
	}
	if len(store.docs) != 1 {
		t.Fatalf("stored %d", len(store.docs))
	}
}

func TestKnowledgeHandlerPostRejectsInvalid(t *testing.T) {
	store := &memoryStore{}
	handler := NewHandler(store)
	for _, tc := range []struct {
		name string
		body string
	}{
		{"missing organizationId", `{"content":"ok","topics":["t"],"authors":[{"userId":"u"}]}`},
		{"empty content", `{"content":"  ","topics":["t"],"authors":[{"userId":"u"}],"organizationId":"org"}`},
		{"empty topics", `{"content":"ok","topics":[],"authors":[{"userId":"u"}],"organizationId":"org"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status %d body %s", w.Code, w.Body.String())
			}
			var envelope struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Error.Code != "invalid_request" || envelope.Error.Message == "" {
				t.Fatalf("%+v", envelope)
			}
			if len(store.docs) != 0 {
				t.Fatalf("stored on invalid: %d", len(store.docs))
			}
		})
	}
}

func TestKnowledgeHandlerGetRequiresOrganizationID(t *testing.T) {
	handler := NewHandler(&memoryStore{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge?query=payment+retry", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "invalid_request" {
		t.Fatalf("%+v", envelope)
	}
}
