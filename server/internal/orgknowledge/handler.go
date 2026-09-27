package orgknowledge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Store is the persistence seam used by the HTTP handler.
type Store interface {
	Insert(ctx context.Context, doc Document) (Document, error)
	Search(ctx context.Context, params SearchParams) (SearchResult, error)
}

// SearchParams aligns with MCP knowledge_search arguments.
type SearchParams struct {
	OrganizationID string
	Query          string
	K              int
	Topics         []string
	Author         string
	From           *time.Time
	To             *time.Time
	Properties     map[string]string
}

// SearchResult is the GET /api/v1/knowledge response body.
type SearchResult struct {
	Items  []Document `json:"items"`
	Scores []float64  `json:"scores,omitempty"`
}

type postRequest struct {
	Content        string            `json:"content"`
	Topics         []string          `json:"topics"`
	Properties     map[string]string `json:"properties"`
	Authors        []Author          `json:"authors"`
	OrganizationID string            `json:"organizationId"`
}

// NewHandler serves GET/POST /api/v1/knowledge.
func NewHandler(store Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleGet(w, r, store)
		case http.MethodPost:
			handlePost(w, r, store)
		default:
			w.Header().Set("Allow", "GET, POST")
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET or POST")
		}
	})
}

func handleGet(w http.ResponseWriter, r *http.Request, store Store) {
	q := r.URL.Query()
	orgID := strings.TrimSpace(q.Get("organizationId"))
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "organizationId is required")
		return
	}
	k := 5
	if raw := strings.TrimSpace(q.Get("k")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 50 {
			writeError(w, http.StatusBadRequest, "invalid_request", "k must be an integer from 1 to 50")
			return
		}
		k = parsed
	}
	params := SearchParams{
		OrganizationID: orgID,
		Query:          strings.TrimSpace(q.Get("query")),
		K:              k,
		Author:         strings.TrimSpace(q.Get("author")),
	}
	if topics := q["topics"]; len(topics) > 0 {
		params.Topics = topics
	} else if raw := strings.TrimSpace(q.Get("topics")); raw != "" {
		params.Topics = strings.Split(raw, ",")
	}
	if raw := strings.TrimSpace(q.Get("from")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "from must be RFC3339")
			return
		}
		params.From = &t
	}
	if raw := strings.TrimSpace(q.Get("to")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "to must be RFC3339")
			return
		}
		params.To = &t
	}
	props := map[string]string{}
	for key, values := range q {
		if strings.HasPrefix(key, "prop.") && len(values) > 0 {
			props[strings.TrimPrefix(key, "prop.")] = values[0]
		}
	}
	if len(props) > 0 {
		params.Properties = props
	}
	result, err := store.Search(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search_failed", "knowledge search failed")
		return
	}
	if result.Items == nil {
		result.Items = []Document{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func handlePost(w http.ResponseWriter, r *http.Request, store Store) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "use application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var input postRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "expected one knowledge document JSON object")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_request", "expected one knowledge document JSON object")
		return
	}
	doc := Document{
		Content:        input.Content,
		Topics:         input.Topics,
		Properties:     input.Properties,
		Authors:        input.Authors,
		OrganizationID: input.OrganizationID,
	}
	if err := doc.ValidateForCreate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	created, err := store.Insert(r.Context(), doc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "persist_failed", "failed to store knowledge document")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

// ValidateForCreate checks fields required to post a knowledge document.
func (d Document) ValidateForCreate() error {
	content := strings.TrimSpace(d.Content)
	if content == "" || !utf8.ValidString(d.Content) {
		return errors.New("content is required")
	}
	if utf8.RuneCountInString(d.Content) > MaxContentRunes {
		return errors.New("content exceeds max length")
	}
	if strings.TrimSpace(d.OrganizationID) == "" {
		return errors.New("organizationId is required")
	}
	if len(d.Topics) == 0 {
		return errors.New("topics is required")
	}
	if len(d.Topics) > MaxTopics {
		return errors.New("too many topics")
	}
	for _, t := range d.Topics {
		if strings.TrimSpace(t) == "" {
			return errors.New("topics must be non-empty")
		}
	}
	if len(d.Authors) == 0 {
		return errors.New("authors is required")
	}
	for _, a := range d.Authors {
		if strings.TrimSpace(a.UserID) == "" {
			return errors.New("authors.userId is required")
		}
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
