# Cortisol Go CLI

The CLI launches Codex with a compact pixel speedometer near the top of its terminal area. It is a
separate Go module alongside `server/` and `dashboard/`. It does not calculate
metrics, score events, read agent transcripts, or call a metrics server. Your
function supplies the complete display snapshot.

The port preserves the previous CLI's Codex launcher, PTY/ConPTY, argument
passthrough, resizes, plain redirected I/O, and status preview. Legacy temporary
scoring and telemetry/debug flags are intentionally outside this implementation.

## Build and run

Requires Go 1.27+ and Codex on PATH. Windows interactive mode requires ConPTY
(Windows 10 version 1809 or later). Unix terminals use a PTY.

```sh
cd cli
go mod download
go build -o bin/cortisol ./cmd/cortisol
go run ./cmd/cortisol --demo
go run ./cmd/cortisol --metrics examples/snapshot.json
go run ./cmd/cortisol --metrics status.json -- resume
go run ./cmd/cortisol -- --help
go run ./cmd/cortisol --no-gauge -- --help
```

On Windows, build with `go build -o bin/cortisol.exe ./cmd/cortisol` and run
`.\bin\cortisol.exe`. Use the built executable when the exact child exit code
matters: `go run` itself converts a nonzero program exit into a Go tool failure.
You can name the built executable `vibecode` to keep the old launcher name.

Everything after `--` goes to Codex unchanged, including its own flags and prompts.
Windows npm shims are resolved to their Node entrypoint without a command shell.
`--no-gauge`, redirected input/output, and a dumb terminal use direct child I/O.
No status text is inserted into redirected Codex output.

`--demo` uses explicitly marked fixtures. In an interactive terminal it cycles
until `q` or Ctrl+C; when redirected it prints four sample lines and exits.
`--render PATH` validates and prints one supplied snapshot without starting Codex.
`--render -` reads that snapshot from stdin, for example:

```sh
go run ./cmd/cortisol --render examples/snapshot.json
```

## Check the CLI-to-server connection

`check-server` sends `POST /jobs` with your input and prints the server response.
It runs independently of Codex and does not connect live metrics yet.

Start the existing Go server in one PowerShell window (from the repository root):

```powershell
cd server
& ..\.cache\toolchain\go\bin\go.exe run ./cmd/server
```

The `.cache` toolchain path is the project-local Go installation. If you have Go
on PATH instead, use `go run ./cmd/server`.

In another PowerShell window, from the repository root, run this single line:

```powershell
.\cli\bin\cortisol.exe check-server --server http://localhost:8080 --input "connection-test-001"
```

Expected output with the current demo server:

```text
POST http://localhost:8080/jobs
HTTP 200 OK
Output: "CONNECTION-TEST-001"
Connection OK
```

The server also logs the completed result. Change `--input` to verify a second
round trip. Stop the server and rerun to verify a connection error and nonzero
`$LASTEXITCODE`. Requests time out after five seconds; override with `--timeout 2s`.
Use `check-server --help` for all options. Exit codes are 0 for success, 1 for a
connection/server/response failure, and 2 for invalid command arguments.

## Supply values from another process

Pass `--metrics status.json`. The producer writes a complete JSON object:

```json
{"score":75,"riskLabel":"TEST","trend":"rising","delta":10}
```

The CLI polls every 250 ms. Write a temporary file in the same directory and
atomically replace the target to avoid partial reads. The file may be created
after Codex starts. It is read-only from the CLI's perspective. Relative paths
are relative to the launch directory.

The presentation contract is `metrics.Snapshot`:

| Field | Meaning |
| --- | --- |
| `score` | Producer-supplied number from 0 to 100, or `null` for unknown. This range only controls meter presentation. |
| `riskLabel` | Optional display text. When omitted, the presentation adapter supplies LOW (0-33), MODERATE (34-66), or HIGH (67-100). |
| `trend` | `rising`, `falling`, `stable`, `unknown`, or empty. |
| `delta` | Optional producer-supplied finite number; never calculated from the prior score. |

All fields may be omitted; `{}` is an unknown snapshot. Unknown never becomes
zero. Missing/unreadable files display a waiting state; invalid snapshots clear
the score until valid input arrives. There is no time-based expiry for a valid
file; the producer should publish `score: null` when its measurement is stale.
`--render` reports detailed validation errors. Files are limited to 64 KiB and
must contain exactly one object with the documented fields.

The default risk words are temporary display labels, not metric calculations.
`terminal.PrepareMeter` applies this policy outside the pixel renderer; a supplied
`riskLabel` takes precedence. Trend and delta must describe actual historical
scores from your producer. Missing trend stays unknown; the CLI does not infer
one from the needle. Control characters in labels cannot inject terminal commands.
Snapshot files remain separate from the browser's `dashboard.v1` payload.

## Call from your Go function

The public `cortisol-cli` package accepts a channel of calculated snapshots. Your
producer owns the channel and its lifecycle. Set `Gauge: true` explicitly.

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
updates := make(chan metrics.Snapshot, 1)

// Run your metric function separately and send its complete result to updates.
// Honor ctx.Done() when sending so the producer stops when the CLI exits.
go func() {
    defer close(updates)
    snapshot := calculateDisplaySnapshot() // your function, returns metrics.Snapshot
    select {
    case updates <- snapshot.Clone():
    case <-ctx.Done():
    }
}()

code, err := cli.Run(ctx, cli.Options{
    Args:    []string{"resume"},
    Gauge:   true,
    Updates: updates,
})
cancel()
// Handle err and return code after your own cleanup.
```

Import `cli "cortisol-cli"` and `"cortisol-cli/metrics"`. Within another local
module, add `require cortisol-cli v0.0.0` and `replace cortisol-cli => ../cli`.
No server module changes are required. Sent snapshots must be immutable; use
`Clone()` before sending if your function reuses numeric storage. A closed channel
leaves the last supplied snapshot on screen. Nil standard streams default to the
process's stdin/stdout/stderr. The renderer is also available directly from
`cortisol-cli/terminal`.

## Terminal behavior and verification

The meter reserves the top five rows of an owned alternate screen. A single
layout policy controls both reservation and drawing:

| Current terminal size | Display |
| --- | --- |
| At least 78 columns and 7 rows | Meter, score text and animated pixel cat |
| 54-77 columns and at least 7 rows | Meter and text; cat completely hidden |
| Below 54 columns or 7 rows | Entire header hidden; child gets all rows |

The canvas uses at most 80 columns and always fits the current width. Components
return when space is restored. The CLI cannot lock the terminal window's resize
controls. Codex receives the real remaining viewport dimensions.

The renderer tracks supported child cursor, color, scroll and erase operations,
retaining the visible child cells for a complete repaint after resize. Child
alternate-screen switches are handled inside that viewport. This prevents a
nested child from switching away from the status screen. Painting waits for
complete control sequences and synchronized child frames; it does not borrow
the child's cursor-save slot. It checks physical dimensions before drawing so
metric updates between resize polls cannot paint an old-width frame.

The arc uses green, yellow, orange and red, with a neutral needle from a fixed
center. `ScoreToNeedlePosition` owns score-to-angle geometry. Actual score text
updates immediately; the needle eases toward it on the existing 100 ms display
refresh. After settling, it sways periodically by at most 1.25 points (clamped at
0 and 100). Animation has separate actual, displayed and target values. It never
writes snapshots, calculates trends, or changes producer history. Identical pixel
frames are not repainted unless child output requires a refresh.

Truecolor is selected for Windows Terminal or a terminal advertising truecolor;
otherwise the meter uses ANSI 256 colors when advertised, then basic ANSI colors.
`NO_COLOR` disables meter colors. `VIBECODE_ASCII=1` replaces block glyphs and
arrows with ASCII. Score, risk words, trend text and scale ticks also work without
color. `--render` remains a plain, single-line summary for redirected output.

The original shell screen returns on exit. The alternate screen has no native
terminal scrollback; the retained child viewport contains visible cells, not a
transcript. Use `--no-gauge` for the native terminal/scrollback experience.
Unsupported controls, ambiguous character widths,
cursor-position requests, or mouse-coordinate reporting release the header for
the rest of that session so Codex can continue with the full terminal. This draft
therefore does not guarantee a persistent meter in every Codex/terminal version.
`--no-gauge` remains available for a direct terminal session.

### Manually preview the draft (no Go on PATH required)

From the repository root, use the rebuilt Windows executable:

```powershell
.\cli\bin\cortisol.exe --demo
```

Use an 80-column window with at least 7 rows. The demo cycles through supplied
LOW, MODERATE and HIGH fixtures every ten seconds so each cat motion cycle is
visible. Watch the needle ease across
the arc, then sway slightly while the numeric score and trend stay unchanged.
Resize through 80, 77, 54 and 40 columns, then restore it. Press `q` or Ctrl+C to exit and
confirm the cursor and normal typing are restored. The demo does not start Codex,
contact the server or calculate metrics.

For colorless or ASCII previews:

```powershell
$env:NO_COLOR = '1'
.\cli\bin\cortisol.exe --demo
Remove-Item Env:NO_COLOR
$env:VIBECODE_ASCII = '1'
.\cli\bin\cortisol.exe --demo
Remove-Item Env:VIBECODE_ASCII
```

For a producer-driven session, use the existing `--metrics PATH` workflow above.
Supply 31, then 70 with `trend: "rising"` and `delta: 39`: text should immediately
show 70 while the needle travels to it. Leave the file alone to observe idle sway.
Send an unknown score (`null`) to remove the needle. Check normal typing, an
approval prompt, output and resizing in your own Codex session; if it uses a
terminal protocol outside this draft's supported set, the header will disappear
and the session will continue without it.

```sh
go test ./...
go vet ./...
go build ./cmd/cortisol
```

Tests cover geometry, motion timing, real-value isolation, rendering widths,
child screen edits/scrolling and nested screen restoration. The Windows resize
regression test reads actual console cells after repeated width/height changes,
compares every header row and rejects stray pixel glyphs in the child viewport.
It also verifies that existing child text survives. Merely finding status words
somewhere in an output stream is not a visual resize test.

### Pixel posture companion

The cat is original 16x10 pixel terminal art adapted from the supplied motion
blueprint, rendered with half-block glyphs into five terminal rows. Fur, eyes and
the colored outline are separate pixels. No image protocol or external asset
loader is required. Colorless and ASCII fallbacks remain available.

| Actual score band | Motion cycle |
| --- | --- |
| LOW / green | Blink, head tilt, slow paw wave, gentle tail wag, relaxed pose (8 seconds) |
| MODERATE / yellow | Wide eyes, ear twitch, body tremor, tail flick, annoyed glance (6 seconds) |
| HIGH / red | Narrow eyes, ears back, bounded shake, tail swish, angry lunge (4 seconds) |

Posture follows the actual Vibe score using the temporary display bands, even
when a producer supplies a custom risk label. A new band must persist for one
second before the posture changes; the numeric score and risk text still update
immediately. Every motion stays within the same pixel canvas, with pauses and
one-pixel offsets instead of layout shifts. Neither animation nor needle sway
changes scores, trends, history or posture selection. Unknown or invalid metrics
immediately restore a still neutral cat and `AWAITING METRICS`.

The last header row shows `CHILL`, `PAY ATTENTION`, or `EMERGENCY PAUSE`.
These are Vibe status cues, not measurements of cortisol or a medical assessment.
The high-state reminder uses the real score, never the animated needle. It has
no sound, popup or input interception. Set `$env:VIBECODE_WARNINGS = '0'` before
launch to replace the reminder with `HIGH ALERT`.

### Resize root cause

The old implementation painted into the normal scrollback buffer. On an 80-to-40
column resize, Windows reflowed its five pixel rows into additional rows before
the CLI received the size change. Clearing only the top five rows left old art
in rows 6-10. ANSI cell counting and no-wrap painting did not remove those old
cells. A real console-cell test reproduced this failure before the fix.

The owned alternate screen avoids normal-buffer reflow, and the retained child
viewport allows a complete redraw when the header's physical offset changes.
See Microsoft's [console screen buffer documentation](https://learn.microsoft.com/en-us/windows/console/console-virtual-terminal-sequences#alternate-screen-buffer).
