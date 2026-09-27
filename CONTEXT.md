# Cortisol CLI — Current product direction

## Purpose

Cortisol is a TUI client of Codex app-server. It helps a client compare their intended product behavior with the behavior Codex implemented when a build request was ambiguous. Codex owns coding actions; the TUI owns evaluation, review questions, and presentation.

## Evaluation and normal conversation

Classify the latest message before rating ambiguity. Only explicit requests to build, implement, add, change, fix, or remove code/product behavior receive a numeric ambiguity score. Use prior conversation to resolve requirements, not to turn every follow-up into another implementation request.

Confirmations, approvals, option selections, answers to clarification questions, greetings, explanations, status checks, and inspection/test-only requests have `verdict: not_applicable`, `ambiguity_score: null`, and `gaps: []`. They go to Codex unchanged with no evaluation card, workspace baseline, or quiz. A confirmation that authorizes an existing plan is still normal conversation. A message that adds a concrete new implementation request is evaluated.

For implementation requests, scores at or below 0.30 are clear; scores above 0.30 require consequential gaps about client-visible behavior. Internal engineering decisions are not grounds for quizzing the client.

## Implementation review

After an ambiguous implementation request, Codex implements the original request. If its completed turn produces no changed text files, return silently to normal chat. If there are files, Snowflake generates up to four distinct, grounded review questions, prioritizing consequential behavior. One is sufficient; never pad to four. If nothing is client-facing and quizzable, return no questions and silently open review.

Questions address a client in plain language about observable outcomes. Never ask the client to design underlying architecture, choose queues/databases/frameworks, optimize API calls, or decide whether quizzes or grading should run individually or in batches. Those are implementation responsibilities, not quiz material. The quiz generator must not ask for implementation permission or confirmation.

The TUI displays one question at a time. Each question opens its first source reference at the starting line in the existing VS Code editor; Ctrl+O cycles references. Quiz panels show references, not source dumps. An answer is recorded locally and waits for Enter to advance. Answers are not sent to Snowflake or Codex, stored on the server, graded, or scored. There are no grading endpoints, retry attempts, answer queues, or batch grading. Future grading, if explicitly requested, will grade questions individually; that policy is settled and must not be asked of the client.

Code and implementation output remain visible throughout; nothing is masked or restored. VS Code opens the live file while question references come from the completed-turn snapshot. Editor launch failures leave the quiz usable with an explanatory message. Normal Codex approval and input requests remain available. Model output is validated for real file/line/gap references. Later overlapping questions are dropped before rendering to reduce repetitive coverage. This cannot prove semantic correctness or comprehension.

## Scope

The application has no login, registration, caller tokens, auth middleware, or account sessions. Snowflake, MongoDB, and Codex retain the credentials required to access those external services. Do not reintroduce application authentication or testing/latency experiments as product features.

The experimental multi-file attachment commands and bundled Codex inputs are removed. Send the user's prompt unchanged. Evaluation and quiz generation remain separate, direct API operations; there is no query-batching endpoint, scope-planning worker pool, progressive generation, or answer submission queue. The quiz generator uses the completed implementation's file snapshot as evidence.

Only numeric implementation evaluations are persisted to MongoDB. Null-score conversational results return directly without storage or a database ID. Quizzes and local answers are transient. Points, correctness feedback, rewards, persistent quiz sessions, and answer grading are out of current scope. The old passive dashboard/metrics direction is also outside this flow.

The intervention is inspired by work on engagement with AI-generated code; neither model scores nor answers establish that a user understands an implementation.

## Database operations

Get explicit approval before applying or reverting a database migration. Code changes, local checks, and read-only inspection may proceed within the requested task.
