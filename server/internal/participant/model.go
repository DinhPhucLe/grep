// Package participant stores anonymous, installation-local participant profiles.
// A participant ID identifies records; it is not an authentication credential.
package participant

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
	"time"
	"unicode"
	"unicode/utf8"
)

type Profile struct {
	ID          string    `json:"id" bson:"_id"`
	DisplayName string    `json:"display_name,omitempty" bson:"display_name,omitempty"`
	CreatedAt   time.Time `json:"created_at" bson:"created_at"`
	LastSeenAt  time.Time `json:"last_seen_at" bson:"last_seen_at"`
}

type Registration struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
}

type Repository interface {
	Register(context.Context, Registration) (Profile, error)
	Exists(context.Context, string) (bool, error)
}

var ErrMigrationRequired = errors.New("participant storage requires a database migration")
var canonicalID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func ValidID(id string) bool { return canonicalID.MatchString(id) }

func NewID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(fmt.Sprintf("generate participant ID: %v", err))
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
}

func (r Registration) Validate() error {
	if !ValidID(r.ID) {
		return errors.New("id must be a lowercase UUID v4")
	}
	if !utf8.ValidString(r.DisplayName) || utf8.RuneCountInString(r.DisplayName) > 100 {
		return errors.New("display_name must contain at most 100 characters")
	}
	for _, char := range r.DisplayName {
		if unicode.IsControl(char) {
			return errors.New("display_name must not contain control characters")
		}
	}
	return nil
}
