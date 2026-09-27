package orgknowledge

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// KnowledgeDashboard is the HTTP seam for knowledge dashboard GETs.
type KnowledgeDashboard interface {
	EmployeeKnowledgeView(ctx context.Context, userID string, year int) (EmployeeKnowledgeView, error)
	OrgKnowledgeView(ctx context.Context, orgID string, from, to time.Time) (OrgKnowledgeView, error)
}

// NewDashboardHandler serves knowledge dashboard routes under /api/v1/dashboard/.
func NewDashboardHandler(svc KnowledgeDashboard) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET")
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/dashboard")
		path = strings.Trim(path, "/")
		parts := strings.Split(path, "/")
		if len(parts) < 2 {
			writeError(w, http.StatusNotFound, "not_found", "expected knowledge dashboard path")
			return
		}
		switch parts[0] {
		case "people":
			handleEmployeeKnowledge(w, r, svc, parts[1:])
		case "organizations":
			handleOrgKnowledge(w, r, svc, parts[1:])
		default:
			writeError(w, http.StatusNotFound, "not_found", "unknown dashboard route")
		}
	})
}

func handleEmployeeKnowledge(w http.ResponseWriter, r *http.Request, svc KnowledgeDashboard, parts []string) {
	if len(parts) != 2 || parts[1] != "knowledge" || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not_found", "expected /api/v1/dashboard/people/{userId}/knowledge")
		return
	}
	userID := parts[0]
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
	view, err := svc.EmployeeKnowledgeView(r.Context(), userID, year)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "aggregate_failed", "employee knowledge view could not be built")
		return
	}
	writeJSON(w, view)
}

func handleOrgKnowledge(w http.ResponseWriter, r *http.Request, svc KnowledgeDashboard, parts []string) {
	if len(parts) != 2 || parts[1] != "knowledge" || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not_found", "expected /api/v1/dashboard/organizations/{orgId}/knowledge")
		return
	}
	orgID := parts[0]
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
	view, err := svc.OrgKnowledgeView(r.Context(), orgID, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "aggregate_failed", "organization knowledge view could not be built")
		return
	}
	writeJSON(w, view)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
