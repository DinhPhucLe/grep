package practice

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const DefaultRecommendationLimit = 10

type PersonRecommendation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Mail string `json:"mail"`
}

type OrgRecommendation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Viewer interface {
	EmployeeView(ctx context.Context, userID, practice string, year int) (EmployeePracticeView, error)
	OrgView(ctx context.Context, orgID, practice string, from, to time.Time) (OrgPracticeView, error)
}

type DashboardService interface {
	Viewer
	RecommendPeople(ctx context.Context, query string, limit int) ([]PersonRecommendation, error)
	RecommendOrganizations(ctx context.Context, query string, limit int) ([]OrgRecommendation, error)
}

func NewDashboardHandler(svc DashboardService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET")
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/dashboard")
		path = strings.Trim(path, "/")
		parts := strings.Split(path, "/")
		if len(parts) == 0 || parts[0] == "" {
			writeError(w, http.StatusNotFound, "not_found", "expected /api/v1/dashboard/...")
			return
		}
		switch parts[0] {
		case "people":
			handleDashboardPeople(w, r, svc, parts[1:])
		case "organizations":
			handleDashboardOrganizations(w, r, svc, parts[1:])
		default:
			writeError(w, http.StatusNotFound, "not_found", "unknown dashboard route")
		}
	})
}

func handleDashboardPeople(w http.ResponseWriter, r *http.Request, svc DashboardService, parts []string) {
	if len(parts) == 1 && parts[0] == "recommendations" {
		items, err := svc.RecommendPeople(r.Context(), r.URL.Query().Get("q"), DefaultRecommendationLimit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "search_failed", "people recommendations could not be loaded")
			return
		}
		writeJSON(w, map[string]any{"items": items})
		return
	}
	if len(parts) != 3 || parts[1] != "practices" || parts[0] == "" || parts[2] == "" {
		writeError(w, http.StatusNotFound, "not_found", "expected /api/v1/dashboard/people/{userId}/practices/{practice}")
		return
	}
	userID, practice := parts[0], parts[2]
	if _, err := bson.ObjectIDFromHex(userID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid user id")
		return
	}
	year := time.Now().UTC().Year()
	if raw := r.URL.Query().Get("year"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1970 || parsed > 3000 {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid year")
			return
		}
		year = parsed
	}
	view, err := svc.EmployeeView(r.Context(), userID, practice, year)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "aggregate_failed", "employee practice view could not be built")
		return
	}
	writeJSON(w, view)
}

func handleDashboardOrganizations(w http.ResponseWriter, r *http.Request, svc DashboardService, parts []string) {
	if len(parts) == 1 && parts[0] == "recommendations" {
		items, err := svc.RecommendOrganizations(r.Context(), r.URL.Query().Get("q"), DefaultRecommendationLimit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "search_failed", "organization recommendations could not be loaded")
			return
		}
		writeJSON(w, map[string]any{"items": items})
		return
	}
	if len(parts) != 3 || parts[1] != "practices" || parts[0] == "" || parts[2] == "" {
		writeError(w, http.StatusNotFound, "not_found", "expected /api/v1/dashboard/organizations/{orgId}/practices/{practice}")
		return
	}
	orgID, practice := parts[0], parts[2]
	if _, err := bson.ObjectIDFromHex(orgID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid organization id")
		return
	}
	now := time.Now().UTC()
	from := time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
	to := now
	if raw := r.URL.Query().Get("from"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid from")
			return
		}
		from = parsed
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid to")
			return
		}
		to = parsed
	}
	if quarter := r.URL.Query().Get("quarter"); quarter != "" {
		q, err := strconv.Atoi(quarter)
		if err != nil || q < 1 || q > 4 {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid quarter")
			return
		}
		year := now.Year()
		if raw := r.URL.Query().Get("year"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "invalid year")
				return
			}
			year = parsed
		}
		month := time.Month((q-1)*3 + 1)
		from = time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
		to = from.AddDate(0, 3, 0).Add(-time.Nanosecond)
	}
	view, err := svc.OrgView(r.Context(), orgID, practice, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "aggregate_failed", "organization practice view could not be built")
		return
	}
	writeJSON(w, view)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
