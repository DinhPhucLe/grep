package practice

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type memMembers map[string]bool

func (m memMembers) IsMember(_ context.Context, org, user bson.ObjectID) (bool, error) {
	return m[org.Hex()+":"+user.Hex()], nil
}

type memEvents struct {
	events []Event
	err    error
}

func (m *memEvents) Insert(_ context.Context, event Event) error {
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, event)
	return nil
}

func (m *memEvents) ListByUserPracticeYear(_ context.Context, userID bson.ObjectID, practice string, year int) ([]Event, error) {
	if m.err != nil {
		return nil, m.err
	}
	var out []Event
	for _, e := range m.events {
		if e.UserID == userID && e.Practice == practice && e.StartedAt.Year() == year {
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *memEvents) ListByOrgPracticeRange(_ context.Context, orgID bson.ObjectID, practice string, from, to time.Time) ([]Event, error) {
	if m.err != nil {
		return nil, m.err
	}
	var out []Event
	for _, e := range m.events {
		if e.OrganizationID == orgID && e.Practice == practice && !e.StartedAt.Before(from) && !e.StartedAt.After(to) {
			out = append(out, e)
		}
	}
	return out, nil
}

func TestServiceInsertRequiresMembership(t *testing.T) {
	event := validEvent()
	repo := &memEvents{}
	svc := NewService(memMembers{}, repo, nil)
	if _, err := svc.Insert(context.Background(), event); !errors.Is(err, ErrNotMember) {
		t.Fatalf("got %v", err)
	}
	if len(repo.events) != 0 {
		t.Fatal("stored without membership")
	}
}

func TestServiceInsertPersistsWhenMember(t *testing.T) {
	event := validEvent()
	repo := &memEvents{}
	members := memMembers{event.OrganizationID.Hex() + ":" + event.UserID.Hex(): true}
	svc := NewService(members, repo, nil)
	stored, err := svc.Insert(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID.IsZero() || len(repo.events) != 1 {
		t.Fatalf("%+v %d", stored, len(repo.events))
	}
	if !repo.events[0].EndedAt.After(repo.events[0].StartedAt.Add(time.Minute)) {
		t.Fatal(repo.events[0])
	}
}
