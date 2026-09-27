package orgknowledge

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const ConnectCollectionName = "knowledge_connects"

// Connect is one directed learning edge: seeker connected a document authored by author.
type Connect struct {
	ID             bson.ObjectID `json:"id" bson:"_id"`
	OrganizationID string        `json:"organizationId" bson:"organization_id"`
	SeekerUserID   string        `json:"seekerUserId" bson:"seeker_user_id"`
	AuthorUserID   string        `json:"authorUserId" bson:"author_user_id"`
	DocumentID     string        `json:"documentId" bson:"document_id"`
	SessionID      string        `json:"sessionId" bson:"session_id"`
	Topics         []string      `json:"topics" bson:"topics"`
	CreatedAt      time.Time     `json:"createdAt" bson:"created_at"`
}
