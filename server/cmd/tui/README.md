# Codex app-server client (TUI)

Run command codex mcp add openaiDeveloperDocs --url https://developers.openai.com/mcp to add the codex docs mcp for your agent

## Architecture

**Codex app-server** is the main process. It runs the Codex AI agent, owns the session, and performs coding actions (edits, shell, tools). Clients talk to it over JSON-RPC (stdio in our case).

**Codex CLI** is OpenAI’s native client of that same app-server. It is not the agent itself — it is a UI that speaks the app-server protocol.

**This repo’s TUI** (`go run ./tui`) is our own client of the same app-server. We spawn `codex app-server --listen stdio://`, speak the same protocol, and keep agentics on the server side.

```text
  you  →  our TUI (render + I/O)  ↔  JSON-RPC/stdio  ↔  codex app-server (agent + tools)
              ↑
         same role as Codex CLI
```

### Client responsibility

Like Codex CLI, our client is responsible for:

- Rendering chat history
- Sending user requests into the agent
- Streaming / showing agent responses back to the user
- Collecting user input when the server asks (approvals, prompts)

It does **not** implement the agent loop or apply patches itself — app-server does that after the client answers approvals.

## Lifecycle

1. **Start** — client spawns app-server and opens stdio.
2. **Handshake** — `initialize` → server result → `initialized` notification.
3. **Thread** — `thread/start` creates a session (cwd, sandbox, approval policy). Returns a `threadId`.
4. **Turn** — each user message is `turn/start` on that thread with text input.
5. **Items** — during a turn the server emits item notifications (deltas, completions) and may send **server requests** (e.g. `item/*/requestApproval`) that the client must answer.
6. **Turn end** — `turn/completed` (or failed). Client returns to the prompt for the next turn.

## Core concepts

| Concept | Meaning |
|--------|---------|
| **Thread** | One persistent agent session (workspace, model, policy). Many turns share one thread. |
| **Turn** | One user → agent round: starts with user input, ends when the agent finishes that round. |
| **Item** | A unit of work inside a turn: user message, agent message, reasoning, file change, command execution, etc. Items stream via notifications (`item/*/delta`, `item/completed`). |

Approvals are server-initiated JSON-RPC **requests** (not notifications). Reply to the same `id` with `{ "decision": "accept" | "decline" | ... }`.

## Developing against the protocol

Generate TypeScript API schemas at the **project root**:

```bash
codex app-server generate-ts --out ./schemas
```

Use those schemas as the source of truth for methods, params, and item shapes when extending the Go client. The `schemas/` directory is gitignored — regenerate locally as needed.

### Run the test TUI

```bash
go run ./tui
go run ./tui --log                              # write tui/log/{EasternTime}_{threadId}.jsonl
go run ./tui --view                             # pretty-print newest log
go run ./tui --view tui/log/some-session.jsonl  # pretty-print a specific log
```

Requires `codex` on `PATH`. Type a message and press Enter; `/quit` or Ctrl+C to exit. When the agent needs approval, answer `y` / `n`. With `--log`, every inbound and outbound RPC line is appended once (handshake buffered until the thread id exists, then flushed into the named file). `--view` prints each event with Eastern timestamps, direction, a one-line RPC summary, and indented JSON.
