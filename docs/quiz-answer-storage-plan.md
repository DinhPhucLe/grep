# Quiz answer storage and Codex scoring

The TUI grades each answer with an isolated Codex CLI run, then submits the
answer and grade together to `/quiz-answers`. The server stores the answer,
`quiz_question`, existing `user_id`, numeric `graded` score (0–1), `reasoning`,
original `prompt`, and `created_at`. It also retains `project_id`, `quiz_id`,
and `question_id` to check ownership and prevent duplicate answers. Existing
users are referenced; there is no login or participant table.

Migration 8 created the collection. Migration 9 updates its validator for the
new concise record; it was applied with approval. Existing
ungraded records remain untouched. MongoDB's `moderate` validation checks new
inserts while grandfathering old records. The TUI keeps the draft if grading
fails, and keeps the grade for a save retry. Without selected user/project IDs,
grades appear locally but are not stored. No migration runs on startup or during
a request; future migrations require explicit permission.

## Proposed third slice: find areas needing practice

Start with ordinary queries over scored answers grouped by project, file/module, and concept. Show sample count and recency alongside average accuracy/completeness; one low answer is insufficient evidence of a persistent weakness.

Then embed question/topic plus relevant code context to retrieve related concepts across files and versions. Optionally embed answers separately for misconception clustering. Vector similarity groups related material; it does not itself measure knowledge or correctness. Use the graded answers and repeated observations for that inference.

Keep user/project boundaries when retrieving history. Choose the embedding provider, dimensions, and vector index after agreeing on data scope; Codex grading does not automatically provide an embedding vector. Version embeddings so model changes can be reindexed.

## Delivery order and checks

1. Agree on identity, source-context retention, one-answer policy, and score weights.
2. Prepare the collection validator and indexes for review. Get explicit approval before applying any database migration.
3. Implement durable individual answer saves and retrieval; check duplicate submissions, invalid quiz references, restart behavior, and storage failures.
4. Calibrate the isolated Codex grader against human-scored examples.
5. Add ordinary topic history, then evaluate whether embeddings improve retrieval enough to justify the additional pipeline.

Graded persistence is implemented in code and migration 9 was applied with approval. Embeddings remain future work.
