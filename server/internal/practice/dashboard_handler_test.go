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

type dashViewFunc struct {
	employee func(context.Context, string, string, int) (EmployeePracticeView, error)
	org      func(context.Context, string, string, time.Time, time.Time) (OrgPracticeView, error)
	people   func(context.Context, string, int) ([]PersonRecommendation, error)
	orgs     func(context.Context, string, int) ([]OrgRecommendation, error)
}

func (v dashViewFunc) EmployeeView(ctx context.Context, userID, practice string, year int) (EmployeePracticeView, error) {
	return v.employee(ctx, userID, practice, year)
}

func (v dashViewFunc) OrgView(ctx context.Context, orgID, practice string, from, to time.Time) (OrgPracticeView, error) {
	return v.org(ctx, orgID, practice, from, to)
}

func (v dashViewFunc) RecommendPeople(ctx context.Context, query string, limit int) ([]PersonRecommendation, error) {
	return v.people(ctx, query, limit)
}

func (v dashViewFunc) RecommendOrganizations(ctx context.Context, query string, limit int) ([]OrgRecommendation, error) {
	return v.orgs(ctx, query, limit)
}

func TestDashboardPeoplePracticeHandler(t *testing.T) {
	user := bson.NewObjectID()
	handler := NewDashboardHandler(dashViewFunc{
		employee: func(_ context.Context, userID, practice string, year int) (EmployeePracticeView, error) {
			if userID != user.Hex() || practice != PracticeLeadAndReveal || year != 2026 {
				t.Fatalf("%s %s %d", userID, practice, year)
			}
			return EmployeePracticeView{SchemaVersion: "employee_practice.v1"}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/people/"+user.Hex()+"/practices/lead_and_reveal?year=2026", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var view EmployeePracticeView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || view.SchemaVersion != "employee_practice.v1" {
		t.Fatalf("%v %s", err, w.Body)
	}
}

func TestDashboardOrgPracticeHandler(t *testing.T) {
	org := bson.NewObjectID()
	handler := NewDashboardHandler(dashViewFunc{
		org: func(_ context.Context, orgID, practice string, from, to time.Time) (OrgPracticeView, error) {
			if orgID != org.Hex() || practice != PracticeLeadAndReveal || from.Month() != time.January {
				t.Fatalf("%s %s %v %v", orgID, practice, from, to)
			}
			return OrgPracticeView{SchemaVersion: "org_practice.v1"}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/organizations/"+org.Hex()+"/practices/lead_and_reveal?quarter=1&year=2026", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestDashboardPeopleRecommendationsDefaultRecent(t *testing.T) {
	handler := NewDashboardHandler(dashViewFunc{
		people: func(_ context.Context, query string, limit int) ([]PersonRecommendation, error) {
			if query != "" || limit != DefaultRecommendationLimit {
				t.Fatalf("query=%q limit=%d", query, limit)
			}
			return []PersonRecommendation{{ID: "u1", Name: "user1", Mail: "user1@example.com"}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/people/recommendations", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var body struct {
		Items []PersonRecommendation `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || len(body.Items) != 1 || body.Items[0].Name != "user1" {
		t.Fatalf("%v %s", err, w.Body)
	}
}

func TestDashboardPeopleRecommendationsQuery(t *testing.T) {
	handler := NewDashboardHandler(dashViewFunc{
		people: func(_ context.Context, query string, limit int) ([]PersonRecommendation, error) {
			if query != "alice" || limit != DefaultRecommendationLimit {
				t.Fatalf("query=%q limit=%d", query, limit)
			}
			return []PersonRecommendation{{ID: "u2", Name: "alice", Mail: "alice@example.com"}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/people/recommendations?q=alice", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestDashboardOrgRecommendations(t *testing.T) {
	handler := NewDashboardHandler(dashViewFunc{
		orgs: func(_ context.Context, query string, limit int) ([]OrgRecommendation, error) {
			if query != "demo" || limit != DefaultRecommendationLimit {
				t.Fatalf("query=%q limit=%d", query, limit)
			}
			return []OrgRecommendation{{ID: "o1", Name: "demo-org"}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/organizations/recommendations?q=demo", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var body struct {
		Items []OrgRecommendation `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Items[0].Name != "demo-org" {
		t.Fatalf("%v %s", err, w.Body)
	}
}
