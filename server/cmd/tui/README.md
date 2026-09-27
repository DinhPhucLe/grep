# Codex app-server client (TUI)

## Architecture

**Codex app-server** is the main process. It runs the Codex AI agent, owns the session, and performs coding actions (edits, shell, tools). Clients talk to it over JSON-RPC (stdio in our case).

**Codex CLI** is OpenAI’s native client of that same app-server. It is not the agent itself — it is a UI that speaks the app-server protocol.

**This repo’s TUI** (`go run ./tui`) is our own client of the same app-server. We spawn `codex app-server --listen stdio://`, speak the same protocol, and keep agentics on the server side.

```text
  you  →  our TUI (render + I/O)  ↔  JSON-RPC/stdio  ↔  codex app-server (agent + tools + MCP)
              ↑
         same role as Codex CLI
```

**Setup guide:** [`docs/codex-mcp.md`](../../../docs/codex-mcp.md)

To connect Codex to our MCP: open `~/.codex/config.toml` and paste:

```toml
[mcp_servers.cortisol]
url = "http://127.0.0.1:3100/mcp"
startup_timeout_sec = 20
tool_timeout_sec = 60
enabled = true
```

MCP must be running (`cd mcp && npm run dev`). Then restart this TUI. Smoke
test: ask the agent to call `quack`.

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

On Windows, install Codex with `npm.cmd install -g @openai/codex`.
The TUI launches npm installations through `node.exe`, so PowerShell script
execution policy does not affect it. It also checks `%APPDATA%\npm` when an
existing terminal has not picked up npm's PATH entry. Node.js must be on PATH.
Native Codex executables on PATH are used directly.

Run these commands from `server/cmd` (or use `go run ./cmd/tui` from `server`):

```bash
go run ./tui
go run ./tui --log                              # write tui/log/{EasternTime}_{threadId}.jsonl
go run ./tui --view                             # pretty-print newest log
go run ./tui --view tui/log/some-session.jsonl  # pretty-print a specific log
```

Requires `codex` on `PATH`. The full-window conversation keeps a fixed composer, streams answers, formats Markdown, and shows expandable file-change summaries. Raw command activity is hidden from the conversation; transport logs still retain it. Enter sends while idle; you can draft during a turn. Alt+Enter inserts a newline. Bracketed multiline pastes remain one prompt.

Scroll the conversation with the mouse wheel or Page Up / Page Down, including while an answer is streaming. Ctrl+Home jumps to the first message and Ctrl+End returns to the latest output from any focus. Scrolling up pauses following; new text displays `New output below`. Scrolling back to the bottom resumes following. Earlier messages stay in the conversation.

Tab / Shift+Tab move among the composer, history, and activity cards. Up / Down scroll one line when history or a card is focused; they continue to move the cursor when composing a prompt. Enter expands a focused card; End returns to the latest output. Esc clears a text selection first, or requests interruption when no selection is active. Type `/quit` (or `/exit`) and Enter to restore the terminal and exit, including while an approval is open. Ctrl+C copies text and does not quit.

Click and drag over visible conversation text to select it, then press Ctrl+C to copy. Selection includes complete Unicode characters and omits terminal styling and display padding. Selected text stays stable while responses continue arriving; Esc, scrolling, resizing, or a new approval clears the selection. Clipboard success or errors appear in the footer.

Press Ctrl+V or Insert to paste clipboard text into the composer or an editable server-input answer. Terminal-provided paste, including Shift+Insert where supported, is also accepted. Multiline composer pastes preserve line breaks and never submit automatically; press Enter to send. Clipboard access uses the operating system clipboard (Unix installations may require the clipboard tools supported by `atotto/clipboard`).

Resizing reflows the transcript at word boundaries, for both streaming and completed answers. Words stay together unless a single token is wider than the available text area. Original message text and explicit newlines are retained.

Short terminals keep a scrollable conversation and a compact input instead of hiding the transcript. The composer grows only within the available height. Decorative borders and the header disappear when space is tight, and approvals show fewer choices at once while preserving the selected action. Press F1 for the complete, scrollable shortcut list; use arrows, Page Up / Page Down, or the wheel to read it and F1 / Esc to return. Extremely small windows prioritize the active input or approval; message history remains available when space permits.

Approval panels explain the requested action in plain English, using the server's file-read, file-listing, and search descriptions when supplied. Unrecognized commands show the actual command without guessing what it does. Choices use labels such as `Allow once`, `Don't allow`, `Allow for this conversation`, and `Stop this task`. Saved command/network rules are labeled separately. Selected choices use a contrasting highlight, and shortcut keys are emphasized. Long approval paths and choices have a horizontal scrollbar: use Left/Right, a horizontal mouse wheel, or click/drag the bar. Alt+Left/Right pans while entering text without taking over cursor editing. Ctrl+D toggles the full technical request; Ctrl+N / Ctrl+P scroll the explanation or details. When the server specifies available decisions, only those exact decisions are offered. A non-granting choice is selected first when available.

Approval and input panels preserve the draft and keep receiving events. Tab / arrow keys select a choice; Enter confirms it. User-input questions support choices and free text; secret questions mask input. MCP elicitation accepts JSON form input or decline/cancel. Unsupported server requests receive a JSON-RPC method-not-found error rather than a fabricated approval response.

Use `--no-icons` for plain role labels, `--reduced-motion` for static indicators, and `--no-color` (or `NO_COLOR=1`) to disable text styling. Streaming and completed answers render Markdown at every width, including bold, italic, headings, code, and links. Headings use terminal styling rather than a larger font size. Rendered buffers are cached until their text or width changes. Raw content is retained separately, and external terminal controls are removed before display.

With `--log`, each inbound and outbound RPC occurrence is appended once, including repeated identical deltas. Handshake events are buffered until the thread ID is known. File names retain Eastern time and thread IDs; `--view` prints Eastern timestamps, direction, a summary, and indented JSON without spawning an agent or entering the interactive screen.

The protocol was checked against schemas generated by `codex-cli 0.155.0-alpha.16.3`. UI state belongs to the Bubble Tea model; the RPC reader feeds a nonblocking queue, writes are serialized, and event batches are bounded to keep keyboard input responsive. Rendering is limited to 30 FPS, and completed Markdown is cached by width.

Run `go test ./...` and `go vet ./...` from `server`. `go test -race ./...` additionally requires cgo and a supported C compiler. Automated checks cover reconciliation, interleaved items, approvals, server-input shapes, disconnects, multiline paste, Unicode widths, compact layouts, follow scrolling, repeated log events, and large responses. Live Windows PTY checks covered two successive turns, `--log`, `--view`, and clean exit. Unix terminal and race-detector runtime checks remain pending on an environment that supports them.
