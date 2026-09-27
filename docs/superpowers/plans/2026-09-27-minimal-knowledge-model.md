# Minimal knowledge records

This replaces unimplemented conversation-data-model tasks 2–7 at the user's
request. Task 1 was also reverted at the user's request (commit `1a5e8b5`, pushed
to origin); the original duplicate version-5 migrations remain unresolved.
The user authorized implementation
on `conversation-data-model` and wants a testing pause after each milestone.

## One storage milestone

Add one MongoDB collection, `knowledge_records`:

| Field | Meaning |
| --- | --- |
| `_id` | Server-generated ObjectId |
| `project_id` | Existing project ObjectId |
| `source` | Origin label or locator, e.g. `conversation:local-1` |
| `topic` | Human-supplied label |
| `content` | Exact original text, never normalized or truncated |
| `embedding` | Optional supplied finite, nonzero vector |
| `embedding_model` | Required with a vector, e.g. `provider/model` |
| `created_at` | Server-generated UTC date |

Source, topic, content and model (when present) must be nonblank UTF-8 strings.
Limits: source 1024 bytes, topic/model 256 bytes, content 1 MiB, vector 1–4096
values. Embedding fields are both absent or both present. No provider or
dimension count is selected; vector length determines its dimensions. These
are storage limits, not a retrieval compatibility guarantee.

The Go repository creates a record, gets one record, and lists a project's
records newest first (created_at then _id; limit 1–100). Every operation resolves
the project's owner from `projects.user_id` using a trusted caller ID. It does
not authenticate an arbitrary supplied ID. Missing/foreign projects and records
return not found to avoid revealing other users' data. No new HTTP API or TUI
wiring is included. Every create is a new record; retry deduplication, editing,
and deletion are outside this milestone.

Migration 7 adds a strict validator and a project/date/_id index. It does not
modify sessions or evaluations. The down migration drops only this collection
and requires a separately reviewed backup/rollback decision. Do not apply live
migrations during implementation. Existing deployments with the historical
version-5 ambiguity still require reconciliation.

Assumption: this milestone is save/read with optional externally supplied
embeddings. Generating embeddings and semantic search are deferred, along with
turn state, revisions, separate chunks, generation hashes, indexing workers,
and evaluation provenance.

## Execution and acceptance

1. Replace the unfinished old-model draft with model validation and tests.
2. Add the owner-scoped repository, migration, schema checks, and opt-in MongoDB
   tests using an explicitly selected disposable database.
3. Document usage; run package tests, all Go tests, vet, and offline migration
   validation. Report live tests as skipped if no disposable database is set.
4. Commit the milestone and pause with reproducible test instructions.

Success means exact text and optional vectors can be saved/reloaded through
the repository, invalid records are rejected, and foreign owners cannot
read/write records. Synthetic vectors demonstrate storage only.
