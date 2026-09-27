# Quiz answer storage and Codex scoring — proposal

Status: participant profiles and individual ungraded answer persistence are now authorized and implemented in source. Migration 8 is prepared but remains unapplied pending explicit approval. Codex grading and embeddings remain planning only.

## Current implementation

- One persistent local UUID identifies the participant without authentication. `/participants` stores the profile; optional names require no email or password.
- The TUI saves each answer via `/quiz-answers` before advancing. Failed saves preserve the draft and can be retried idempotently.
- Generated quizzes are held temporarily in a bounded, expiring server registry. Saved answer records contain the server's question and source snapshot, so grading does not depend on later edits or client-supplied evidence.
- MongoDB collections `participants` and `quiz_answers` require migration 8, which has not been applied. Runtime writes refuse missing collections. Both numeric and null-score evaluations are saved in `evaluations`; null-score conversations still skip quizzes. The nullable validator requires migration 7, restored for review but not applied in this change.
- There is no answer grading, embedding generation, or public answer-history endpoint yet.

## Storage design

Use a MongoDB collection named `quiz_answers` (the collection equivalent of a table). One document represents one submitted answer to one question. Embed the question and grading evidence in that document initially so interpretation does not depend on mutable files or an unsaved quiz object.

Suggested fields:

| Group | Fields and purpose |
| --- | --- |
| Identity | `_id`, `participant_id`, `project_id`, `codex_thread_id`, `codex_turn_id`, `quiz_run_id`, `question_id`, `evaluation_id` when available |
| Question | Text, topic, flagged gaps, generator model and prompt version |
| Evidence | Repository identity, commit when available, normalized paths, line ranges, symbols/concepts when known, content hashes, and enough source snapshot/context to judge the answer |
| Submission | Answer text, submission timestamp, idempotency key |
| Evaluation | `status: pending/completed/failed`, accuracy, completeness, overall score, covered/missing points, short evidence-based explanation, grader model, rubric version, scoring timestamp |
| Later embeddings | Concept/context text, vector, embedding model/version, generation timestamp |

`q1` is only unique within a quiz. Use a unique index on `(quiz_run_id, question_id, participant_id)` for the current one-answer-per-question policy. A repeated identical submission returns the existing record; conflicting text does not silently overwrite it. Index participant/project/time for history queries. Do not use bare line numbers as stable identities: files move and lines change, and uncommitted code may differ from the recorded commit.

The user selected a persistent local participant UUID to preserve the no-login flow, stored separately from existing users. A local UUID identifies an installation/profile, not a verified person, and is not access control. Do not add authentication as part of this proposal.

Save each answer before grading so failures do not lose it. The server should retain the generated question/evidence and validate answer links against that record; do not trust an answer request to supply its own authoritative question or score. Initially this can be a bounded in-memory quiz registry, with answer documents becoming self-contained on submission. Unanswered questions would not survive a server restart; durable quiz runs can be added later if resume/abandonment tracking is required.

Individual answer submission is implemented. A participant/project history read operation remains future work. Show a save error without discarding the local draft. Resending the same submission must not create a second answer or a second grade. No batch-query endpoint or answer worker pool is part of this first slice.

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

Keep participant/project boundaries when retrieving history. Choose the embedding provider, dimensions, and vector index after agreeing on data scope; Codex grading does not automatically provide an embedding vector. Version embeddings so model changes can be reindexed.

## Delivery order and checks

1. Agree on identity, source-context retention, one-answer policy, and score weights.
2. Prepare the collection validator and indexes for review. Get explicit approval before applying any database migration.
3. Implement durable individual answer saves and retrieval; check duplicate submissions, invalid quiz references, restart behavior, and storage failures.
4. Add the isolated Codex grader; calibrate with correct, incomplete, contradictory, and irrelevant answers. Check that failure preserves the answer and cannot mutate the workspace or coding conversation.
5. Add ordinary topic history, then evaluate whether embeddings improve retrieval enough to justify the additional pipeline.

Storage was subsequently authorized by the user. Applying the prepared migration still requires explicit approval. Scoring and embeddings are not implemented or authorized by this plan alone.
