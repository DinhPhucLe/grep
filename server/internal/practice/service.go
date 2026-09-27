package practice

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type MemberChecker interface {
	IsMember(ctx context.Context, organizationID, userID bson.ObjectID) (bool, error)
}

type EventRepository interface {
	Insert(ctx context.Context, event Event) error
	ListByUserPracticeYear(ctx context.Context, userID bson.ObjectID, practice string, year int) ([]Event, error)
	ListByOrgPracticeRange(ctx context.Context, orgID bson.ObjectID, practice string, from, to time.Time) ([]Event, error)
}

type Service struct {
	members   MemberChecker
	events    EventRepository
	directory Directory
}

func NewService(members MemberChecker, events EventRepository, directory Directory) *Service {
	return &Service{members: members, events: events, directory: directory}
}

func (s *Service) RecommendPeople(ctx context.Context, query string, limit int) ([]PersonRecommendation, error) {
	if s.directory == nil {
		return []PersonRecommendation{}, nil
	}
	return s.directory.RecommendPeople(ctx, query, limit)
}

func (s *Service) RecommendOrganizations(ctx context.Context, query string, limit int) ([]OrgRecommendation, error) {
	if s.directory == nil {
		return []OrgRecommendation{}, nil
	}
	return s.directory.RecommendOrganizations(ctx, query, limit)
}

func (s *Service) Insert(ctx context.Context, event Event) (Event, error) {
	ok, err := s.members.IsMember(ctx, event.OrganizationID, event.UserID)
	if err != nil {
		return Event{}, err
	}
	if !ok {
		return Event{}, ErrNotMember
	}
	if event.ID.IsZero() {
		event.ID = bson.NewObjectID()
	}
	if err := s.events.Insert(ctx, event); err != nil {
		return Event{}, err
	}
	return event, nil
}

func (s *Service) EmployeeView(ctx context.Context, userID, practice string, year int) (EmployeePracticeView, error) {
	id, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return EmployeePracticeView{}, err
	}
	events, err := s.events.ListByUserPracticeYear(ctx, id, practice, year)
	if err != nil {
		return EmployeePracticeView{}, err
	}
	orgID := ""
	if len(events) > 0 {
		orgID = events[0].OrganizationID.Hex()
	}
	return BuildEmployeeView(EmployeeSubject{
		UserID:         userID,
		OrganizationID: orgID,
		Practice:       practice,
		Year:           year,
	}, events), nil
}

func (s *Service) OrgView(ctx context.Context, orgID, practice string, from, to time.Time) (OrgPracticeView, error) {
	id, err := bson.ObjectIDFromHex(orgID)
	if err != nil {
		return OrgPracticeView{}, err
	}
	events, err := s.events.ListByOrgPracticeRange(ctx, id, practice, from, to)
	if err != nil {
		return OrgPracticeView{}, err
	}
	return BuildOrgView(OrgSubject{
		OrganizationID: orgID,
		Practice:       practice,
		From:           from.UTC().Format(time.RFC3339),
		To:             to.UTC().Format(time.RFC3339),
	}, events), nil
}
