package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"cortisol-server/internal/orgknowledge"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// KnowledgeSeedOptions controls Marketplace-import or synthetic knowledge seeding.
type KnowledgeSeedOptions struct {
	ExportPath string
	Count      int
}

// SeedKnowledge ensures the collection exists and upserts mapped knowledge documents.
func SeedKnowledge(ctx context.Context, database *mongo.Database, opts KnowledgeSeedOptions) (orgknowledge.SeedResult, error) {
	var zero orgknowledge.SeedResult
	if err := orgknowledge.EnsureSchema(ctx, database); err != nil {
		return zero, err
	}
	rows, err := loadKnowledgeRows(opts)
	if err != nil {
		return zero, err
	}
	cast := DemoCastIDs()
	docs, err := orgknowledge.MapExportRows(rows, DemoKnowledgeAuthors(), cast.NovaPayOrgID.Hex(), cast.AtlasHealthOrgID.Hex())
	if err != nil {
		return zero, err
	}
	return orgknowledge.SeedDocuments(ctx, database, docs)
}

func loadKnowledgeRows(opts KnowledgeSeedOptions) ([]orgknowledge.GitHubExportRow, error) {
	if opts.ExportPath != "" {
		data, err := os.ReadFile(opts.ExportPath)
		if err != nil {
			return nil, fmt.Errorf("read knowledge export: %w", err)
		}
		var rows []orgknowledge.GitHubExportRow
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, fmt.Errorf("parse knowledge export JSON array: %w", err)
		}
		if len(rows) == 0 {
			return nil, fmt.Errorf("knowledge export is empty")
		}
		return rows, nil
	}
	count := opts.Count
	if count <= 0 {
		count = orgknowledge.DefaultSeedCount
	}
	return orgknowledge.GenerateSyntheticExport(count), nil
}
