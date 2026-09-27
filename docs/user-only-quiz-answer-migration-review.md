# Migration 10: user-only quiz answers (applied with approval)

Version 9 is already applied, so its migration file remains historical. Version
10 changes the `quiz_answers` validator to remove `project_id` from new records.
It replaces the old `(user_id, project_id, created_at)` history index with
`(user_id, created_at)`. The unique `(user_id, quiz_id, question_id)` index stays.

Existing answer documents are not changed or deleted. The validator retains
`validationLevel: "moderate"` so old documents with `project_id` remain in place,
while new inserts must use the project-free shape. The down migration restores
version 9's validator and old history index; it does not delete answers. New
project-free records would not satisfy version 9's validator after rollback,
so a full rollback needs a separate data decision.

The application user is still required. `DB_USERNAME` in `.env` is a MongoDB
connection credential and does not identify a record in `users`. The TUI saves
only after an existing user ID is selected with `--user-id`; no project or login
is needed.

Validate without connecting to MongoDB:

```sh
cd server
go run ./cmd/migrate -check
```

Version 10 was applied after explicit approval. No application startup or
request handler applies migrations.
