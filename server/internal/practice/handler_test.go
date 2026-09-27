package practice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type insertFunc func(context.Context, Event) (Event, error)

func (f insertFunc) Insert(ctx context.Context, e Event) (Event, error) { return f(ctx, e) }

func validJSON(t *testing.T) string {
	t.Helper()
	org := bson.NewObjectID()
	user := bson.NewObjectID()
	session := bson.NewObjectID()
	project := bson.NewObjectID()
	start := time.Date(2026, 9, 26, 14, 0, 0, 0, time.UTC)
	body, err := json.Marshal(map[string]any{
		"practice":              PracticeLeadAndReveal,
		"organization_id":       org.Hex(),
		"user_id":               user.Hex(),
		"session_id":            session.Hex(),
		"project_id":            project.Hex(),
		"started_at":            start.Format(time.RFC3339),
		"ended_at":              start.Add(2 * time.Minute).Format(time.RFC3339),
		"total_duration_ms":     120000,
		"active_answer_time_ms": 45000,
		"attempts":              1,
		"outcome":               OutcomeCorrect,
		"points_delta":          nil,
		"repo_name":             "cortisol-cli",
		"repo_org":              "DinhPhucLe",
		"file_path":             "server/internal/practice/model.go",
		"module":                "server",
		"start_line":            10,
		"end_line":              40,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestIngestHandlerCreatesPracticeInstance(t *testing.T) {
	var stored Event
	handler := NewIngestHandler(insertFunc(func(_ context.Context, e Event) (Event, error) {
		e.ID = bson.NewObjectID()
		stored = e
		return e, nil
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/practice-events", strings.NewReader(validJSON(t)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var got Event
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID.IsZero() || got.Practice != PracticeLeadAndReveal || got.Outcome != OutcomeCorrect {
		t.Fatalf("%+v", got)
	}
	if stored.Practice != PracticeLeadAndReveal || stored.Attempts != 1 {
		t.Fatalf("stored %+v", stored)
	}
}

func TestIngestHandlerErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body, code string
		err              error
		status           int
	}{
		{"invalid json shape", `{"practice":"lead_and_reveal","attempts":0}`, "invalid_request", nil, 400},
		{"not member", validBodyNeedsIDs(), "forbidden", ErrNotMember, 403},
		{"storage", validBodyNeedsIDs(), "persist_failed", errors.New("private details"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.body
			if body == "" {
				body = validJSON(t)
			}
			handler := NewIngestHandler(insertFunc(func(context.Context, Event) (Event, error) {
				return Event{}, tc.err
			}))
			req := httptest.NewRequest(http.MethodPost, "/api/v1/practice-events", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			var resp struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if w.Code != tc.status || json.Unmarshal(w.Body.Bytes(), &resp) != nil || resp.Error.Code != tc.code {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if strings.Contains(w.Body.String(), "private details") {
				t.Fatal("leaked internals")
			}
		})
	}
}

func validBodyNeedsIDs() string {
	return `{"practice":"lead_and_reveal","organization_id":"66aaaaaaaaaaaaaaaaaaaaaa","user_id":"66bbbbbbbbbbbbbbbbbbbbbb","session_id":"66cccccccccccccccccccccc","project_id":"66dddddddddddddddddddddd","started_at":"2026-09-26T14:00:00Z","ended_at":"2026-09-26T14:02:00Z","total_duration_ms":120000,"active_answer_time_ms":45000,"attempts":1,"outcome":"correct","points_delta":null,"repo_name":"cortisol-cli","repo_org":"DinhPhucLe","file_path":"a.go","module":"server","start_line":1,"end_line":2}`
}

func TestIngestHandlerRejectsWrongMethod(t *testing.T) {
	handler := NewIngestHandler(insertFunc(func(context.Context, Event) (Event, error) {
		return Event{}, nil
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/practice-events", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatal(w.Code)
	}
}
