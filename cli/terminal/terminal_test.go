package terminal

import (
	"bytes"
	"errors"
	"io"
	"math"
	"strings"
	"testing"

	"cortisol-cli/metrics"
)

func snapshot(score float64) metrics.Snapshot {
	return metrics.Snapshot{Score: &score, RiskLabel: "USER LABEL", Trend: "rising"}
}

func TestStatusSuppliedValuesAndUnknown(t *testing.T) {
	line := RenderStatusLine(metrics.Snapshot{}, 80, false)
	if !strings.Contains(line, "-- waiting for metrics") || strings.Contains(line, "LOW") {
		t.Fatalf("unknown snapshot invented a value: %q", line)
	}
	line = RenderStatusLine(metrics.Snapshot{RiskLabel: "Invalid metrics"}, 80, false)
	if !strings.Contains(line, "-- Invalid metrics") {
		t.Fatalf("missing producer status: %q", line)
	}
	line = RenderStatusLine(snapshot(72), 80, false)
	if !strings.Contains(line, "72  USER LABEL  ^") {
		t.Fatalf("supplied values missing: %q", line)
	}
	state := snapshot(0)
	state.RiskLabel, state.Trend = "", "unknown"
	line = RenderStatusLine(state, 80, false)
	if !strings.Contains(line, "  0") || strings.ContainsAny(line, "^>v") {
		t.Fatalf("zero or unknown trend misrepresented: %q", line)
	}
	for _, invalid := range []float64{math.NaN(), math.Inf(1), -1, 101} {
		if got := RenderStatusLine(snapshot(invalid), 80, false); !strings.Contains(got, "--") {
			t.Fatalf("invalid value %v rendered as a known score: %q", invalid, got)
		}
	}
}

func TestStatusWidthAndSanitization(t *testing.T) {
	state := snapshot(100)
	state.RiskLabel = "CUSTOM\x1b[2J\r\n\x1b]52;c;payload\a LABEL 界🙂"
	state.Delta = new(float64)
	*state.Delta = 12.3
	for columns := 0; columns < 160; columns++ {
		line := RenderStatusLine(state, columns, false)
		if len(line) > max(0, columns-1) || strings.ContainsAny(line, "\x1b\r\n\a") || strings.Contains(line, "payload") {
			t.Fatalf("unsafe line at width %d: %q", columns, line)
		}
	}
	if got := RenderStatusLine(state, 100, false); !strings.Contains(got, "CUSTOM LABEL ??") || !strings.Contains(got, "+12.3") {
		t.Fatalf("unexpected sanitized line: %q", got)
	}
	if got := RenderStatusLine(snapshot(100), 3, false); got != "" {
		t.Fatalf("score must not be truncated: %q", got)
	}
}

func TestANSIStreamPreservesArbitraryChunkBoundaries(t *testing.T) {
	transcript := "a界é\x1b[38;2;1;2;3mtext\x1b[0m\r\n\x1b]0;title\x1b\\\x1bPabc\x1b\\\x1b[?1049h\x1b["
	for split := 0; split <= len(transcript); split++ {
		var stream ansiStream
		tokens := stream.push(transcript[:split])
		tokens = append(tokens, stream.push(transcript[split:])...)
		tokens = append(tokens, stream.flush())
		if got := strings.Join(tokens, ""); got != transcript {
			t.Fatalf("split %d corrupted bytes: %q", split, got)
		}
	}
	var stream ansiStream
	var tokens []string
	for i := range len(transcript) {
		tokens = append(tokens, stream.push(transcript[i:i+1])...)
	}
	if got := strings.Join(tokens, "") + stream.flush(); got != transcript {
		t.Fatalf("byte-at-a-time corrupted bytes: %q", got)
	}
}

func TestANSIOverflowIsBoundedAndLossless(t *testing.T) {
	var stream ansiStream
	input := "\x1b]" + strings.Repeat("x", maxPending+5)
	if got := strings.Join(stream.push(input), ""); got != input || !stream.overflowed {
		t.Fatalf("overflow lost bytes or failed to release: length %d", len(got))
	}
	if got := strings.Join(stream.push("more"), ""); got != "more" {
		t.Fatalf("overflow passthrough: %q", got)
	}
}

func TestRendererDefersPaintForIncompleteControlsAndWrap(t *testing.T) {
	var output bytes.Buffer
	r := NewRenderer(&output, 54, 8, true)
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if _, err := r.Write([]byte("\x1b[31")); err != nil {
		t.Fatal(err)
	}
	r.Update(snapshot(23))
	if output.Len() != 0 {
		t.Fatalf("paint entered incomplete control: %q", output.String())
	}
	r.Write([]byte("mhello"))
	if got := output.String(); got != "\x1b[31mhello" {
		t.Fatalf("child output not immediately forwarded: %q", got)
	}
	r.Update(snapshot(23))
	if !strings.Contains(output.String(), CSI+"31m"+CSI+"?7h"+CSI+"6;6H") {
		t.Fatalf("child attributes/cursor not restored: %q", output.String())
	}
	output.Reset()
	r.Write([]byte(strings.Repeat("x", 49)))
	r.Update(snapshot(24))
	if output.String() != strings.Repeat("x", 49) {
		t.Fatalf("paint discarded pending wrap: %q", output.String())
	}
	r.Write([]byte("\r\n"))
	r.Update(snapshot(24))
	if !strings.Contains(output.String(), "VIBE") {
		t.Fatalf("paint did not resume after wrap resolved: %q", output.String())
	}
}

func TestRendererDefersSynchronizedFrames(t *testing.T) {
	var output bytes.Buffer
	r := NewRenderer(&output, 80, 24, true)
	r.Start()
	output.Reset()
	r.Write([]byte(CSI + "?2026h" + "child frame"))
	r.Update(snapshot(70))
	if strings.Contains(output.String(), "VIBE") {
		t.Fatal("paint interrupted synchronized child frame")
	}
	r.Write([]byte(CSI + "?2026l"))
	r.Update(snapshot(70))
	if !strings.Contains(output.String(), "VIBE") {
		t.Fatal("paint did not resume after synchronized frame")
	}
}

func TestUnsupportedControlReleasesRowBeforePassthrough(t *testing.T) {
	for _, unsupported := range []string{CSI + "?6h", "\x1b]8;;https://example.com\x1b\\", "🙂", "\x1bPpayload\x1b\\"} {
		t.Run(unsupported, func(t *testing.T) {
			var output bytes.Buffer
			r := NewRenderer(&output, 80, 24, true)
			r.Start()
			output.Reset()
			r.Write([]byte(unsupported))
			_, rows := r.Dimensions()
			if rows != 24 || !strings.HasSuffix(output.String(), unsupported) {
				t.Fatalf("unsupported bytes or dimensions changed: %d %q", rows, output.String())
			}
			reset := strings.Index(output.String(), CSI+"r")
			if reset < 0 || reset >= strings.Index(output.String(), unsupported) {
				t.Fatalf("row was not released first: %q", output.String())
			}
			output.Reset()
			r.Update(snapshot(80))
			r.Write([]byte("plain\x1b[5~"))
			r.Stop()
			if got := output.String(); got != "plain\x1b[5~" {
				t.Fatalf("fallback must remain passthrough: %q", got)
			}
		})
	}
}

func TestResizeClampsViewportAndSwallowsConPTYAcknowledgements(t *testing.T) {
	var output bytes.Buffer
	r := NewRenderer(&output, 80, 24, true)
	r.Start()
	r.Resize(40, 10)
	if c, h := r.Dimensions(); c != 40 || h != 10 {
		t.Fatalf("wrong child dimensions %dx%d", c, h)
	}
	output.Reset()
	r.Write([]byte(CSI + "8;19;80t" + CSI + "8;10;40t"))
	if output.Len() != 0 {
		t.Fatalf("ConPTY acknowledgments resized host: %q", output.String())
	}
	r.Write([]byte(CSI + "99;99H"))
	if output.String() != CSI+"10;40H" {
		t.Fatalf("cursor escaped reserved viewport: %q", output.String())
	}
}

func TestCursorSaveSlotAndUnicode(t *testing.T) {
	s := newTerminalState(80, 23)
	for _, token := range []string{"a界e\u0301", "\x1b7", "abc", "\x1b8"} {
		if _, safe := s.accept(token); !safe {
			t.Fatalf("unexpected unsupported token: %q", token)
		}
	}
	if s.x != 5 || s.y != 1 {
		t.Fatalf("Unicode/save cursor was %d,%d", s.x, s.y)
	}
	s.accept(CSI + "?1049h")
	s.accept(CSI + "H")
	s.accept("alternate")
	s.accept(CSI + "?1049l")
	if s.x != 5 || s.y != 1 || s.saved.x != 5 {
		t.Fatalf("alternate screen lost normal cursor: %+v", s.cursor)
	}
}

func TestRendererPassthroughAndCleanup(t *testing.T) {
	var output bytes.Buffer
	r := NewRenderer(&output, 80, 24, false)
	r.Start()
	r.Update(snapshot(100))
	r.Write([]byte("raw\x1b[unfinished"))
	r.Stop()
	if got := output.String(); got != "raw\x1b[unfinished" {
		t.Fatalf("disabled renderer decorated pipe: %q", got)
	}
	output.Reset()
	r = NewRenderer(&output, 80, 24, true)
	r.Start()
	r.Write([]byte(CSI + "?1049h" + CSI))
	output.Reset()
	r.Stop()
	if got := output.String(); !strings.HasPrefix(got, CSI+"\x18") || !strings.Contains(got, CSI+"?1049l") || !strings.HasSuffix(got, CSI+"?25h") || !strings.Contains(got, CSI+"r") {
		t.Fatalf("terminal cleanup incomplete: %q", got)
	}
	output.Reset()
	r.Stop()
	if output.Len() != 0 {
		t.Fatal("stop must be idempotent")
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) / 2, nil }

func TestRendererPropagatesWriteFailure(t *testing.T) {
	r := NewRenderer(shortWriter{}, 80, 24, true)
	if err := r.Start(); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write error = %v", err)
	}
	if _, err := r.Write([]byte("child")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("child write error = %v", err)
	}
}

func TestInvalidDirectUpdateClearsStaleScore(t *testing.T) {
	var output bytes.Buffer
	r := NewRenderer(&output, 80, 24, true)
	r.Start()
	r.Update(snapshot(72))
	output.Reset()
	r.Update(snapshot(math.NaN()))
	if got := output.String(); !strings.Contains(got, "VIBE --") || !strings.Contains(got, "Invalid metrics") || strings.Contains(got, "72") {
		t.Fatalf("invalid update retained a score: %q", got)
	}
	state := snapshot(72)
	delta := math.MaxFloat64
	state.Delta = &delta
	if got := RenderStatusLine(state, 80, false); strings.Contains(got, "Inf") || strings.Contains(got, "NaN") {
		t.Fatalf("finite delta overflowed in formatting: %q", got)
	}
}
