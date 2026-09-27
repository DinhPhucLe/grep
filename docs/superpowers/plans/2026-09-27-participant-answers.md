# Local participants and quiz answers implementation plan

**Goal:** Register persistent local participant profiles and save individual quiz answers in MongoDB, without authentication or grading.

**Architecture:** The TUI keeps a UUID in the user's config directory. The server upserts participant profiles and remembers generated quizzes briefly so answers can be saved with authoritative question/evidence snapshots. Answer writes are immutable and idempotent per participant/quiz/question.

**Spec:** `docs/quiz-answer-storage-plan.md`, with participant and answer storage now authorized by the user. Grading, embeddings, and migrations remain separate approvals/work.

**Constraints:** No authentication, password/email requirement, batch endpoint, grading, or automatic migration. Preserve current quiz UI/editor and prompt-rubric changes. No live database writes before collection migration approval.

- [ ] Profiles: `internal/participant` implements bounded POST registration, Mongo upsert, UUID validation, and safe missing-collection errors; `cmd/tui/participant.go` creates/reuses the local UUID atomically. Test repeat/concurrent startup and corrupt identity files.
- [ ] Answers: `internal/quiz/answers*.go` records authoritative generation snapshots with a bounded expiring registry, writes ungraded answers individually, and returns existing answers on duplicate submissions. Test conflicts, expired/unknown quiz, unknown participant, storage errors, and concurrent access.
- [ ] TUI: load ID at startup; register before each prompt evaluation; attach ID/project/thread/turn metadata to quiz generation; save answers before advancing and retain drafts on failure. Ignore stale save callbacks. Test failure/duplicate/later callbacks and actual serialized requests.
- [ ] Routes and migration: expose `/participants` and `/quiz-answers` with no auth; prepare a new migration creating only `participants` and `quiz_answers` plus indexes. Verify files with `go run ./cmd/migrate -check`; do not apply them.
- [ ] Review and validation: run server tests/race checks, vet, build and diff checks; review migration commands and obtain user's approval as the final step before applying.

Collection creation is explicitly guarded: runtime writes must fail safely if the collections are absent, rather than implicitly creating them ahead of migration approval.
