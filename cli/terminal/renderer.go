// Package terminal renders an externally supplied metric snapshot alongside a
// wrapped command. It owns a compact header and never computes metric values.
package terminal

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"cortisol-cli/metrics"
	"github.com/charmbracelet/x/term"
)

// Renderer is an io.Writer for child terminal output. Call Update periodically
// to coalesce status paints; complete child output is forwarded immediately.
// Methods are safe to call concurrently, including shutdown and resize.
type Renderer struct {
	mu                          sync.Mutex
	output                      io.Writer
	columns, rows               int
	screenActive                bool
	enabled                     bool
	color                       ColorMode
	ascii                       bool
	started                     bool
	unsafe                      bool
	dirty                       bool
	stream                      ansiStream
	terminal                    *terminalState
	snapshot                    metrics.Snapshot
	err                         error
	animation                   needleAnimation
	posture                     postureAnimation
	postureFrame                postureView
	warnings                    bool
	now                         func() time.Time
	lastFrame                   string
	pendingColumns, pendingRows int
	readSize                    func() (int, int, error)
}

func NewRenderer(output io.Writer, columns, rows int, enabled bool) *Renderer {
	columns, rows = max(1, columns), max(1, rows)
	r := &Renderer{output: output, columns: columns, rows: rows, enabled: enabled, color: detectColorMode(), ascii: os.Getenv("VIBECODE_ASCII") == "1", dirty: true, now: time.Now, warnings: os.Getenv("VIBECODE_WARNINGS") != "0"}
	c, h := r.dimensions()
	r.terminal = newTerminalState(c, h)
	r.terminal.rowOffset = r.reservedRows()
	if file, ok := output.(*os.File); ok {
		r.readSize = func() (int, int, error) { return term.GetSize(file.Fd()) }
	}
	return r
}

func (r *Renderer) reservedRows() int {
	return r.layout().rows
}

func (r *Renderer) layout() statusLayout { return layoutFor(r.columns, r.rows, r.enabled && !r.unsafe) }

func (r *Renderer) dimensions() (int, int) { return r.columns, r.rows - r.reservedRows() }

// Dimensions returns the viewport to assign to the child's PTY. Unsupported
// child controls release the status row, so check again after Write.
func (r *Renderer) Dimensions() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dimensions()
}

func (r *Renderer) emit(value string) error {
	if r.err != nil || value == "" {
		return r.err
	}
	n, err := io.WriteString(r.output, value)
	if err == nil && n != len(value) {
		err = io.ErrShortWrite
	}
	r.err = err
	return err
}

func (r *Renderer) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return r.err
	}
	r.started = true
	if !r.enabled || r.unsafe {
		return r.err
	}
	// The main scrollback buffer reflows old pixel rows before Resize runs.
	// Own an alternate screen for the entire status session, even when the
	// header is temporarily hidden. Nested child screen switches are virtual.
	r.screenActive, r.terminal.ownedScreen = true, true
	if err := r.emit(CSI + "?1049h" + CSI + "0m" + CSI + "2J" + r.terminal.margins() + r.terminal.position()); err != nil {
		return err
	}
	return r.paint()
}

func (r *Renderer) Write(data []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.started || !r.enabled || r.unsafe {
		if err := r.emit(string(data)); err != nil {
			return 0, err
		}
		return len(data), nil
	}
	if err := r.refreshSize(); err != nil {
		return 0, err
	}
	tokens := r.stream.push(string(data))
	if r.stream.overflowed {
		if err := r.disable(); err != nil {
			return 0, err
		}
	}
	for _, token := range tokens {
		forwarded := token
		if !r.unsafe {
			var safe bool
			forwarded, safe = r.terminal.accept(token)
			if !safe {
				if err := r.disable(); err != nil {
					return 0, err
				}
				forwarded = token
			}
		}
		if err := r.emit(forwarded); err != nil {
			return 0, err
		}
	}
	if r.unsafe {
		if err := r.emit(r.stream.flush()); err != nil {
			return 0, err
		}
	}
	r.dirty = true
	if err := r.applyPendingResize(); err != nil {
		return 0, err
	}
	return len(data), r.err
}

// Update accepts display values from the caller. A nil score stays unknown;
// trend and delta remain producer-owned. Repeated calls advance only the needle.
func (r *Renderer) Update(snapshot metrics.Snapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if snapshot.Validate() != nil {
		snapshot = metrics.Snapshot{RiskLabel: "Invalid metrics"}
	}
	r.snapshot = snapshot.Clone()
	now := r.now()
	r.animation.update(r.snapshot.Score, now)
	r.postureFrame = r.posture.update(PrepareMeter(r.snapshot), now)
	if err := r.applyPendingResize(); err != nil {
		return err
	}
	return r.paint()
}

func (r *Renderer) Resize(columns, rows int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if columns == r.columns && rows == r.rows && r.pendingColumns == 0 {
		return r.paint()
	}
	r.pendingColumns, r.pendingRows = max(1, columns), max(1, rows)
	if err := r.applyPendingResize(); err != nil {
		return err
	}
	return r.paint()
}

func (r *Renderer) applyPendingResize() error {
	// A host resize must never insert escapes inside a buffered child control
	// or synchronized frame. The runner picks up new dimensions after Write.
	if r.pendingColumns == 0 || !r.stream.complete() || r.terminal.synchronized {
		return r.err
	}
	r.columns, r.rows = r.pendingColumns, r.pendingRows
	r.pendingColumns, r.pendingRows = 0, 0
	c, h := r.dimensions()
	r.terminal.resize(c, h)
	r.terminal.rowOffset = r.reservedRows()
	if r.started && r.enabled && !r.unsafe {
		// Rebuild the entire owned viewport: hiding the header changes the
		// child's physical offset, and height changes may discard old cells.
		frame := CSI + "?2026h" + CSI + "?25l" + CSI + "?7l" + CSI + "0m" + CSI + "2J" + r.terminal.screen.paint(r.reservedRows()) + r.terminal.margins() + r.restoreCursor() + CSI + "?2026l"
		if err := r.emit(frame); err != nil {
			return err
		}
	}
	r.dirty = true
	return r.err
}

func (r *Renderer) clearRow(row int) string {
	if row <= 0 || row > r.rows {
		return ""
	}
	return fmt.Sprintf("%s%d;1H%s2K", CSI, row, CSI)
}

func (r *Renderer) disable() error {
	r.unsafe = true
	r.terminal.ownedScreen = false
	r.terminal.rowOffset = 0
	c, h := r.dimensions()
	r.terminal.resize(c, h)
	leave := ""
	if r.screenActive {
		leave = CSI + "?1049l"
		r.screenActive = false
	}
	return r.emit(CSI + "?2026l" + leave + CSI + "0m" + CSI + "r" + CSI + "?7l" + r.terminal.screen.paint(0) + r.restoreCursor())
}

func (r *Renderer) paint() error {
	if r.started && r.enabled && !r.unsafe {
		if err := r.refreshSize(); err != nil {
			return err
		}
	}
	// CUP destroys pending wrap. Also never insert a frame inside a child
	// control, UTF-8 character, or synchronized terminal frame.
	if !r.started || r.reservedRows() == 0 || !r.stream.complete() || r.terminal.wrap || r.terminal.synchronized {
		return r.err
	}
	view := PrepareMeter(r.snapshot)
	lines := renderMeter(view, r.animation.displayedScore, r.layout().columns, r.color, r.ascii, r.postureFrame, r.warnings)
	content := strings.Join(lines, "\n")
	if !r.dirty && content == r.lastFrame {
		return r.err
	}
	// Keep horizontal resize from exposing a partly cleared, partly redrawn
	// five-row character.
	frame := CSI + "?2026h" + CSI + "?25l" + CSI + "?7l" + CSI + "0m"
	for i, line := range lines {
		frame += r.clearRow(i+1) + line
	}
	// Explicit restoration keeps the child's ESC 7 / ESC 8 save slot intact.
	frame += r.restoreCursor()
	frame += CSI + "?2026l"
	if err := r.emit(frame); err != nil {
		return err
	}
	r.dirty = false
	r.lastFrame = content
	return nil
}

// Metric updates can arrive between the runner's 100 ms resize polls. Probe
// before painting so those updates do not emit an old-width frame after shrink.
func (r *Renderer) refreshSize() error {
	if r.readSize != nil {
		w, h, err := r.readSize()
		if err == nil && w > 0 && h > 0 && (w != r.columns || h != r.rows) {
			r.pendingColumns, r.pendingRows = w, h
		}
	}
	return r.applyPendingResize()
}

func (r *Renderer) restoreCursor() string {
	value := CSI + "0m" + r.terminal.style
	if r.terminal.autoWrap {
		value += CSI + "?7h"
	} else {
		value += CSI + "?7l"
	}
	value += r.terminal.position()
	if r.terminal.visible {
		value += CSI + "?25h"
	} else {
		value += CSI + "?25l"
	}
	return value
}

func (r *Renderer) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.started {
		return r.err
	}
	if tail := r.stream.flush(); tail != "" {
		// Preserve the tail and cancel its unfinished control before cleanup.
		r.emit(tail + "\x18")
	}
	if r.enabled && !r.unsafe {
		if r.terminal.alternate {
			sequence, _ := r.terminal.accept(CSI + "?1049l")
			r.emit(sequence)
		}
		r.emit(CSI + "?2026l" + CSI + "?2004l" + CSI + "?1003l" + CSI + "?1004l" + CSI + "?1006l" + CSI + "?1l" + CSI + "?7h" + CSI + "0m" + CSI + "r" + CSI + "?1049l" + CSI + "?25h")
		r.screenActive = false
	}
	r.started = false
	return r.err
}
