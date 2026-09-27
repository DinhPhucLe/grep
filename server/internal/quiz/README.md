# Client-facing implementation review

`POST /quizzes` generates review questions for an ambiguous implementation request.
The local API needs no application credentials. It never grades answers.
`POST /quiz-answers` saves individual ungraded answers. `/jobs`, `/quizzes/batch`,
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
- For answer storage: `user_id` and `project_id` are existing MongoDB ObjectID
  hex strings, plus Codex `thread_id` and `turn_id`. The TUI uses the locally
  selected IDs. The server reads `users` and verifies `projects.user_id` matches
  the selected user. These metadata fields are not sent to Snowflake.

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

Answers are submitted individually by the TUI. Each question opens
its source reference in VS Code; Ctrl+O cycles additional references. Enter records
the answer only after MongoDB acknowledges it; Enter again advances. Nothing is
submitted for grading. Future grading is predefined as individual per question,
not a client decision. The TUI grades each answer with an isolated Codex CLI
run after submission and displays a 0–1 accuracy score. No grading API, batch
submission, worker queue, or persisted score exists.

Identified generation returns `quiz_id`. The server holds at most 128 unanswered
quiz snapshots for 24 hours. `POST /quiz-answers` accepts only `user_id`,
`quiz_id`, `question_id`, and `answer` (1–8000 bytes). It looks up the question and
code from the server snapshot, and stores a self-contained `quiz_answers` record
with status `ungraded`. Duplicate identical submissions return the existing
receipt; different text for an answered question returns 409. Saved receipts can
be recovered by resubmitting the same answer even after a server restart. Unsaved
quiz snapshots do not survive restart. There is no public answer-list endpoint.

Migration 8 creates only `quiz_answers` and its indexes. Stored user/project/quiz
IDs are BSON ObjectIDs; identity metadata is stored at the document root, not
inside the request snapshot. Runtime requires `one_answer_per_user_question`
to be a non-partial unique index on `(user_id, quiz_id, question_id)`. Runtime code does not
create missing collections or run migrations. Existing unidentified callers can
still generate transient quizzes, but cannot save answers from those quizzes.

Generation uses one Cortex call with a deadline and no automatic retry. Invalid
output returns 502 with a safe validation reason; provider failures return 502 and
timeouts return 504. The TUI displays errors and returns to chat. Implementation output and files stay
visible throughout; quiz panels list references without dumping source.

Run `go test ./...` from `server/`. Coverage includes grounded generation, overlap
recovery, null-score rejection, local answers, editor navigation, and captured-response rendering.
