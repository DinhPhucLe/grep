# Question generation API

`POST /quizzes` generates an implementation-grounded quiz through the existing
Snowflake Cortex client. It uses the server's `SNOWFLAKE_MODEL` (configure a
supported Claude model to use Claude), account URL, PAT, and evaluation timeout.
This is a new route on our Go server; no new Snowflake-side endpoint is needed.

This first version is stateless and is not connected to the TUI. It does not
save quizzes, grade answers, award points, or hide/apply changes. HTTP 200 means
generation completed, not that a durable quiz/session was created.

## Request

Send `Content-Type: application/json` with:

```json
{
  "input": "fix login",
  "context": "A login endpoint for a public web app",
  "conversation": [],
  "evaluation": {
    "verdict": "ambiguous",
    "summary": "The required failure behavior is unspecified.",
    "ambiguity_score": 0.7,
    "gaps": [
      {
        "description": "Unknown-user and wrong-password responses are unspecified.",
        "consequence": "Different responses could expose whether an account exists."
      }
    ]
  },
  "files": [
    {
      "path": "login.go",
      "content": "package login\n\nfunc failureMessage() string { return \"Invalid credentials\" }\n"
    }
  ],
  "max_questions": 6
}
```

- `input`, `evaluation`, and `files` are required. Only ambiguous evaluations
  with a score **strictly greater than 0.3** and nonempty gaps are accepted.
- `context` and `conversation` are optional. Include relevant earlier
  requirements so a short follow-up isn't interpreted in isolation.
- `files` contains 1–30 complete generated text files, with unique normalized
  repository-relative paths using forward slashes. These are caller-supplied;
  the endpoint does not read files or run Codex. Submit only relevant files.
- Each file is limited to 128,000 bytes; combined request text to 256,000 bytes;
  HTTP body to 1 MiB. Prompt/conversation limits match `/evaluations`.
- `max_questions` is 4, 5, or 6; omitted/zero means 6. The model aims for four
  to the requested cap, but may return fewer when distinct grounded questions
  are unavailable. It must not invent or repeat questions to fill a quota.

For a manual test, save a request as `quiz-request.json`, start the configured
Go server, then run:

```sh
curl --fail-with-body http://127.0.0.1:8080/quizzes \
  -H 'Content-Type: application/json' \
  --data-binary @quiz-request.json
```

## Response

```json
{
  "model": "configured-model-name",
  "prompt_version": "implementation-quiz-v1",
  "questions": [
    {
      "id": "q1",
      "topic": "Account enumeration",
      "question": "How would the response for an unknown user compare with a wrong password?",
      "gap_indices": [0],
      "evidence": [
        {"file_path": "login.go", "start_line": 3, "end_line": 3}
      ]
    }
  ],
  "no_questions_reason": ""
}
```

Questions are free response. Gap indices are zero-based into the submitted
evaluation; evidence line ranges are one-based and inclusive in submitted file
contents. IDs are `q1`, `q2`, etc., local to this response, not persistent IDs.
The response does not contain an answer key, submitted code, or source excerpts.
The future TUI should present `question` and retain grounding metadata for review.

Coverage is driven by the system prompt: spread questions across distinct
consequential gaps and implementation behaviors as evenly as the evidence
permits, rather than distributing questions across unrelated files. Questions
must test assumptions or behavior arising from ambiguity, not syntax trivia.

If no grounded questions are possible, HTTP 200 returns `questions: []` and a
nonempty `no_questions_reason`. The future workflow should handle this explicitly
rather than forcing a quiz or treating it as a correct answer.

The server validates question count, unique topics/question text, IDs, gap
indices, file paths, and line bounds. Strict decoding rejects extra fields such
as answer keys. Those checks do not establish semantic correctness, perfectly
even coverage, or guarantee that model-written prose cannot hint at an answer.
The supplied evaluation is validated but is not fetched from MongoDB or
authenticated against an evaluation ID in this stateless version.

## Errors and execution

Errors use `{"error":{"code":"...","message":"..."}}`:

| HTTP | Meaning |
| --- | --- |
| 400 | Invalid request, clear/threshold score, missing files, or invalid cap |
| 405 / 415 | Wrong method / content type |
| 413 | HTTP body larger than 1 MiB |
| 502 | Cortex failure or invalid generated quiz |
| 503 | Queue full or unavailable; full queues include `Retry-After: 1` |
| 504 | Generation timeout |
| 500 | Other generation failure |

The route has a separate bounded in-process queue (4 workers, 100 pending),
respects request cancellation, and uses `EVALUATION_TIMEOUT`. Responses use
`Cache-Control: no-store`. Submitted code and provider response bodies are not
logged by this endpoint.

## Later TUI integration

After an ambiguous Codex turn completes, the TUI will need to gather the actual
generated files, retain the evaluation and turn association, and call this
endpoint. Preventing early disclosure also requires gating assistant text,
file-change cards, and other code-bearing output. Codex currently writes to disk;
view gating alone cannot make that code inaccessible. Workspace isolation and
review/application of changes require a separate implementation. Approval
requests must remain available throughout the workflow.

Answer grading, the two-attempt limit, code reveal, points, and persistence are
deliberately deferred until the endpoint contract is reviewed.
