package orgknowledge

import (
	"context"
	"time"
)

// NameLookup resolves optional display names for dashboard subjects.
type NameLookup interface {
	LookupUserName(ctx context.Context, userID string) (string, error)
	LookupOrganizationName(ctx context.Context, orgID string) (string, error)
}

// DashboardService builds employee and org knowledge dashboard views.
type DashboardService struct {
	connects ConnectStore
	docs     DocumentLookup
	names    NameLookup
}

// NewDashboardService wires connect + document stores for dashboard GETs.
func NewDashboardService(connects ConnectStore, docs DocumentLookup) *DashboardService {
	return &DashboardService{connects: connects, docs: docs}
}

// WithNames attaches optional name lookups.
func (s *DashboardService) WithNames(names NameLookup) *DashboardService {
	s.names = names
	return s
}

// EmployeeKnowledgeView loads and aggregates a person's year of connects.
func (s *DashboardService) EmployeeKnowledgeView(ctx context.Context, userID string, year int) (EmployeeKnowledgeView, error) {
	edges, err := s.connects.ListByUserYear(ctx, userID, year)
	if err != nil {
		return EmployeeKnowledgeView{}, err
	}
	subject := EmployeeKnowledgeSubject{UserID: userID, Year: year}
	if len(edges) > 0 {
		subject.OrganizationID = edges[0].OrganizationID
	}
	if s.names != nil {
		if name, err := s.names.LookupUserName(ctx, userID); err == nil {
			subject.UserName = name
		} else {
			return EmployeeKnowledgeView{}, err
		}
	}
	return BuildEmployeeKnowledgeView(subject, edges), nil
}

// OrgKnowledgeView loads and aggregates org connects in a time window.
func (s *DashboardService) OrgKnowledgeView(ctx context.Context, orgID string, from, to time.Time) (OrgKnowledgeView, error) {
	edges, err := s.connects.ListByOrgRange(ctx, orgID, from, to)
	if err != nil {
		return OrgKnowledgeView{}, err
	}
	subject := OrgKnowledgeSubject{
		OrganizationID: orgID,
		From:           from.UTC().Format(time.RFC3339),
		To:             to.UTC().Format(time.RFC3339),
	}
	if s.names != nil {
		if name, err := s.names.LookupOrganizationName(ctx, orgID); err == nil {
			subject.OrganizationName = name
		} else {
			return OrgKnowledgeView{}, err
		}
	}
	docIDs := make([]string, 0, len(edges))
	seen := map[string]struct{}{}
	for _, e := range edges {
		if _, ok := seen[e.DocumentID]; ok {
			continue
		}
		seen[e.DocumentID] = struct{}{}
		docIDs = append(docIDs, e.DocumentID)
	}
	var docs []Document
	if s.docs != nil && len(docIDs) > 0 {
		docs, err = s.docs.FindByIDs(ctx, docIDs)
		if err != nil {
			return OrgKnowledgeView{}, err
		}
	}
	return BuildOrgKnowledgeView(subject, edges, docs), nil
}
