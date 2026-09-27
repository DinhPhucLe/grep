package practice

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Ingester interface {
	Insert(context.Context, Event) (Event, error)
}

type ingestRequest struct {
	Practice           string  `json:"practice"`
	OrganizationID     string  `json:"organization_id"`
	UserID             string  `json:"user_id"`
	SessionID          string  `json:"session_id"`
	ProjectID          string  `json:"project_id"`
	StartedAt          string  `json:"started_at"`
	EndedAt            string  `json:"ended_at"`
	TotalDurationMs    int64   `json:"total_duration_ms"`
	ActiveAnswerTimeMs int64   `json:"active_answer_time_ms"`
	Attempts           int     `json:"attempts"`
	Outcome            string  `json:"outcome"`
	PointsDelta        *int    `json:"points_delta"`
	RepoName           string  `json:"repo_name"`
	RepoOrg            string  `json:"repo_org"`
	FilePath           string  `json:"file_path"`
	Module             string  `json:"module"`
	StartLine          int     `json:"start_line"`
	EndLine            int     `json:"end_line"`
	AnswerQuality      *string `json:"answer_quality"`
	Relevance          *string `json:"relevance"`
}

func (r ingestRequest) toEvent() (Event, error) {
	orgID, err := bson.ObjectIDFromHex(r.OrganizationID)
	if err != nil {
		return Event{}, errors.New("invalid organization_id")
	}
	userID, err := bson.ObjectIDFromHex(r.UserID)
	if err != nil {
		return Event{}, errors.New("invalid user_id")
	}
	sessionID, err := bson.ObjectIDFromHex(r.SessionID)
	if err != nil {
		return Event{}, errors.New("invalid session_id")
	}
	projectID, err := bson.ObjectIDFromHex(r.ProjectID)
	if err != nil {
		return Event{}, errors.New("invalid project_id")
	}
	startedAt, err := time.Parse(time.RFC3339, r.StartedAt)
	if err != nil {
		return Event{}, errors.New("invalid started_at")
	}
	endedAt, err := time.Parse(time.RFC3339, r.EndedAt)
	if err != nil {
		return Event{}, errors.New("invalid ended_at")
	}
	return Event{
		Practice:           r.Practice,
		OrganizationID:     orgID,
		UserID:             userID,
		SessionID:          sessionID,
		ProjectID:          projectID,
		StartedAt:          startedAt,
		EndedAt:            endedAt,
		TotalDurationMs:    r.TotalDurationMs,
		ActiveAnswerTimeMs: r.ActiveAnswerTimeMs,
		Attempts:           r.Attempts,
		Outcome:            r.Outcome,
		PointsDelta:        r.PointsDelta,
		RepoName:           r.RepoName,
		RepoOrg:            r.RepoOrg,
		FilePath:           r.FilePath,
		Module:             r.Module,
		StartLine:          r.StartLine,
		EndLine:            r.EndLine,
		AnswerQuality:      r.AnswerQuality,
		Relevance:          r.Relevance,
	}, nil
}

func NewIngestHandler(ingester Ingester) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
			return
		}
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "use application/json")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var input ingestRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "expected one practice event JSON object")
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_request", "expected one practice event JSON object")
			return
		}
		event, err := input.toEvent()
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		if err := event.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		stored, err := ingester.Insert(r.Context(), event)
		switch {
		case errors.Is(err, ErrNotMember):
			writeError(w, http.StatusForbidden, "forbidden", "user is not a member of the organization")
		case err != nil:
			writeError(w, http.StatusInternalServerError, "persist_failed", "practice event could not be stored")
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(stored)
		}
	})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}
