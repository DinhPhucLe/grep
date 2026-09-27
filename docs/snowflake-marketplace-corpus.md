# Snowflake Marketplace → Atlas knowledge corpus

Build a pitch-ready org knowledge base from free Marketplace text, then let
Atlas Automated Embedding (`voyage-code-4`) handle vectors. Cortisol does **not**
store embedding arrays on documents.

## Prerequisites

- Snowflake account with a role that can **Get** Marketplace listings, plus a
  warehouse (this is separate from Cortex REST used by prompt evaluation).
- MongoDB Atlas cluster for Cortisol (`MONGODB_URI` / `MONGODB_DATABASE`).
- Before bulk export: open the listing **Terms**. If redistribution is
  restricted, query only in Snowflake and seed Atlas from a transformed demo
  subset you are allowed to use, or use the bundled generator / sample export
  in this repo.

## Marketplace search keywords

Open [Snowflake Marketplace data products](https://app.snowflake.com/marketplace/data-products),
filter **Pricing → Free**, and search:

| Priority | Keywords |
|----------|----------|
| Primary | `github`, `github archive`, `Snowflake Public Data`, `public data free` |
| Eng Q&A | `stack overflow`, `stackoverflow` |
| Tickets / ops | `support ticket`, `customer support`, `service desk`, `ITSM` |
| Weaker fit | `SEC filings`, `10-K`, `documentation` |

**Avoid for this wedge:** weather, retail POS, demographics-only, pure market feeds.

**Get first:** **GitHub Archive** and/or **Snowflake Public Data (Free)**. After
Get, inspect schemas for issue / PR / comment title and body columns (names vary
by listing version). Prefer non-empty body text.

## Export shape (canonical)

Export JSON or JSONL with one object per row matching this schema (used by the
seed mapper):

```json
{
  "event_id": "1234567890",
  "event_type": "IssuesEvent",
  "repo_name": "novapay/novapay-api",
  "actor_login": "alice",
  "title": "Retry card network 429 with jitter",
  "body": "We should cap retries at 5 and never retry 400s.",
  "created_at": "2025-06-01T15:04:05Z",
  "labels": ["payments", "reliability"]
}
```

Sample Snowsight SQL templates: [`server/internal/db/seed/sql/github_archive_sample.sql`](../server/internal/db/seed/sql/github_archive_sample.sql).

A small checked-in sample: [`server/internal/db/seed/testdata/github_export_sample.json`](../server/internal/db/seed/testdata/github_export_sample.json).

## Seed into Atlas (no client embeddings)

From `server/`:

```bash
# Creates knowledge_documents if needed, inserts mapped cards, optional Voyage index
go run ./cmd/seed -knowledge -knowledge-count 1500
# Or import a Marketplace export:
go run ./cmd/seed -knowledge -knowledge-export path/to/export.json
# Ensure Atlas autoEmbed index (voyage-code-4) after documents exist:
go run ./cmd/seed -knowledge-vector-index
```

Documents use NovaPay / AtlasHealth demo cast attribution. Atlas embeds `content`
via the `knowledge_content_voyage` vector search index (`voyage-code-4`, 1024d).

## Dashboard pitch view

After seeding:

```text
GET /api/v1/dashboard/organizations/{orgId}/knowledge
```

Dashboard route: `/organizations/{orgId}/knowledge`.
