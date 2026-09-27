// Package orgknowledge stores org-scoped knowledge documents for Atlas vector search.
// Embeddings are produced by Atlas Automated Embedding (voyage-code-4), not by this package.
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
	FindByID(ctx context.Context, id string) (Document, error)
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
	Content    string            `json:"content"`
	Topics     []string          `json:"topics"`
	Properties map[string]string `json:"properties"`
	// Authors and OrganizationID are ignored when a session principal is present.
	Authors        []Author `json:"authors"`
	OrganizationID string   `json:"organizationId"`
}

// NewHandler serves GET/POST /api/v1/knowledge. Requires auth.Principal on the request.
// When hub is non-nil, successful inserts are published for SSE subscribers.
func NewHandler(store Store, hub *Hub) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleGet(w, r, store)
		case http.MethodPost:
			handlePost(w, r, store, hub)
		default:
			w.Header().Set("Allow", "GET, POST")
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET or POST")
		}
	})
}

type connectRequest struct {
	DocumentID string `json:"documentId"`
	SessionID  string `json:"sessionId"`
}

type connectResponse struct {
	Inserted int       `json:"inserted"`
	Edges    []Connect `json:"edges"`
}

// NewConnectHandler serves POST /api/v1/knowledge/connects.
func NewConnectHandler(docs DocumentByID, connects ConnectStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
			return
		}
		principal, ok := principalFrom(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "use application/json")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var input connectRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "expected connect JSON object")
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_request", "expected connect JSON object")
			return
		}
		docID := strings.TrimSpace(input.DocumentID)
		sessionID := strings.TrimSpace(input.SessionID)
		if docID == "" || sessionID == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "documentId and sessionId are required")
			return
		}
		doc, err := docs.FindByID(r.Context(), docID)
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "knowledge document not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup_failed", "failed to load knowledge document")
			return
		}
		orgID := principal.OrganizationID.Hex()
		if doc.OrganizationID != orgID {
			writeError(w, http.StatusForbidden, "forbidden", "document is outside session organization")
			return
		}
		seeker := principal.UserID.Hex()
		now := time.Now().UTC().Truncate(time.Millisecond)
		topics := append([]string(nil), doc.Topics...)
		edges := make([]Connect, 0, len(doc.Authors))
		for _, author := range doc.Authors {
			authorID := strings.TrimSpace(author.UserID)
			if authorID == "" || authorID == seeker {
				continue
			}
			edges = append(edges, Connect{
				OrganizationID: orgID,
				SeekerUserID:   seeker,
				AuthorUserID:   authorID,
				DocumentID:     doc.ID.Hex(),
				SessionID:      sessionID,
				Topics:         topics,
				CreatedAt:      now,
			})
		}
		if len(edges) == 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "no learning edges (document has no other authors)")
			return
		}
		inserted, err := connects.InsertConnects(r.Context(), edges)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "persist_failed", "failed to store knowledge connects")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if inserted > 0 {
			w.WriteHeader(http.StatusCreated)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		_ = json.NewEncoder(w).Encode(connectResponse{Inserted: inserted, Edges: edges})
	})
}

func handleGet(w http.ResponseWriter, r *http.Request, store Store) {
	principal, ok := principalFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	q := r.URL.Query()
	orgID := principal.OrganizationID.Hex()
	if raw := strings.TrimSpace(q.Get("organizationId")); raw != "" && raw != orgID {
		writeError(w, http.StatusBadRequest, "invalid_request", "organizationId does not match session organization")
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

func handlePost(w http.ResponseWriter, r *http.Request, store Store, hub *Hub) {
	principal, ok := principalFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
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
	authorName := principal.Name
	if principal.GitHubLogin != "" {
		authorName = principal.Name
		if authorName == "" {
			authorName = principal.GitHubLogin
		}
	}
	doc := Document{
		Content:    input.Content,
		Topics:     input.Topics,
		Properties: input.Properties,
		Authors: []Author{{
			UserID: principal.UserID.Hex(),
			Name:   authorName,
		}},
		OrganizationID: principal.OrganizationID.Hex(),
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
	if hub != nil {
		hub.Publish(created)
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
