# Migration 8: quiz answers — ready for review, not applied

This replaces the earlier, unapplied `000008_participant_answers` draft.
It creates **only `quiz_answers`**. It does not create `participants` or `quizzes`,
alter `users`/`projects`/`sessions`/`evaluations`, or modify existing records.
Version 6 was a retired authentication draft. Version 7 restores the nullable
evaluation validator; version 8 remains the separate quiz-answer migration.
The nullable evaluation files are `000007_evaluation_intent.up.json` and
`000007_evaluation_intent.down.json`. They restore the original migration 7,
which was previously applied and then rolled back. Neither has been executed
in this restoration. The default migration command applies **all** pending
versions, so approval for version 7 alone must not also apply version 8.

## Files

- Up: `server/internal/db/migrations/000008_quiz_answers.up.json`
- Down: `server/internal/db/migrations/000008_quiz_answers.down.json`

## Stored document

| Fields | Purpose |
| --- | --- |
| `_id` | Answer ObjectId |
| `user_id` | Existing `users._id`, as an ObjectId; no new user fields |
| `project_id` | Existing `projects._id`, as an ObjectId |
| `quiz_id`, `question_id` | Identify the generated quiz run and its question; `quiz_id` is an ObjectId, not a reference to a new collection |
| `thread_id`, `turn_id` | Codex implementation provenance |
| `answer`, `created_at`, `status` | Submitted text, initial server timestamp, and `ungraded` |
| `model`, `prompt_version` | Quiz generator provenance |
| `question` | Exact question, topic, gap indices, and file/line references |
| `request` | Original prompt/context, ambiguity evaluation, and source snapshots needed to interpret the answer later |

All root fields are required. The request's context, conversation, and question
limit are optional. Unknown properties are rejected at every defined object level.
No authentication, participant, grading-score, or embedding fields are introduced.

The validator uses supported MongoDB JSON Schema keywords, including `bsonType`
and the draft-4 boolean form of `exclusiveMinimum`.
[MongoDB JSON Schema reference](https://www.mongodb.com/docs/manual/reference/operator/query/jsonschema/).

The application still needs to check user/project existence, project ownership,
question ID consistency, source bounds, nonblank content, and byte limits. Schema
validation alone does not perform those cross-record or cross-field checks.
Neither user nor project identity is authentication.

## Indexes

- Unique `(user_id, quiz_id, question_id)` named `one_answer_per_user_question`:
  one saved answer per user/question. Identical retries return the existing answer;
  conflicting answers must be rejected by the application rather than overwritten.
- `(user_id, project_id, created_at descending)` named `user_project_answer_history`:
  supports recent answer history for a user and codebase.

## Rollback and deployment boundaries

The down migration drops **only `quiz_answers`**, including any answers stored
there. A future rollback therefore needs a backup/export if those answers must be
kept, and explicit approval before execution.

The application now uses existing users and projects, stores ObjectID references,
checks project ownership, checks the unique index, and omits identity metadata
from the nested request snapshot. Users and projects are read only; their schemas
and records are unchanged. The TUI remembers an explicitly selected user/project
locally and does not register a profile before evaluation.

Validation performed without a database connection:

```sh
cd server
go run ./cmd/migrate -check
go test ./internal/db -count=1
```

These checks validate migration structure and JSON syntax; they do not apply the
commands or prove live MongoDB acceptance. **No migration was applied during these checks.**
