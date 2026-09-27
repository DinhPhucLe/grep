# Migration 9: graded quiz answers (applied with approval)

Version 9 changes only the `quiz_answers` validator. It keeps the collection,
the existing unique `(user_id, quiz_id, question_id)` index, the user/project
history index, and all existing documents. It does not add login, users, or
participant records.

New documents require `_id`, `user_id`, `project_id`, `quiz_id`, `question_id`,
`answer`, `quiz_question`, `graded`, `reasoning`, `prompt`, and `created_at`.
`graded` must be numeric from 0 to 1. The server supplies the question, prompt,
and timestamp from its quiz snapshot; the TUI supplies the user's answer and
Codex score. The three quiz/project identifiers support ownership checks and
idempotent retries. No source-file snapshot, generator metadata, or status field
is written in the new record.

The validator uses `validationLevel: "moderate"` so it checks new inserts while
leaving legacy ungraded documents intact. Those older documents have no honest
score to backfill. [MongoDB documents this validation behavior](https://www.mongodb.com/docs/manual/core/schema-validation/specify-validation-level/).

The down file restores the version 8 validator without deleting documents.
Documents written under version 9 would not match that older validator, so a
full application rollback needs a separate data decision before applying down.

Validate files without connecting to MongoDB:

```sh
cd server
go run ./cmd/migrate -check
```

Version 9 was applied after explicit user approval. No application startup
or request handler applies migrations.
