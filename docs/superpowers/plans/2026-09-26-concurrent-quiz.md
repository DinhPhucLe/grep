# Direct evaluation and concurrent quiz generation

User requirements: evaluation bypasses queues; quiz question generation runs six
independent jobs using three workers; successful API responses terminate generation
and render the quiz without request loops.

Design: POST /evaluations invokes Evaluate directly with its existing timeout and
persistence. POST /quizzes first plans up to six grounded, disjoint code scopes,
then submits one question job per scope to a shared queue (three workers, six waiting
slots). Jobs carry explicit scopes, with no preceding-question dependency. Results
are collected in completion order, assigned display IDs, and validated together.
Planning requires one separate Cortex call. Fewer than six scopes remain valid when
the supplied code cannot support six grounded questions. No automatic regeneration
or retries. Cancellation or one failed job cancels the other jobs for that request.

The TUI requests one complete quiz and removes the progressive prefetch state
machine. Questions are displayed after the batch completes. Legacy progressive
service helpers remain covered by their existing tests, but server routes and the
TUI move to the batch path. Model selection stays unchanged.

Steps:
1. Test direct evaluation dispatch and retain timeout/error/persistence behavior.
2. Test six jobs reach three concurrent workers, out-of-order completion, finite
   success, zero/fewer questions, invalid outputs, cancellation, and shared limits.
3. Implement planner plus independent question jobs and wire server routes.
4. Test and simplify TUI generation/grading/reveal to eliminate prefetch chaining.
5. Update API and latency documentation; run targeted tests, server suite, race
   checks, and an independent code review.

Work in the existing workspace because the feature being changed is already
uncommitted here. Preserve unrelated modifications; do not commit or reset them.

Completed:
- Evaluation service is called directly; test admits six concurrent requests with
  deadlines and no worker admission queue.
- Shared three-worker/six-pending-slot question generator implemented. Tests cover
  six independent jobs, shared limits, finite success, malformed plans/questions,
  duplicate question rejection, fewer/zero scopes, cancellation, failure, deadline
  and shutdown.
- TUI uses a single batch response; sequential prefetch removed. Tests cover
  rendering, masking, grading, duplicate notifications/results, six-question
  termination, explicit retry, empty quizzes and late replies after reveal.
- Full server `go test ./...` passed. Race checks passed for evaluation, quiz,
  jobs and TUI packages. `git diff --check` passed. Independent reviewer reported
  no important findings. Validation used mock Cortex responses, not live inference.
