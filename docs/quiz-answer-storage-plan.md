# Quiz answer storage and Codex scoring

Status: individual ungraded answer storage uses existing users and projects.
Migrations 7 and 8 were applied with approval; any future migration requires explicit approval.
Codex grades answers in the TUI; grade persistence and embeddings remain planning only.

## Current implementation

- The TUI remembers existing `users._id` and `projects._id` selected through
  `--user-id` and `--project-id`, per workspace, in `cortisol/users.json`.
- There is no registration, generated identity, authentication, or new user field.
  Evaluation goes directly to `/evaluations`, independent of answer storage.
- The server checks user existence and project ownership before identified quiz
  generation and answer submission. IDs identify records; they are not credentials.
- With a selection, the TUI saves each answer via `/quiz-answers` before advancing.
  Failed saves preserve the draft; identical retries return the existing answer.
  Without a selection, chat and quizzes work and answers are labeled local only; Codex still grades them.
- The server retains at most 128 quiz snapshots for 24 hours. Saved answers embed
  the authoritative question and source snapshot. Unanswered quizzes do not survive
  a server restart, but saved answers remain retryable after restart.
- Migration 8 creates only `quiz_answers`. Root `user_id`, `project_id`, and
  `quiz_id` use BSON ObjectIDs. The unique index is `(user_id, quiz_id, question_id)`;
  the history index is `(user_id, project_id, created_at descending)`.
- The request snapshot contains prompt/context, evaluation, and source files;
  identity and Codex thread/turn IDs live at the document root. See
  `docs/quiz-answer-migration-review.md` for the exact schema.
- Numeric and null-score evaluations are saved in `evaluations`; null-score
  conversations still skip quizzes. Migration 7 supplies the nullable validator.
- No runtime code creates collections, indexes, users, or projects. No migration
  is executed without explicit approval. Missing answer schema blocks saves,
  not ordinary evaluation/chat.
- Codex grading displays accuracy and corrections in the TUI. Grades are not saved in `quiz_answers`; embedding generation and public answer-history endpoints are not implemented.

## Individual Codex grading now implemented

After each answer is saved (or accepted locally without a selected user), the TUI
runs `codex exec` in an ephemeral, read-only scratch directory. The prompt contains
one question, the answer, and numbered excerpts from the quiz's source snapshot.
The grading run is separate from the implementation conversation and does not use
Snowflake. Its JSON result contains an accuracy score from 0 to 1 and a brief
explanation when the answer is wrong or partly right. Perfect answers show only
the score. Malformed output, missing evidence, timeout, and process errors show
“Accuracy unavailable” without inventing a zero; the user can continue.

The grade is display-only for now. The existing answer record remains `ungraded`
because migration 8 introduced no grade fields. Persisted grading, calibration,
and embeddings would need a separate design and explicitly approved migration.

## Proposed third slice: find areas needing practice

Start with ordinary queries over scored answers grouped by project, file/module, and concept. Show sample count and recency alongside average accuracy/completeness; one low answer is insufficient evidence of a persistent weakness.

Then embed question/topic plus relevant code context to retrieve related concepts across files and versions. Optionally embed answers separately for misconception clustering. Vector similarity groups related material; it does not itself measure knowledge or correctness. Use the graded answers and repeated observations for that inference.

Keep user/project boundaries when retrieving history. Choose the embedding provider, dimensions, and vector index after agreeing on data scope; Codex grading does not automatically provide an embedding vector. Version embeddings so model changes can be reindexed.

## Delivery order and checks

1. Agree on identity, source-context retention, one-answer policy, and score weights.
2. Prepare the collection validator and indexes for review. Get explicit approval before applying any database migration.
3. Implement durable individual answer saves and retrieval; check duplicate submissions, invalid quiz references, restart behavior, and storage failures.
4. Calibrate the isolated Codex grader against human-scored examples, then design grade persistence separately.
5. Add ordinary topic history, then evaluate whether embeddings improve retrieval enough to justify the additional pipeline.

Storage against existing users is authorized by the user. Display-only scoring is implemented; persisted grades and embeddings remain future work.
