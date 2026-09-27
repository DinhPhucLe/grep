# API & database

Context for agents working on the Cortisol dashboard and MongoDB schema.

**Contract:** the dashboard is a presentation layer only. It never streams from the CLI.
It loads data with HTTP GETs against the server, which reads MongoDB and returns
JSON aggregates. Ingest (TUI/CLI → server) is a separate write path.

Default server listen address: `HTTP_ADDR` or `127.0.0.1:8080`.  
Dashboard Vite proxies `/api` → that address in development.

---

## Error envelope

All JSON error responses use:

```json
{
  "error": {
    "code": "invalid_request",
    "message": "human-readable reason"
  }
}
```

Common `code` values: `method_not_allowed`, `not_found`, `invalid_request`,
`unsupported_media_type`, `forbidden`, `search_failed`, `aggregate_failed`,
`persist_failed`.

---

## Dashboard REST API

Prefix: `/api/v1/dashboard/`. All routes below are **GET** only.  
IDs in path segments are MongoDB ObjectId hex strings (24 hex chars).

Practice name currently supported: `lead_and_reveal`.

### People recommendations

`GET /api/v1/dashboard/people/recommendations`

| Query | Type | Default | Notes |
|-------|------|---------|--------|
| `q` | string | empty | Case-insensitive substring match on user `name` or `mail`. Empty → most recent users by `created_at` desc. |

Limit: fixed at **10**.

**200 response**

```json
{
  "items": [
    { "id": "<objectIdHex>", "name": "string", "mail": "string" }
  ]
}
```

### Organization recommendations

`GET /api/v1/dashboard/organizations/recommendations`

| Query | Type | Default | Notes |
|-------|------|---------|--------|
| `q` | string | empty | Case-insensitive substring match on org `name`. Empty → most recent orgs by `created_at` desc. |

Limit: fixed at **10**.

**200 response**

```json
{
  "items": [
    { "id": "<objectIdHex>", "name": "string" }
  ]
}
```

### Employee practice view

`GET /api/v1/dashboard/people/{userId}/practices/{practice}`

| Query | Type | Default | Notes |
|-------|------|---------|--------|
| `year` | int | current UTC year | Must be in `1970…3000` if set. |

Aggregates `practice_events` for that user + practice + calendar year.

**200 response** — `EmployeePracticeView` (`schemaVersion`: `"employee_practice.v1"`)

| Field | Type | Meaning |
|-------|------|---------|
| `schemaVersion` | string | Always `employee_practice.v1` |
| `generatedAt` | string (RFC3339) | Server generation time |
| `subject` | object | `{ userId, organizationId, practice, year }` — `organizationId` from first event if any |
| `activityCalendar` | object | `{ status, days: [{ date, count, intensity }] }` — GitHub-style year heat |
| `outcomePie` | object | `{ status, segments: [{ label, value }], display: { primary, secondary? } }` |
| `metrics` | array | Presentation metrics (`id`, `category`, `label`, `status`, `display`, `visualization`) |
| `summary` | object? | `{ status, bullets: string[] }` |

`status` values used in viz blocks: typically `available` / `unknown` (and related metric statuses shared with the dashboard metric contract).

### Organization practice view

`GET /api/v1/dashboard/organizations/{orgId}/practices/{practice}`

| Query | Type | Default | Notes |
|-------|------|---------|--------|
| `from` | RFC3339 | Jan 1 of current UTC year | Inclusive lower bound on `started_at` |
| `to` | RFC3339 | now (UTC) | Inclusive upper bound |
| `quarter` | int `1…4` | unset | If set, replaces `from`/`to` with that quarter of `year` |
| `year` | int | current UTC year | Used with `quarter` |

**200 response** — `OrgPracticeView` (`schemaVersion`: `"org_practice.v1"`)

| Field | Type | Meaning |
|-------|------|---------|
| `schemaVersion` | string | Always `org_practice.v1` |
| `generatedAt` | string (RFC3339) | Server generation time |
| `subject` | object | `{ organizationId, practice, from, to }` |
| `codebaseTreemaps` | array | Per-project file/module heat: `{ projectId, repoName, status, root }` |
| `timeseries` | object | `{ status, points: [{ t, correct, failedReveal, medianActiveAnswerTimeMs }] }` |
| `metrics` | array | Same metric shape as employee view |
| `summary` | object? | `{ status, bullets }` |

### Frontend routes (not HTTP API)

| Path | Loads |
|------|--------|
| `/` | Empty landing + people/org search (recommendations APIs) |
| `/people/:id` | Employee practice view (`practice` query, default `lead_and_reveal`) |
| `/organizations/:id` | Org practice view |

---

## Practice ingest (feeds the dashboard)

Not a dashboard GET, but this is how practice rows land in MongoDB.

`POST /api/v1/practice-events`  
`Content-Type: application/json`

Body fields (snake_case):

| Field | Type | Required |
|-------|------|----------|
| `practice` | string | yes — must be `lead_and_reveal` |
| `organization_id` | ObjectId hex | yes |
| `user_id` | ObjectId hex | yes — must be a member of the org |
| `session_id` | ObjectId hex | yes |
| `project_id` | ObjectId hex | yes |
| `started_at` / `ended_at` | RFC3339 | yes |
| `total_duration_ms` / `active_answer_time_ms` | int64 | yes ≥ 0 |
| `attempts` | int | yes — `1` or `2` |
| `outcome` | string | yes — `correct` or `failed_reveal` |
| `points_delta` | int \| null | yes (may be null) |
| `repo_name`, `repo_org`, `file_path`, `module` | string | yes (non-blank where validated) |
| `start_line`, `end_line` | int | yes — valid range |
| `answer_quality`, `relevance` | string \| omit | optional |

**201** — stored `Event` document (JSON, includes generated `id`).  
**403** — user not in `organization_members` for that org.

One event = one completed practice **instance** (whole quiz), not one LLM question.

---

## MongoDB schema

Migrations live in `server/internal/db/migrations/` as paired  
`NNNNNN_name.up.json` / `NNNNNN_name.down.json` (golang-migrate MongoDB JSON driver).

Version tracking collection: `schema_migrations` (`version`, `dirty`).

Current application collections by migration version:

| Version | Collections added |
|---------|-------------------|
| 1 | `users` |
| 2 | `projects` (+ index `projects_by_user`) |
| 3 | `sessions` (+ index `sessions_by_project`) |
| 4 | `evaluations` (prompt evaluation store; not dashboard practice) |
| 5 | `organizations`, `organization_members`, `practice_events` (+ indexes) |

Validators use `$jsonSchema` with `additionalProperties: false` and
`validationAction: "error"`. Application code owns referential integrity
(ObjectId references are not DB-enforced FKs).

### `users`

| Field | BSON | Notes |
|-------|------|--------|
| `_id` | objectId | |
| `name` | string | |
| `mail` | string | |
| `created_at` | date | |

### `projects`

| Field | BSON | Notes |
|-------|------|--------|
| `_id` | objectId | |
| `user_id` | objectId | → `users._id` |
| `name` | string | |
| `root_path` | string | |
| `repo_url` | string | |

### `sessions`

| Field | BSON | Notes |
|-------|------|--------|
| `_id` | objectId | |
| `project_id` | objectId | → `projects._id` |
| `agent_model` | string | |
| `started_at` | date | |
| `ended_at` | date \| null | |
| `branch` | string | |
| `status` | string | app-managed |
| `shell` | string | |

### `organizations`

| Field | BSON | Notes |
|-------|------|--------|
| `_id` | objectId | |
| `name` | string | |
| `created_at` | date | |

### `organization_members`

| Field | BSON | Notes |
|-------|------|--------|
| `_id` | objectId | |
| `organization_id` | objectId | → `organizations._id` |
| `user_id` | objectId | → `users._id` |
| `role` | string | app-managed (`admin` / `member` in seed) |
| `joined_at` | date | |

Indexes: unique `(organization_id, user_id)`; by org; by user.

### `practice_events`

One document per completed practice instance. Fields match the Go
`practice.Event` / ingest body (snake_case in BSON). Notable indexes:

- `(organization_id, practice, started_at)`
- `(user_id, practice, started_at)`
- `(organization_id, project_id, practice)`
- `session_id`

Optional nullable fields: `answer_quality`, `relevance`.

### `evaluations`

Used by `/evaluations` / jobs (Cortex), not by the practice dashboard views.

### `knowledge_records`

Minimal project-scoped storage in `server/internal/knowledge`:

| Field | Meaning |
| --- | --- |
| `_id`, `project_id` | ObjectId identity and existing project |
| `source` | Origin label or locator, e.g. `conversation:local-1` |
| `topic` | Short human-supplied label |
| `content` | Exact source text |
| `embedding`, `embedding_model` | Optional vector and its model label, supplied together |
| `created_at` | Server-generated UTC BSON date |

The repository exposes `Create(ctx, callerID, projectID, Input)`,
`Get(ctx, callerID, projectID, recordID)`, and
`List(ctx, callerID, projectID, limit)`. Every operation checks the current
`projects.user_id` owner. Caller IDs must come from a trusted authenticated or
administrative context; accepting an ID is not authentication. Missing and
foreign projects both return `knowledge.ErrNotFound`. List accepts 1–100 and
orders by `created_at` descending, then `_id` descending. Each create inserts
a new record; repeated source labels are allowed.

Example input (within Go code that already has a database and trusted caller):

```go
repo := knowledge.NewMongoRepository(database)
record, err := repo.Create(ctx, callerID, projectID, knowledge.Input{
    Source: "conversation:local-1",
    Topic: "Deployment",
    Content: "Preserve the original text here.",
})
```

Validation preserves text without trimming or truncating it. Nonblank UTF-8
strings are required: source ≤1024 bytes, topic/model ≤256 bytes, content ≤1 MiB.
An optional vector has 1–4096 finite values and cannot be all zero. MongoDB
validates basic types, character limits, and paired embedding fields; Go also
enforces byte limits and finite/nonzero vectors. Vector length supplies its
dimension count. No embedding provider is selected or called.

This is storage scaffolding only: no public endpoint, TUI capture, embedding
generation, semantic search, revision tracking, or indexing worker is wired in.
Migration `000007_knowledge_records` creates just this collection and an ordinary
project/date/ID index. Its down migration drops the collection and all its data;
use only after a separate rollback decision and backup.

**Migration baseline blocker:** the original two version-5 migration pairs remain
because the baseline repair was reverted at the user's request. The full offline
migration check fails on that existing duplication. The new pair is tested in
isolation, but full deployment must wait for a separately agreed reconciliation.
No migrations or seed writes were performed for this milestone.

---

## Developing the database

Work from `server/` with a `.env` containing at least:

```bash
MONGODB_URI=...
MONGODB_DATABASE=cortisol
```

Same env is used by `cmd/server`, `cmd/migrate`, and `cmd/seed`.

### 1. Validate migration files (no DB)

```bash
cd server
go run ./cmd/migrate -check
```

Parses every `*.up.json` / `*.down.json` pair and checks golang-migrate naming.

### 2. Apply migrations

```bash
cd server
go run ./cmd/migrate
```

- Applies **pending up** migrations only (no automatic down).
- Records version in `schema_migrations`.
- After up, verifies required collections exist for that version
  (`internal/db/schema_integrity.go`). If version is stamped but collections
  are missing, migrate fails with a repair hint.

### 3. Seed demo data

Requires **clean** migration version **≥ 5** and all collections present.

```bash
cd server
go run ./cmd/seed
```

Behavior:

- Upserts with `$setOnInsert` on `_id` — safe to re-run; never overwrites
  existing docs.
- Ordered inserts; not a single transaction — re-run if interrupted.
- Prints stable “demo cast” ObjectIds for dashboard URLs.

Deterministic seed IDs (prefix `0x66`):

| Role | Hex id |
|------|--------|
| NovaPay org | `660100000000000004000000` |
| AtlasHealth org | `660200000000000004000000` |
| Alex Rivera (NovaPay) | `660100000000000001010000` |
| Jordan Kim (NovaPay) | `660100000000000001020000` |
| Sam Okonkwo (AtlasHealth) | `660200000000000001010000` |

Example after seed:

```text
GET /api/v1/dashboard/people/660100000000000001010000/practices/lead_and_reveal?year=2026
GET /api/v1/dashboard/organizations/660100000000000004000000/practices/lead_and_reveal
```

### 4. Run the API server

```bash
cd server
go run ./cmd/server
```

### Typical local loop

1. Edit or add migration JSON under `internal/db/migrations/`.
2. `go run ./cmd/migrate -check`
3. `go run ./cmd/migrate`
4. If schema changed in a way seed depends on, adjust `internal/db/seed/` and
   update `RequiredCollectionsForVersion` when adding a new version’s
   collections.
5. `go run ./cmd/seed` (optional for demo)
6. `go run ./cmd/server` + dashboard `npm run dev`

### Adding a new migration

1. Create `00000N_description.up.json` and matching `.down.json` (JSON array of
   Mongo commands: `create`, `createIndexes`, `drop`, etc.).
2. Extend `RequiredCollectionsForVersion` if the up creates collections the
   app expects.
3. Bump any seed gate that hard-codes a minimum version (currently `>= 5` in
   `seed.Seed`).
4. Run migrate; never hand-edit production data to “look migrated” without
   updating `schema_migrations` consistently.

### Repair: dirty or incomplete version

If golang-migrate marks `dirty`, or version N is recorded but collections are
missing:

1. Inspect Atlas / local DB: which collections exist, what `schema_migrations`
   says.
2. Force `schema_migrations.version` back to **N−1** with `dirty: false`
   (only after understanding partial applies — may need to drop half-created
   collections manually).
3. Re-run `go run ./cmd/migrate`, then `go run ./cmd/seed` if needed.

Do not invent alternate migrate CLIs; this repo’s entrypoint is
`go run ./cmd/migrate`.

### Source of truth

| Concern | Location |
|---------|----------|
| Dashboard HTTP routing | `server/internal/practice/dashboard_handler.go` |
| Aggregates / JSON shapes | `server/internal/practice/aggregate.go` |
| Event model + validation | `server/internal/practice/model.go` |
| Ingest | `server/internal/practice/handler.go` |
| Directory search (recs) | `server/internal/practice/directory.go` |
| Migrations | `server/internal/db/migrations/` |
| Seed data | `server/internal/db/seed/` |
| Dashboard TS contracts | `dashboard/src/contracts/practice.ts`, `search.ts` |

When code and this file disagree, **update this file** to match the code.
