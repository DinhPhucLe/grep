// Package orgknowledge stores org-scoped knowledge documents for Atlas vector search.
// Embeddings are produced by Atlas Automated Embedding (voyage-code-4), not by this package.
package orgknowledge

import (
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	CollectionName  = "knowledge_documents"
	VectorIndexName = "knowledge_content_voyage"
	EmbeddingModel  = "voyage-code-4"
	EmbeddingDims   = 1024
	MaxContentRunes = 4000
	MaxTopics       = 16
)

var (
	ErrInvalid  = errors.New("invalid knowledge document")
	ErrNotFound = errors.New("organization or knowledge not found")
)

// Author matches the MCP knowledge contract (userId + optional name).
type Author struct {
	UserID string `json:"userId" bson:"user_id"`
	Name   string `json:"name,omitempty" bson:"name,omitempty"`
}

// Document is the org knowledge unit shared by API and MCP HTTP client.
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
