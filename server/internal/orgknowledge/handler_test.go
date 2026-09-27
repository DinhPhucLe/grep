package orgknowledge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cortisol-server/internal/identity"

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

func testPrincipal() identity.Principal {
	orgID, _ := bson.ObjectIDFromHex("660100000000000004000000")
	return identity.Principal{
		UserID:         bson.NewObjectID(),
		Name:           "Alex Rivera",
		GitHubLogin:    "alexr",
		OrganizationID: orgID,
		OrgName:        "NovaPay",
	}
}

func withPrincipal(r *http.Request, p identity.Principal) *http.Request {
	return r.WithContext(identity.WithPrincipal(r.Context(), p))
}

func TestKnowledgeHandlerGetDefaultsKToFive(t *testing.T) {
	p := testPrincipal()
	store := &memoryStore{}
	for i := 0; i < 8; i++ {
		store.docs = append(store.docs, Document{
			ID:             bson.NewObjectID(),
			Content:        "payment retry backoff note",
			Topics:         []string{"payments"},
			Properties:     map[string]string{},
			Authors:        []Author{{UserID: "u"}},
			OrganizationID: p.OrganizationID.Hex(),
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		})
	}
	handler := NewHandler(store, nil)
	req := withPrincipal(httptest.NewRequest(http.MethodGet, "/api/v1/knowledge?query=payment+retry", nil), p)
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
		if item.OrganizationID != p.OrganizationID.Hex() {
			t.Fatalf("leaked org %q", item.OrganizationID)
		}
	}
}

func TestKnowledgeHandlerUnauthorized(t *testing.T) {
	handler := NewHandler(&memoryStore{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", w.Code)
	}
}

func TestKnowledgeHandlerMethodNotAllowed(t *testing.T) {
	handler := NewHandler(&memoryStore{}, nil)
	req := withPrincipal(httptest.NewRequest(http.MethodDelete, "/api/v1/knowledge", nil), testPrincipal())
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", w.Code)
	}
}

func TestKnowledgeHandlerPostCreatesDocument(t *testing.T) {
	p := testPrincipal()
	store := &memoryStore{}
	hub := NewHub()
	handler := NewHandler(store, hub)
	body := `{
		"content": "Use exponential backoff with jitter on 429/503.",
		"topics": ["payments", "retries"],
		"properties": {"repo": "novapay/novapay-api"}
	}`
	req := withPrincipal(httptest.NewRequest(http.MethodPost, "/api/v1/knowledge", strings.NewReader(body)), p)
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
	if got.OrganizationID != p.OrganizationID.Hex() {
		t.Fatalf("organizationId %q", got.OrganizationID)
	}
	if len(got.Authors) != 1 || got.Authors[0].UserID != p.UserID.Hex() {
		t.Fatalf("authors %#v", got.Authors)
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

func TestKnowledgeHandlerPostIgnoresClientAuthors(t *testing.T) {
	p := testPrincipal()
	store := &memoryStore{}
	handler := NewHandler(store, nil)
	body := `{
		"content": "spoof attempt",
		"topics": ["t"],
		"authors": [{"userId": "attacker", "name": "Evil"}],
		"organizationId": "other-org"
	}`
	req := withPrincipal(httptest.NewRequest(http.MethodPost, "/api/v1/knowledge", strings.NewReader(body)), p)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var got Document
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Authors[0].UserID != p.UserID.Hex() {
		t.Fatalf("spoofed author %q", got.Authors[0].UserID)
	}
	if got.OrganizationID != p.OrganizationID.Hex() {
		t.Fatalf("spoofed org %q", got.OrganizationID)
	}
}

func TestKnowledgeHandlerPostRejectsInvalid(t *testing.T) {
	p := testPrincipal()
	store := &memoryStore{}
	handler := NewHandler(store, nil)
	for _, tc := range []struct {
		name string
		body string
	}{
		{"empty content", `{"content":"  ","topics":["t"]}`},
		{"empty topics", `{"content":"ok","topics":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := withPrincipal(httptest.NewRequest(http.MethodPost, "/api/v1/knowledge", strings.NewReader(tc.body)), p)
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

func TestKnowledgeHandlerGetRejectsOrgMismatch(t *testing.T) {
	p := testPrincipal()
	handler := NewHandler(&memoryStore{}, nil)
	req := withPrincipal(httptest.NewRequest(http.MethodGet, "/api/v1/knowledge?organizationId=other", nil), p)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d", w.Code)
	}
}
