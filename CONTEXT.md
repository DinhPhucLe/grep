# Cortisol CLI — Current product direction

## Purpose

Cortisol is a TUI client of Codex app-server. It helps a client compare their intended product behavior with the behavior Codex implemented when a build request was ambiguous. Codex owns coding actions; the TUI owns evaluation, review questions, and presentation.

## Evaluation and normal conversation

Classify the latest message before rating ambiguity. Only explicit requests to build, implement, add, change, fix, or remove code/product behavior receive a numeric ambiguity score. Use prior conversation to resolve requirements, not to turn every follow-up into another implementation request.

Confirmations, approvals, option selections, answers to clarification questions, greetings, explanations, status checks, and inspection/test-only requests have `verdict: not_applicable`, `ambiguity_score: null`, and `gaps: []`. They go to Codex unchanged with no evaluation card, workspace baseline, or quiz. A confirmation that authorizes an existing plan is still normal conversation. A message that adds a concrete new implementation request is evaluated.

For implementation requests, a good prompt specifies what should happen, the relevant implementation approach and constraints, and applicable examples or references to reusable code. Existing conversation can supply these details; narrow edits do not need artificial checklist items. Scores at or below 0.30 are clear; scores above 0.30 require consequential behavior, compatibility, or integration gaps. Higher scores mean greater ambiguity. The evaluator only sees supplied text and cannot verify repository references. Unrelated internal engineering choices are not grounds for quizzing the client.

## Implementation review

After an ambiguous implementation request, Codex implements the original request. If its completed turn produces no changed text files, return silently to normal chat. If there are files, Snowflake generates up to four distinct, grounded review questions, prioritizing consequential behavior. One is sufficient; never pad to four. If nothing is client-facing and quizzable, return no questions and silently open review.

Questions address a client in plain language about observable outcomes. Never ask the client to design underlying architecture, choose queues/databases/frameworks, optimize API calls, or decide whether quizzes or grading should run individually or in batches. Those are implementation responsibilities, not quiz material. The quiz generator must not ask for implementation permission or confirmation.

The TUI displays one question at a time in a muted-yellow ASCII dashed box (plain borders when color is disabled). Each question opens its first source reference at the starting line in the existing VS Code editor; Ctrl+O cycles references. Quiz panels show references, not source dumps. Individual answers are saved to MongoDB with the question and source snapshot before Enter advances. A failed save keeps the draft for an idempotent retry. Answers are not sent to Snowflake or Codex, graded, or scored. There are no grading endpoints, answer queues, or batch grading. Future grading, if explicitly requested, will grade questions individually; that policy is settled and must not be asked of the client. Codex scoring and embeddings remain proposals; see docs/quiz-answer-storage-plan.md. Persistence uses existing users and projects and requires migration 8 to be explicitly approved and applied. Without a selected user/project, quizzes remain local and label answers as unsaved.

Code and implementation output remain visible throughout; nothing is masked or restored. VS Code opens the live file while question references come from the completed-turn snapshot. Editor launch failures leave the quiz usable with an explanatory message. Normal Codex approval and input requests remain available. Model output is validated for real file/line/gap references. Later overlapping questions are dropped before rendering to reduce repetitive coverage. This cannot prove semantic correctness or comprehension.

## Scope

The application has no login, registration, caller tokens, auth middleware, or account sessions. Snowflake, MongoDB, and Codex retain the credentials required to access those external services. Do not reintroduce application authentication or testing/latency experiments as product features.

The experimental multi-file attachment commands and bundled Codex inputs are removed. Send the user's prompt unchanged. Evaluation and quiz generation remain separate, direct API operations; there is no query-batching endpoint, scope-planning worker pool, progressive generation, or answer submission queue. The quiz generator uses the completed implementation's file snapshot as evidence.

Numeric and null-score evaluations are persisted to MongoDB in `evaluations`, with a database ID returned only after a successful insert. Null scores remain null and still skip quizzes; migration 7 supplies the required nullable validator and requires explicit approval before application. Quiz answers reference existing `users._id` and `projects._id` as ObjectIDs; the server checks user existence and project ownership. User fields remain unchanged. The TUI remembers IDs selected with `--user-id` and `--project-id` per workspace in `cortisol/users.json`. No identity is generated, no profile is created, and evaluation does not depend on a registration call. Generated unanswered quizzes are held temporarily by the server; saved answer records embed the authoritative question and source snapshot. Points, correctness feedback, rewards, persistent quiz sessions, and answer grading are out of current scope. The old passive dashboard/metrics direction is also outside this flow.

The intervention is inspired by work on engagement with AI-generated code; neither model scores nor answers establish that a user understands an implementation.

## Database operations

Get explicit approval before applying or reverting a database migration. Code changes, local checks, and read-only inspection may proceed within the requested task.
