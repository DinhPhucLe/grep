# Codex TUI UI Implementation Plan

**Goal:** Make the existing Go client feel like a clear, responsive coding conversation, with a person icon for the user, live agent responses, readable formatting, and visible approvals and tool activity.

**Architecture:** Keep the app-server boundary described in the supplied Markdown. The TUI owns presentation and interaction; the app-server owns agent execution. Route incoming RPC events into one UI state model and render that model without blocking the RPC reader.

**Tech stack:** Go; proposed Bubble Tea, Bubbles, Lip Gloss, and Glamour dependencies. Inspect the existing go.mod and select compatible versions before implementation.

**Design basis:** The Markdown and UI requirements pasted in this conversation on September 26, 2026. No repository source was supplied; file names below are proposed responsibilities to map onto the actual repo.

**Status:** Implemented on `codex-tui-ui`. Automated Go tests and Windows PTY multi-turn/log/view smoke checks pass. Unix terminal and race-detector runtime checks remain pending because this environment has no Unix runtime or C compiler.

## 1. Design direction

Recommended: a full-window conversation with a fixed composer. A compact header shows workspace and session state; the middle scrolls; the bottom contains the input and keyboard hints. This gives streaming messages, approvals, and multiline prompts predictable places on screen.

Alternatives: an inline transcript is a smaller change and preserves normal terminal scrollback, but provides less control over a persistent input area. A split-pane dashboard exposes more activity, but consumes too much width for a first version. Start with the conversation layout and expandable activity rows.

Use the terminal's background, readable foreground text, muted metadata, cyan accents for the user, and a restrained violet accent for the agent. Amber marks pending approvals; red plus a written label marks failures. Color must never be the only indication of state.

## 2. Screen layout and message styling

| Area | Contents and behavior |
|---|---|
| Header | App name, workspace basename, and connection state. Hide optional metadata at narrow widths. |
| Conversation | User messages, agent responses, and compact tool cards in order. |
| Status | Waiting for response, responding, running a command, approval required, interrupted, or failed, derived from actual events. |
| Composer | Person icon, visible `You` label, editable prompt, and a subtle border. Grows from 1 to 6 visible input lines before scrolling internally. |
| Footer | Context-sensitive shortcuts for sending, newline, scrolling, and quitting. |

User messages: `👤 You`, a small gap, then the original prompt. Preserve its whitespace and display it as plain text. Agent messages: `✦ Codex`, followed by rendered Markdown. Keep both left-aligned so code has enough horizontal space. Use spacing and a subtle border or accent to distinguish messages instead of enclosing every answer in a heavy box.

Offer an icon-free mode using `You` and `Codex`. Do not require a patched font. Measure visible terminal cells when wrapping; an emoji or Vietnamese character is not reliably one byte or one screen cell.

## 3. Text formatting

- Render Markdown headings, bold, italic, lists, blockquotes, inline code, fenced code blocks, and links.
- Use bold accent text for headings; terminals do not provide browser-style heading font sizes.
- Syntax-highlight completed code blocks. Preserve their source for any later copy feature.
- Underline links and focused controls through the style layer. Markdown has no standard underline delimiter; do not reinterpret underscores as underline or promise arbitrary HTML rendering.
- Treat italic and underline as best-effort terminal styles; text must remain understandable when a terminal or font does not display them.
- Wrap prose to available width. For the MVP, allow long code lines to wrap visually while preserving the exact source. Avoid adding visible line numbers that get mixed into copied code.
- Keep table content readable at narrow widths, initially by allowing a plain-text fallback rather than forcing a wide table off-screen.

Retain raw message text separately from rendered output. Escape or remove embedded terminal control sequences from displayed external content before applying the client's own formatting.

## 4. Streaming and animation

Before text arrives, show a small spinner with `Waiting for response`. As agent text arrives, append it to the current message and render updates at most every 33 milliseconds, a proposed 30 FPS ceiling. This is a rendering limit, not a delay on reading or storing RPC events.

Use a blinking cursor at the end of the active answer. Show tool-specific status only when the corresponding server event arrives; do not invent progress percentages or hidden reasoning.

For the first version, incoming text creates the typing effect naturally. Do not reveal one character on a timer after the server has already finished. If an optional typewriter mode is added later, cap its backlog and immediately reveal everything on completion, interruption, error, or an approval request.

Streaming Markdown is often incomplete. Maintain the full raw buffer, style stable blocks, and display the unfinished tail conservatively. Re-render the full message on item completion; reconcile with the final payload rather than appending it a second time. Do not treat every blank line as a guaranteed Markdown boundary, particularly inside lists and fenced code.

Auto-scroll only while the user is already at the bottom. If they scroll upward, preserve their reading position and show `New output below`. Returning to the bottom re-enables following. Cache completed message rendering and invalidate it on width or theme changes.

Provide a reduced-motion setting with a static status indicator and no cursor animation. Incoming text must remain visible in this mode.

## 5. Tools, approvals, and input

Tool cards begin collapsed and display the actual action plus its status, for example `Running command: go test ./...`. Expand a focused card with Enter to inspect output or a file diff. Show exit status and failures explicitly. A file-change proposal must not be labeled applied until the server confirms it.

Approvals take input focus in a dedicated panel. Show the requested command or file operation and the choices supported by that request's schema. Preserve the partially written prompt. Keep receiving RPC traffic while the panel is open. Respond once using the original request ID and the exact response shape for that request type. Do not assume every server-initiated request is an approval; route input prompts to an appropriate input panel as well.

Proposed key bindings:

| Key | Action |
|---|---|
| Enter | Send from the composer while idle; activate the focused control elsewhere. |
| Alt+Enter | Insert a newline; advertise this dependable binding. |
| Shift+Enter | Optional newline alias when the terminal distinguishes it. |
| Tab / Shift+Tab | Move between composer and interactive conversation controls. |
| Page Up / Page Down | Scroll conversation history. |
| End while history is focused | Return to latest output. |
| Esc | Request interruption of the active turn using the installed protocol; wait for server confirmation. |
| Ctrl+C | Copy selected conversation text. |
| Ctrl+V / Insert | Paste clipboard text into the input without submitting. |
| /quit | Exit and restore the terminal, including during approvals. |

Allow drafting the next prompt during a turn but disable sending until the turn ends in the MVP. Label this state clearly. Treat pasted multiline text as one paste; embedded newlines must not submit a prompt automatically. Keep approval choices isolated from composer shortcuts.

## 6. Internal responsibilities

| Proposed file or existing equivalent | Responsibility |
|---|---|
| `tui/model.go` | UI state: connection, current thread/turn, items, pending requests, draft, focus, viewport, terminal dimensions. |
| `tui/update.go` | Reduce RPC, keyboard, resize, and timer events into state changes. |
| `tui/view.go` | Compose header, transcript, status, composer, and footer. |
| `tui/theme.go` | Colors, borders, labels, spacing, and icon/motion options. |
| `tui/markdown.go` | Render raw messages, handle incomplete Markdown, and cache finished rendering. |
| `tui/approval.go` | Pending request presentation, focus, and exactly-once response tracking. |
| Existing transport and logger | Process lifecycle, JSON-RPC I/O, protocol request IDs, and raw logging. |

Use one owner for UI state. Background readers emit events rather than mutating the view directly. Serialize writes to the server. Key items by thread, turn, and item IDs rather than assuming all deltas belong to the most recent message. Keep transport ingestion independent of animation timers.

Keep raw RPC logging at the transport boundary so a redraw never creates a second log entry. Preserve `--log`, `--view`, Eastern timestamps, and the existing file naming described in the supplied Markdown. The pretty-print viewer should continue to work without starting the interactive screen or spawning an agent.

Generate schemas from the installed app-server before implementation. Verify exact notification names, final turn status fields, interruption methods, and each server request's response shape against those schemas. The names in the supplied document are architectural guidance, not a substitute for version-matched protocol types.

## 7. Build order and acceptance checks

### Phase 1: Conversation shell

- [x] Inspect the actual repo, go.mod, terminal entry point, transport, and logging code; map proposed files onto existing structure.
- [x] Pin compatible UI dependencies and introduce a single event-driven UI model around the existing transport.
- [x] Build the header, transcript, composer, role labels/icons, focus rules, and resizing behavior.
- [x] Preserve existing command-line flags and clean exit behavior.

Acceptance: a user can send two successive turns, see correctly associated messages, resize the terminal, and exit with the shell restored. Check layouts at 80x24 and 120x35, plus a compact fallback below those dimensions.

### Phase 2: Streaming and formatted answers

- [x] Add raw message buffers, Markdown rendering, the waiting indicator, cursor animation, and the 30 FPS render ceiling.
- [x] Add conservative rendering for unfinished content, final-message reconciliation, completed-message caching, and reduced motion.
- [x] Add follow-output scrolling that pauses when the user reads history.

Acceptance: split a bold marker and a fenced code block across delta events; the final message must be correct and appear once. Interleave two items without mixing their text. Test Vietnamese, emoji, long code lines, and a 100 KB response without losing input responsiveness.

### Phase 3: Activity and approvals

- [x] Add expandable command/file cards with event-derived status.
- [x] Add approval and server-input panels while preserving the user's draft.
- [x] Add interruption, error, and disconnection states without claiming success on a failed turn.

Acceptance: an approval arriving during streaming is visible immediately, does not block the RPC reader, and produces one correctly correlated reply. A completed text item does not prematurely mark the whole turn complete. Disconnection leaves the transcript readable and prevents sending into a dead connection.

### Phase 4: Compatibility and regression checks

- [ ] Verify multiline paste, newline bindings, narrow screens, icon fallback, no-color rendering, and reduced motion in Windows Terminal and at least one Unix terminal.
- [x] Confirm each RPC line is still logged exactly once and `--view` retains its behavior.
- [ ] Add focused state/stream/approval regression tests; run `go test ./...` and `go test -race ./...` where supported.
- [x] Smoke-test `go run ./tui`, `go run ./tui --log`, and `go run ./tui --view` against the installed app-server.

Definition of done: the requested person icon, live response effect, and readable formatting work through a real multi-turn session; approvals and logging still behave correctly; resizing and exiting do not corrupt the terminal.

## 8. Scope and review focus

First release includes the conversation layout, icons with fallback, formatted streaming, multiline input, scrolling, activity cards, approvals, and existing logs. Defer sidebars, session search, custom avatars, elaborate animations, dashboards, and theme editors until that core works reliably.

During review, prioritize incomplete Markdown, Unicode cell widths, approval arrival during streaming, scroll position during redraws, and exactly-once handling of final messages and approval replies. The phase checks above cover these failure modes.

## Reference libraries

- Bubble Tea: https://github.com/charmbracelet/bubbletea — Go TUI state and rendering framework.
- Bubbles: https://github.com/charmbracelet/bubbles — reusable input, viewport, and spinner components.
- Lip Gloss: https://github.com/charmbracelet/lipgloss — terminal layout and styling.
- Glamour: https://github.com/charmbracelet/glamour — terminal Markdown rendering.

Use their version-matched documentation during implementation; do not mix examples from different major versions.
