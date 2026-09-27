// Package orgknowledge stores org-scoped knowledge documents for Atlas vector search.
// Embeddings are produced by Atlas Automated Embedding (voyage-code-4), not by this package.
package orgknowledge

import (
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"time"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	CollectionName   = "knowledge_documents"
	VectorIndexName  = "knowledge_content_voyage"
	EmbeddingModel   = "voyage-code-4"
	EmbeddingDims    = 1024
	MaxContentRunes  = 4000
	MaxTopics        = 16
	DefaultSeedCount = 1500
)

var (
	ErrInvalid  = errors.New("invalid knowledge document")
	ErrNotFound = errors.New("organization or knowledge summary not found")
)

// Author matches the MCP knowledge contract (userId + optional name).
type Author struct {
	UserID string `json:"userId" bson:"user_id"`
	Name   string `json:"name,omitempty" bson:"name,omitempty"`
}

// Document is the org knowledge unit shared by seed, API, and future MCP HTTP client.
type Document struct {
	ID             bson.ObjectID     `json:"id" bson:"_id"`
	Content        string            `json:"content" bson:"content"`
	Topics         []string          `json:"topics" bson:"topics"`
	Properties     map[string]string `json:"properties" bson:"properties"`
	Authors        []Author          `json:"authors" bson:"authors"`
	CreatedAt      time.Time         `json:"createdAt" bson:"created_at"`
	UpdatedAt      time.Time         `json:"updatedAt" bson:"updated_at"`
	OrganizationID string            `json:"organizationId" bson:"organization_id"`
}

// GitHubExportRow is the canonical Snowflake Marketplace export row.
type GitHubExportRow struct {
	EventID    string   `json:"event_id"`
	EventType  string   `json:"event_type"`
	RepoName   string   `json:"repo_name"`
	ActorLogin string   `json:"actor_login"`
	Title      string   `json:"title"`
	Body       string   `json:"body"`
	CreatedAt  string   `json:"created_at"`
	Labels     []string `json:"labels"`
}

// DemoAuthor is a seed-time cast member used for attribution.
type DemoAuthor struct {
	UserID string
	Name   string
	OrgID  string
}

// MapExportRows converts Marketplace (or sample) rows into Documents with demo attribution.
func MapExportRows(rows []GitHubExportRow, authors []DemoAuthor, novaOrgID, atlasOrgID string) ([]Document, error) {
	if len(authors) == 0 {
		return nil, fmt.Errorf("authors required: %w", ErrInvalid)
	}
	out := make([]Document, 0, len(rows))
	for i, row := range rows {
		doc, err := mapExportRow(row, i, authors, novaOrgID, atlasOrgID)
		if err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, nil
}

func mapExportRow(row GitHubExportRow, index int, authors []DemoAuthor, novaOrgID, atlasOrgID string) (Document, error) {
	title := strings.TrimSpace(row.Title)
	body := strings.TrimSpace(row.Body)
	if title == "" && body == "" {
		return Document{}, fmt.Errorf("row %d empty content: %w", index, ErrInvalid)
	}
	content := title
	if body != "" {
		if content != "" {
			content += "\n\n" + body
		} else {
			content = body
		}
	}
	content = truncateRunes(content, MaxContentRunes)
	if !utf8.ValidString(content) || strings.TrimSpace(content) == "" {
		return Document{}, fmt.Errorf("row %d invalid content: %w", index, ErrInvalid)
	}

	orgID := pickOrgID(row.RepoName, row.Labels, novaOrgID, atlasOrgID)
	author := pickAuthor(row.ActorLogin+row.EventID, authors, orgID)
	created := time.Now().UTC()
	if row.CreatedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, row.CreatedAt); err == nil {
			created = parsed.UTC()
		}
	}

	topics := normalizeTopics(row.Labels, row.RepoName)
	props := map[string]string{
		"source":     "snowflake_github",
		"event_type": strings.TrimSpace(row.EventType),
		"repo":       strings.TrimSpace(row.RepoName),
	}
	if row.EventID != "" {
		props["event_id"] = row.EventID
	}
	if lang := inferLanguage(row.RepoName, body); lang != "" {
		props["language"] = lang
	}
	if module := inferModule(row.RepoName, topics); module != "" {
		props["module"] = module
	}

	id := deterministicID(row.EventID, index)
	return Document{
		ID:             id,
		Content:        content,
		Topics:         topics,
		Properties:     props,
		Authors:        []Author{{UserID: author.UserID, Name: author.Name}},
		CreatedAt:      created,
		UpdatedAt:      created,
		OrganizationID: orgID,
	}, nil
}

func pickOrgID(repo string, labels []string, nova, atlas string) string {
	blob := strings.ToLower(repo + " " + strings.Join(labels, " "))
	atlasHints := []string{"atlas", "fhir", "health", "patient", "eligibility", "claims", "billing"}
	for _, h := range atlasHints {
		if strings.Contains(blob, h) {
			return atlas
		}
	}
	novaHints := []string{"novapay", "payment", "stripe", "checkout", "wallet", "ledger", "transfer"}
	for _, h := range novaHints {
		if strings.Contains(blob, h) {
			return nova
		}
	}
	if hashString(blob)%2 == 0 {
		return nova
	}
	return atlas
}

func pickAuthor(key string, authors []DemoAuthor, orgID string) DemoAuthor {
	orgAuthors := make([]DemoAuthor, 0, len(authors))
	for _, a := range authors {
		if a.OrgID == orgID {
			orgAuthors = append(orgAuthors, a)
		}
	}
	pool := orgAuthors
	if len(pool) == 0 {
		pool = authors
	}
	return pool[int(hashString(key)%uint32(len(pool)))]
}

func normalizeTopics(labels []string, repo string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		t := strings.ToLower(strings.TrimSpace(raw))
		t = strings.ReplaceAll(t, " ", "-")
		if t == "" || seen[t] || len(t) > 64 {
			return
		}
		seen[t] = true
		out = append(out, t)
	}
	for _, l := range labels {
		add(l)
		if len(out) >= MaxTopics {
			return out
		}
	}
	if repo != "" {
		parts := strings.Split(repo, "/")
		add(parts[len(parts)-1])
	}
	if len(out) == 0 {
		add("engineering")
	}
	return out
}

func inferLanguage(repo, body string) string {
	lower := strings.ToLower(repo + " " + body)
	switch {
	case strings.Contains(lower, ".go") || strings.Contains(lower, "golang"):
		return "go"
	case strings.Contains(lower, ".tsx") || strings.Contains(lower, "react"):
		return "typescript"
	case strings.Contains(lower, ".ts"):
		return "typescript"
	case strings.Contains(lower, ".py"):
		return "python"
	default:
		return ""
	}
}

func inferModule(repo string, topics []string) string {
	if len(topics) > 0 {
		return topics[0]
	}
	parts := strings.Split(repo, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}

func hashString(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

// deterministicID builds a stable ObjectID from event id / index for idempotent seeding.
func deterministicID(eventID string, index int) bson.ObjectID {
	h := fnv.New64a()
	_, _ = h.Write([]byte(eventID))
	_, _ = fmt.Fprintf(h, "#%d", index)
	sum := h.Sum64()
	var id bson.ObjectID
	id[0] = 0x67
	id[1] = 0x4b // knowledge
	for i := 0; i < 10; i++ {
		id[2+i] = byte(sum >> (8 * (9 - i)))
	}
	return id
}
