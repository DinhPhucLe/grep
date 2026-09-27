// Package identity holds the authenticated caller attached to request contexts.
package identity

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type contextKey struct{}

// Principal is the authenticated caller for API handlers.
type Principal struct {
	UserID         bson.ObjectID
	Name           string
	Mail           string
	GitHubLogin    string
	AvatarURL      string
	OrganizationID bson.ObjectID
	OrgName        string
	SessionID      bson.ObjectID
}

// WithPrincipal stores p on ctx.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, p)
}

// PrincipalFromContext returns the authenticated principal when present.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(contextKey{}).(Principal)
	return p, ok
}
