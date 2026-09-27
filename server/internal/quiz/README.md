# Question generation API

`POST /quizzes` generates an implementation-grounded quiz through the existing
Snowflake Cortex client. It uses the server's `SNOWFLAKE_MODEL` (configure a
supported Claude model to use Claude), account URL, PAT, and evaluation timeout.
This is a new route on our Go server; no new Snowflake-side endpoint is needed.

The TUI calls this endpoint after a successful ambiguous Codex turn, and uses
`POST /quiz-answers` for grading. The server does not persist quizzes or award
points. HTTP 200 returns a complete validated quiz.

## Concurrent generation

1. One Cortex call plans up to six independent, non-overlapping code scopes.
2. One question job per scope is submitted to a shared queue with **three workers
   and six pending slots**. Six scopes produce six question jobs, usually two
   waves of three calls, in addition to the planning call.
3. Jobs contain an explicit scope and the full plan; no preceding question text
   is required. Results are collected in completion order and assigned display
   IDs `q1` through `q6`, keeping the evidence attached to its question.
4. The assembled quiz is validated and returned in one JSON response. The TUI
   shows question 1 after the batch completes. No continuation or prefetch calls
   are made. `/quizzes/start` and `/quizzes/next` are no longer registered routes;
   restart both server and TUI when upgrading.

Fewer questions are allowed when the code supports fewer distinct decisions. An
empty valid plan returns an empty quiz with a reason and makes no question calls.
The three question workers are shared across requests, not created per quiz.
Planning runs before the question queue so it cannot deadlock waiting for its own
child jobs. Overload returns 503; it does not spin or retry. Any failed job cancels
its siblings. The single request deadline covers planning, queueing, and all
question calls. Each job sends exactly one terminal result to a buffered channel;
no successful response schedules additional generation.

The TUI holds output until the complete batch arrives. This trades the previous
first-question streaming behavior for concurrent generation of the whole quiz.
Tests verify concurrency and termination, not live latency or model answer quality.

## Batch request

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
  "prompt_version": "implementation-quiz-concurrent-v1",
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
The TUI presents one `question` at a time and uses grounding metadata to focus
and progressively reveal the referenced code.

Coverage is driven by the system prompt: spread questions across distinct
consequential gaps and implementation behaviors as evenly as the evidence
permits, rather than distributing questions across unrelated files. Questions
must test assumptions or behavior arising from ambiguity, not syntax trivia.

If no grounded questions are possible, HTTP 200 returns `questions: []` and a
nonempty `no_questions_reason`. The TUI explicitly ends the quiz and opens code review without treating it as
a correct answer.

The server validates question count, unique topics/question text, IDs, gap
indices, file paths, line bounds, and non-overlapping evidence across questions. Strict decoding rejects extra fields such
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

The question queue has three workers and six pending slots. The full batch
respects request cancellation and uses `EVALUATION_TIMEOUT`. Responses use
`Cache-Control: no-store`. Submitted code and provider response bodies are not
logged by this endpoint.

## Answer grading

`POST /quiz-answers` accepts JSON with `quiz` (the original `/quizzes` request),
`questions` (the generated object containing `questions` and `no_questions_reason`),
`question_id` (for example `q1`), and `answer` (1–8000 bytes of nonblank text).
It validates the complete quiz and selected question, then asks Cortex whether
the answer captures the consequential behavior/assumption. Successful responses
are exactly `{"correct":true}` or `{"correct":false}`. No answer key, hints, code,
or explanatory feedback is returned, avoiding premature disclosure of other answers.

The TUI owns the two-attempt limit and progression. Provider failures consume no
attempts. Grading uses the supplied immutable file snapshot and has its own bounded
queue with the same timeout. This is stateless: submissions are not authenticated
against saved quiz IDs and it is not an anti-cheating or durable scoring system.

## TUI visibility

The TUI holds assistant messages and raw change cards from ambiguous turns,
then shows file snapshots with unanswered evidence ranges masked. Nonquiz code
remains visible. Correct answers or two failed attempts reveal a section before
the user advances. Original output is released after completion or explicit
`/reveal`. Generation errors offer `/retry` and `/reveal`; zero grounded questions
open review without awarding credit. See the [TUI guide](../../cmd/tui/README.md).

This does not isolate files on disk or redact optional RPC logs. Codex approval
and input requests remain available. Semantic leakage through model-written
questions or code outside selected ranges cannot be ruled out by line-bound
validation. Points and persistence are deferred.
