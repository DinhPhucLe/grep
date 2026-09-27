package practice

import (
	"context"

	"cortisol-server/internal/orgknowledge"
)

// KnowledgeDashboard adapts orgknowledge.Summarizer to the dashboard HTTP layer.
type KnowledgeDashboard struct {
	inner *orgknowledge.Summarizer
}

func NewKnowledgeDashboard(summarizer *orgknowledge.Summarizer) *KnowledgeDashboard {
	return &KnowledgeDashboard{inner: summarizer}
}

func (k *KnowledgeDashboard) OrgKnowledgeSummary(ctx context.Context, organizationID string) (any, error) {
	return k.inner.OrgSummary(ctx, organizationID)
}
