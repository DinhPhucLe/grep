# Client-facing implementation review

`POST /quizzes` generates review questions for an ambiguous implementation request.
The local API needs no application credentials. It never grades answers.
`POST /quiz-answers` saves individually graded answers. `/jobs`, `/quizzes/batch`,
and application authentication endpoints are absent.

## Request

Send `Content-Type: application/json` with:

- `input`: the original implementation request.
- `context` and `conversation`: optional relevant requirements/history.
- `evaluation`: an `ambiguous` verdict, numeric `ambiguity_score > 0.30`, summary,
  and nonempty consequential client-facing gaps. Null scores are not quizzable.
- `files`: 1–30 generated text files with normalized relative `path` and complete
  `content`. Each file is at most 128,000 bytes; combined text is at most 256,000
  bytes and the HTTP body at most 1 MiB. The endpoint never reads local files.
- `max_questions`: 1–4; omitted or zero means 4.
- For answer storage: `user_id` is an existing MongoDB ObjectID hex string,
  plus Codex `thread_id` and `turn_id`. The TUI uses the current signed-in user's
  ID from its session for both quiz generation and answer submission.
  The server checks that the user exists. These metadata fields are not sent
  to Snowflake.

## Response

```json
{
  "model": "configured-model",
  "prompt_version": "client-behavior-quiz-v5",
  "questions": [
    {
      "id": "q1",
      "topic": "Unsaved work",
      "question": "You close the page with an unsaved note. What happens when you reopen it?",
      "gap_indices": [0],
      "evidence": [{"file_path": "notes.go", "start_line": 12, "end_line": 18}]
    }
  ],
  "no_questions_reason": ""
}
```

The model receives numbered copies of files to ground exact one-based line ranges.
Original snapshots remain unchanged. Every question must address a distinct,
consequential client-visible outcome supported by a flagged gap and actual code.
Prioritize the most consequential behaviors. One good question is sufficient; never fill a quota. No syntax trivia, underlying
architecture, queues, databases, frameworks, API-call optimization, or decisions
about batching/individual grading. Do not ask the client for design approval.

All generated questions are checked for valid structure and real file/line/gap
references. Later questions overlapping retained evidence are dropped and IDs
renumbered. `quiz.independent_questions` logs generated and retained counts, not
payloads. This avoids rejecting an otherwise usable response and reduces repetitive coverage. Semantic quality still depends on the model.

If only internal mechanics or no consequential client behavior can be tested,
return `questions: []` with a nonempty `no_questions_reason`. The TUI silently
skips the quiz. No generated files also skips generation entirely.

## Answers and errors

Answers are graded individually by an isolated Codex CLI run before submission.
The TUI sends one answer, score (`graded`, 0–1), and `reasoning` to
`POST /quiz-answers`; partial and incorrect answers require reasoning. The
server validates the input, checks that the user exists, and uses its
quiz snapshot to add `quiz_question`, the original `prompt`, and `created_at`.
The record also retains `quiz_id` and `question_id` for unique submissions. Identical retries return the existing receipt;
conflicting answers or grades return 409. No grade API or batching is used.

Each saved question also creates one `practice_events` record for the dashboard.
The event reuses the answer's `_id` and retains its `user_id`, `quiz_id`,
`question_id`, numeric grade, and submission time. A grade of 1 maps to `correct`;
lower grades map to `failed_reveal`. Retries reuse the same event. If projection
fails after the answer was saved, retrying the answer repairs the missing event.
The configured login organization is used only when the user is a member;
otherwise a sole membership is used, or organization is left unknown.
Organization attribution is captured when the event is first inserted.

Quiz-answer events have no measured duration, project, or code-location data.
They contribute to activity, outcome counts, and organization time series, but
not timing medians or codebase heatmaps. Existing practice events remain intact.

Migration 14 preserves the existing practice-event schema and adds a separate
quiz-answer variant. Apply it before running the updated API or backfill. The
down migration restores the old validator without deleting projected events;
quiz-answer writes require migration 14 to be reapplied.

Backfill existing graded answers from `server/`:

```sh
go run ./cmd/migrate                          # includes migration 14
go run ./cmd/backfill-practice                 # validate/count only
go run ./cmd/backfill-practice -apply          # idempotent backfill for all users
go run ./cmd/backfill-practice -apply -user-id <ObjectID>  # one user
```

An opt-in integration check projects only a selected user's existing answers and
verifies concurrent retries, repeated backfill, and dashboard aggregation:

```sh
CORTISOL_VERIFY_PRACTICE_USER=<ObjectID> go test ./internal/practice -run TestLiveQuizAnswerProjection -v -count=1
```

Migration 8 created `quiz_answers` and its indexes. Migration 9 changes the
validator for the concise graded record; it was applied with approval. Existing
ungraded records remain untouched. Runtime code does not create collections or
run migrations. Migration 10 removes `project_id` from new answer records and adds a user-history
index; it was applied with approval. The configured database is at version 10. Unidentified quizzes can be graded locally but
cannot save answers. Unanswered quiz snapshots expire after 24 hours and do not
survive a server restart.

Generation uses one Cortex call with a deadline and no automatic retry. Invalid
output returns 502 with a safe validation reason; provider failures return 502 and
timeouts return 504. The TUI displays errors and returns to chat. Implementation output and files stay
visible throughout; quiz panels list references without dumping source.

Run `go test ./...` from `server/`. Coverage includes grounded generation, overlap
recovery, null-score rejection, local answers, editor navigation, and captured-response rendering.
