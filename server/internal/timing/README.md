# Prompt-to-quiz latency

Normal conversation follows evaluation (null score) → Codex → normal display.
Implementation requests follow evaluation → Codex → changed-file collection →
quiz generation only when ambiguity exceeds 0.30 and quizzable code exists.
No-file and no-question results silently restore chat.

Evaluation and quiz generation each make one direct Cortex request. Answers and
section reveals are local TUI actions: no grading calls, answer queues, or batches.
The first-question path is evaluation + baseline + Codex + collection + generation
+ UI preparation. Advancing through questions makes no additional API calls.

## Capture one run

Restart the server with current code. From `server/`, in one terminal:

```sh
go run ./cmd/server 2>&1 | tee /tmp/cortisol-server.log
```

In another terminal, from `server/` (also Codex's working directory):

```sh
go run ./cmd/tui --timing-log /tmp/cortisol-tui-timing.jsonl
```

Submit an ambiguous prompt, wait for question 1, then answer it. Timing records
go to a separate file so they do not corrupt the full-screen TUI. Add `--log` to
capture raw RPC traffic when investigating the Codex handoff or approvals. Raw
RPC logs can contain code and prompts; timing events do not.

Each prompt has a `trace_id`; each HTTP request has a `request_id` to match
client/server spans. Each successful `/evaluations` or `/quizzes` request produces
one `cortex.total` event. No answer-grading events are produced. Server timings appear
even without the flag but lack TUI end-to-end correlation.

Read both logs together:

```sh
python3 internal/timing/summarize.py \
  /tmp/cortisol-tui-timing.jsonl /tmp/cortisol-server.log
```

Add `--trace <trace_id>` to isolate one prompt. Events are ordered by completion
timestamp. Durations use local elapsed time; clock synchronization only affects
ordering across machines, not durations.

## Find the slow stage

| Event | Measurement | What a large value suggests |
| --- | --- | --- |
| `quiz.first_question_ready` | Enter through first-question UI readiness | Overall wait; excludes physical terminal paint |
| `http.client` / `evaluations` | Evaluation round trip | Compare Cortex and MongoDB spans; no queue |
| `cortex.total` | One Cortex call, networking and decoding included | Model/provider/network/output latency |
| `cortex.response_headers` | Dispatch until HTTP response headers | Network plus provider work; not pure inference time |
| `cortex.response_body` | Reading completion body after headers | Transfer/provider delivery after headers |
| `mongo.insert` | Saving a numeric implementation evaluation | Unrated conversation and quizzes do not write MongoDB |
| `http.server` | Entire HTTP handler | Evaluation or quiz generation |
| `workspace.baseline` | Pre-Codex enumeration and text hashing | Large tracked/untracked trees |
| `codex.turn` | Turn dispatch through completion/failure/disconnect | Tools, tests, model work, and approvals |
| `codex.user_decision_wait` | Request arrival until user response submitted | Approval/input waiting, included in Codex turn |
| `workspace.collect` | Post-turn enumeration, hashing, changed-file reads | Second workspace scan; includes file/byte counts |
| `quiz.skipped_no_files` | Successful collection found no quiz-eligible files | Expected for clarification-only turns; chat resumes without a quiz API call |
| `http.client` / `quizzes` | Complete quiz API round trip | One whole-quiz Cortex completion and validation |
| `ui.question_render` | Build and refresh question viewport | Large display buffers; not terminal paint |
| `quiz.prepare_to_ready` | Post-Codex collection through question readiness | The wait after coding finishes |

Outcomes distinguish success, error, cancellation, and timeout. Generation errors
end the quiz and return to chat. Answers are never sent to a grading service.
Successful responses schedule no new generation. Six questions require
six local answer/reveal advances, not six further question API calls.

Spans overlap. Do **not** sum `http.client + http.server + cortex.total`,
Use `http.server` for the entire quiz request and `http.client` for its round trip.
For evaluation, `http.server - cortex.total - mongo.insert` estimates remaining
handler/validation overhead. Provider telemetry is needed to distinguish internal
Snowflake queueing from inference. These tests do not measure live model latency.

If the last TUI event is `workspace.baseline` with no `codex.turn`, inspect the RPC
log for dispatch, approval/input requests and completion before changing generation settings.
The quiz cannot start until a successful Codex turn supplies generated code.
