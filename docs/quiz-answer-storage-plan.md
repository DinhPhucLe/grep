# Quiz answer storage and Codex scoring

Status: individual ungraded answer storage uses existing users and projects in source.
Migration files are prepared for review; applying migrations requires explicit approval.
Codex grading and embeddings remain planning only.

## Current implementation

- The TUI remembers existing `users._id` and `projects._id` selected through
  `--user-id` and `--project-id`, per workspace, in `cortisol/users.json`.
- There is no registration, generated identity, authentication, or new user field.
  Evaluation goes directly to `/evaluations`, independent of answer storage.
- The server checks user existence and project ownership before identified quiz
  generation and answer submission. IDs identify records; they are not credentials.
- With a selection, the TUI saves each answer via `/quiz-answers` before advancing.
  Failed saves preserve the draft; identical retries return the existing answer.
  Without a selection, chat and quizzes work and answers are labeled local only.
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
- There is no grading, embedding generation, or public answer-history endpoint yet.

## Proposed second slice: grade individually with Codex

Use Codex app-server for grading, separate from Snowflake's prompt evaluation and quiz generation. Each submission supplies the exact question, relevant implementation snapshot, expected key points derived from that evidence, and the user's answer to a dedicated grading thread. Keep it separate from the active coding conversation so grading cannot start implementation work or confuse turn state.

The installed Codex protocol exposes `thread/start` with developer instructions and `turn/start.outputSchema` for structured final output. A planning-time schema inspection confirmed these fields. Before implementation, validate the installed client's thread lifecycle and tool restrictions in a small isolated probe. Require a read-only, no-edit review; disable execution/tools where supported and never grant elevated access just to grade. Treat source comments and user answers as evidence, never grader instructions.

Proposed rubric, to agree and calibrate on human-scored examples:

- Accuracy: 0–1 for factual correctness against the exact implementation, allowing semantic paraphrases.
- Completeness: 0–1 for the proportion of required points covered; extra words do not earn credit and unasked details are not required.
- Overall: proposed `0.7 * accuracy + 0.3 * completeness`, rounded to two decimals. This measures answer quality, not a probability that the user understands the code. Define how material contradictions affect both components before shipping.
- Anchors: 0 = incorrect/no relevant understanding; about 0.5 = partly right with significant omissions; 1 = correct and covers every required point.

Request a structured object containing component scores, covered/missing points, and a short reason; calculate the final number in application code so the formula is consistent. Validate finite numeric values within [0,1]. Inadequate evidence, refusal, timeout, and malformed output produce a failed/ungraded state with no score, never a zero. Record model/rubric versions and preserve the answer for a later explicit re-evaluation. Do not mix grades from changed rubrics without identifying their version.

## Proposed third slice: find areas needing practice

Start with ordinary queries over scored answers grouped by project, file/module, and concept. Show sample count and recency alongside average accuracy/completeness; one low answer is insufficient evidence of a persistent weakness.

Then embed question/topic plus relevant code context to retrieve related concepts across files and versions. Optionally embed answers separately for misconception clustering. Vector similarity groups related material; it does not itself measure knowledge or correctness. Use the graded answers and repeated observations for that inference.

Keep user/project boundaries when retrieving history. Choose the embedding provider, dimensions, and vector index after agreeing on data scope; Codex grading does not automatically provide an embedding vector. Version embeddings so model changes can be reindexed.

## Delivery order and checks

1. Agree on identity, source-context retention, one-answer policy, and score weights.
2. Prepare the collection validator and indexes for review. Get explicit approval before applying any database migration.
3. Implement durable individual answer saves and retrieval; check duplicate submissions, invalid quiz references, restart behavior, and storage failures.
4. Add the isolated Codex grader; calibrate with correct, incomplete, contradictory, and irrelevant answers. Check that failure preserves the answer and cannot mutate the workspace or coding conversation.
5. Add ordinary topic history, then evaluate whether embeddings improve retrieval enough to justify the additional pipeline.

Storage against existing users is authorized by the user. Applying the prepared migration still requires explicit approval. Scoring and embeddings are not implemented or authorized by this plan alone.
