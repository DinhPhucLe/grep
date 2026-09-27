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

type memoryConnectStore struct {
	edges []Connect
}

func (m *memoryConnectStore) InsertConnects(_ context.Context, edges []Connect) (int, error) {
	inserted := 0
	for _, e := range edges {
		dup := false
		for _, existing := range m.edges {
			if existing.SeekerUserID == e.SeekerUserID &&
				existing.DocumentID == e.DocumentID &&
				existing.AuthorUserID == e.AuthorUserID &&
				existing.SessionID == e.SessionID {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		if e.ID.IsZero() {
			e.ID = bson.NewObjectID()
		}
		m.edges = append(m.edges, e)
		inserted++
	}
	return inserted, nil
}

func (m *memoryConnectStore) ListByUserYear(_ context.Context, userID string, year int) ([]Connect, error) {
	from := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(1, 0, 0)
	var out []Connect
	for _, e := range m.edges {
		if e.CreatedAt.Before(from) || !e.CreatedAt.Before(to) {
			continue
		}
		if e.SeekerUserID == userID || e.AuthorUserID == userID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *memoryConnectStore) ListByOrgRange(_ context.Context, orgID string, from, to time.Time) ([]Connect, error) {
	var out []Connect
	for _, e := range m.edges {
		if e.OrganizationID != orgID {
			continue
		}
		if e.CreatedAt.Before(from) || e.CreatedAt.After(to) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func TestConnectHandlerCreatesEdges(t *testing.T) {
	p := testPrincipal()
	authorID := bson.NewObjectID().Hex()
	doc := Document{
		ID:             bson.NewObjectID(),
		Content:        "retry with jitter",
		Topics:         []string{"payments", "reliability"},
		Properties:     map[string]string{},
		Authors:        []Author{{UserID: authorID, Name: "Jordan"}},
		OrganizationID: p.OrganizationID.Hex(),
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	docs := &memoryStore{docs: []Document{doc}}
	connects := &memoryConnectStore{}
	handler := NewConnectHandler(docs, connects)

	body := `{"documentId":"` + doc.ID.Hex() + `","sessionId":"thread-1"}`
	req := withPrincipal(httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/connects", strings.NewReader(body)), p)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var got connectResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Inserted != 1 || len(got.Edges) != 1 {
		t.Fatalf("want 1 edge got inserted=%d edges=%d", got.Inserted, len(got.Edges))
	}
	if got.Edges[0].AuthorUserID != authorID || got.Edges[0].SeekerUserID != p.UserID.Hex() {
		t.Fatalf("unexpected edge %+v", got.Edges[0])
	}

	// Idempotent reconnect in same session.
	w2 := httptest.NewRecorder()
	req2 := withPrincipal(httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/connects", strings.NewReader(body)), p)
	req2.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("idempotent status %d", w2.Code)
	}
}

func TestBuildEmployeeKnowledgeViewCountsInAndOut(t *testing.T) {
	user := "user-a"
	year := 2026
	edges := []Connect{
		{SeekerUserID: user, AuthorUserID: "b", SessionID: "s1", CreatedAt: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC), OrganizationID: "org"},
		{SeekerUserID: "c", AuthorUserID: user, SessionID: "s2", CreatedAt: time.Date(2026, 3, 1, 13, 0, 0, 0, time.UTC), OrganizationID: "org"},
		{SeekerUserID: user, AuthorUserID: "d", SessionID: "s1", CreatedAt: time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC), OrganizationID: "org"},
	}
	view := BuildEmployeeKnowledgeView(EmployeeKnowledgeSubject{UserID: user, Year: year}, edges)
	if view.ActivityCalendar.Status != "available" || len(view.ActivityCalendar.Days) != 2 {
		t.Fatalf("calendar %+v", view.ActivityCalendar)
	}
	if len(view.Metrics) != 4 {
		t.Fatalf("metrics %d", len(view.Metrics))
	}
}

func TestBuildOrgKnowledgeViewTopicNetworkAndDots(t *testing.T) {
	docID := bson.NewObjectID()
	edges := []Connect{
		{
			OrganizationID: "org",
			SeekerUserID:   "a",
			AuthorUserID:   "b",
			DocumentID:     docID.Hex(),
			SessionID:      "s1",
			Topics:         []string{"payments", "retries"},
			CreatedAt:      time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
		},
		{
			OrganizationID: "org",
			SeekerUserID:   "c",
			AuthorUserID:   "b",
			DocumentID:     docID.Hex(),
			SessionID:      "s2",
			Topics:         []string{"payments", "retries"},
			CreatedAt:      time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC),
		},
	}
	docs := []Document{{
		ID:             docID,
		Topics:         []string{"payments", "retries"},
		Authors:        []Author{{UserID: "b", Name: "Jordan"}},
		CreatedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		OrganizationID: "org",
	}}
	view := BuildOrgKnowledgeView(OrgKnowledgeSubject{OrganizationID: "org"}, edges, docs)
	if view.TopicNetwork.Status != "available" || len(view.TopicNetwork.Nodes) != 2 || len(view.TopicNetwork.Links) != 1 {
		t.Fatalf("network %+v", view.TopicNetwork)
	}
	if view.LearningDots.Status != "available" || len(view.LearningDots.Items) != 1 {
		t.Fatalf("dots %+v", view.LearningDots)
	}
	if view.LearningDots.Items[0].AuthorName != "Jordan" {
		t.Fatalf("author name %q", view.LearningDots.Items[0].AuthorName)
	}
}
