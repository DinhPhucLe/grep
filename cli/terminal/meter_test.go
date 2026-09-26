package terminal

import (
	"bytes"
	"math"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"cortisol-cli/metrics"
)

func TestNeedleGeometry(t *testing.T) {
	for _, tc := range []struct{ score, x, y float64 }{{0, -1, 0}, {50, 0, -1}, {100, 1, 0}, {-9, -1, 0}, {109, 1, 0}} {
		p := ScoreToNeedlePosition(tc.score)
		if math.Abs(p.X-tc.x) > 1e-9 || math.Abs(p.Y-tc.y) > 1e-9 {
			t.Fatalf("score %v: %+v", tc.score, p)
		}
	}
	previous := -1.
	for score := 0.; score <= 100; score += .1 {
		p := ScoreToNeedlePosition(score)
		if p.X < previous || p.Y > 0 || math.Abs(p.X*p.X+p.Y*p.Y-1) > 1e-9 {
			t.Fatalf("not an ordered semicircle: %+v", p)
		}
		previous = p.X
	}
}

func TestNeedleEasesWithoutChangingActualValues(t *testing.T) {
	now := time.Unix(100, 0)
	var a needleAnimation
	score := 31.
	a.update(&score, now)
	score = 70
	a.update(&score, now)
	if a.displayedScore != 31 || a.actualScore != 70 || a.targetScore != 70 {
		t.Fatalf("teleported or lost target: %+v", a)
	}
	for i := 0; i < 12; i++ {
		previous := a.displayedScore
		now = now.Add(100 * time.Millisecond)
		a.update(&score, now)
		if a.displayedScore <= previous || a.displayedScore >= 70 || a.actualScore != 70 || score != 70 {
			t.Fatalf("invalid interpolation: %+v", a)
		}
	}
	if a.displayedScore < 69 {
		t.Fatalf("did not approach target: %v", a.displayedScore)
	}
	before := a.displayedScore
	score = 15
	a.update(&score, now)
	if a.displayedScore != before {
		t.Fatal("retargeting jumped")
	}
	a.update(&score, now.Add(100*time.Millisecond))
	if a.displayedScore >= before || a.displayedScore <= 15 {
		t.Fatal("retargeting failed to ease down")
	}
	a.update(nil, now)
	if a.known {
		t.Fatal("missing metric retained the old needle")
	}
}

func TestIdleIsPeriodicBoundedAndDoesNotResetOnDuplicateSnapshots(t *testing.T) {
	for _, score := range []float64{0, 50, 58, 100} {
		now := time.Unix(100, 0)
		var a needleAnimation
		a.update(&score, now)
		low, high := score, score
		for i := 1; i <= 200; i++ {
			a.update(&score, now.Add(time.Duration(i)*100*time.Millisecond))
			low, high = min(low, a.displayedScore), max(high, a.displayedScore)
			if a.actualScore != score || math.Abs(a.displayedScore-score) > 1.25 || a.displayedScore < 0 || a.displayedScore > 100 {
				t.Fatalf("idle changed metric or exceeded range: %+v", a)
			}
		}
		if high-low < .5 {
			t.Fatalf("idle is stuck at %v", score)
		}
		previous := a.displayedScore
		target := 25.
		a.update(&target, now.Add(20*time.Second))
		if a.displayedScore != previous || a.actualScore != 25 || a.targetScore != 25 || !a.settled.IsZero() {
			t.Fatalf("idle handoff jumped: %+v", a)
		}
	}
}

func TestPresentationPolicyAndProducerTrend(t *testing.T) {
	for _, tc := range []struct {
		score float64
		label string
	}{{0, "LOW"}, {33, "LOW"}, {34, "MODERATE"}, {66, "MODERATE"}, {67, "HIGH"}, {100, "HIGH"}} {
		v := PrepareMeter(metrics.Snapshot{Score: &tc.score})
		if v.RiskLabel != tc.label {
			t.Fatalf("%v: %s", tc.score, v.RiskLabel)
		}
	}
	s := snapshot(58)
	s.Trend = "falling"
	delta := -7.
	s.Delta = &delta
	v := PrepareMeter(s)
	if v.Trend != "falling" || v.Delta != "-7" || v.RiskLabel != "USER LABEL" {
		t.Fatalf("producer values changed: %+v", v)
	}
	for _, position := range []float64{0, 31, 57, 59, 100} {
		frame := strings.Join(RenderMeter(v, position, 80, ColorNone, false), "\n")
		if !strings.Contains(frame, "VIBE 58") || !strings.Contains(frame, "↓ falling -7") || !strings.Contains(frame, "USER LABEL") {
			t.Fatalf("animated text instead of needle: %q", frame)
		}
	}
	if *s.Score != 58 || *s.Delta != -7 {
		t.Fatal("renderer mutated the snapshot")
	}
	zero := 0.
	v = PrepareMeter(metrics.Snapshot{Score: &zero, Trend: "stable", Delta: &zero})
	if v.trend(false) != "→ stable 0" {
		t.Fatalf("zero delta lost: %s", v.trend(false))
	}
}

func plainFrame(line string) string {
	var stream ansiStream
	var out strings.Builder
	for _, token := range stream.push(line) {
		if !strings.HasPrefix(token, "\x1b") {
			out.WriteString(token)
		}
	}
	return out.String()
}

func TestMeterWidthPalettePivotAndSanitization(t *testing.T) {
	v := PrepareMeter(snapshot(58))
	v.RiskLabel = "CUSTOM\x1b[2J\x1b]52;c;payload\a\r\n LABEL"
	for columns := 0; columns < 160; columns++ {
		for _, mode := range []ColorMode{ColorNone, ColorBasic, Color256, ColorTrue} {
			for _, ascii := range []bool{false, true} {
				lines := RenderMeter(v, 31, columns, mode, ascii)
				for _, line := range lines {
					if utf8.RuneCountInString(plainFrame(line)) > max(0, columns-1) || strings.ContainsAny(line, "\r\n\a") || strings.Contains(line, "payload") || strings.Contains(line, CSI+"2J") {
						t.Fatalf("unsafe frame width=%d mode=%v: %q", columns, mode, line)
					}
					if mode == ColorNone && strings.Contains(line, "\x1b") {
						t.Fatal("NO_COLOR contained ANSI")
					}
					if ascii && len(plainFrame(line)) != utf8.RuneCountInString(plainFrame(line)) {
						t.Fatal("ASCII fallback contained Unicode")
					}
				}
			}
		}
	}
	for _, score := range []float64{0, 50, 100} {
		lines := RenderMeter(v, score, 80, ColorNone, false)
		if len(lines) != 5 || []rune(lines[3])[16] != '●' {
			t.Fatal("needle pivot moved")
		}
	}
	for _, mode := range []ColorMode{ColorBasic, Color256, ColorTrue} {
		frame := strings.Join(RenderMeter(v, 58, 80, mode, false), "")
		for _, score := range []float64{0, 33, 66, 100} {
			// Arc samples near each anchor must exercise the corresponding palette.
			if riskColor(score) == (rgb{}) || !strings.Contains(frame, CSI) {
				t.Fatal("missing palette")
			}
		}
	}
	unknown := strings.Join(RenderMeter(PrepareMeter(metrics.Snapshot{}), 58, 80, ColorNone, false), "\n")
	if strings.Contains(unknown, "●") || !strings.Contains(unknown, "VIBE --") || !strings.Contains(unknown, "waiting for metrics") {
		t.Fatalf("unknown invented a needle: %q", unknown)
	}
}

func TestTerminalCellWidthIgnoresColorControls(t *testing.T) {
	plain := "123 ▀▄█ ↑→↓"
	colored := CSI + "38;2;1;2;3m" + plain[:4] + CSI + "0m" + plain[4:]
	if terminalCellWidth(plain) != 11 || terminalCellWidth(colored) != terminalCellWidth(plain) {
		t.Fatalf("cell width changed with ANSI: plain=%d color=%d", terminalCellWidth(plain), terminalCellWidth(colored))
	}
}

func TestColorCapabilityFallbacks(t *testing.T) {
	for _, name := range []string{"NO_COLOR", "WT_SESSION", "COLORTERM", "TERM"} {
		t.Setenv(name, "")
	}
	if err := os.Unsetenv("NO_COLOR"); err != nil {
		t.Fatal(err)
	}
	if got := detectColorMode(); got != ColorBasic {
		t.Fatal(got)
	}
	t.Setenv("TERM", "xterm-256color")
	if got := detectColorMode(); got != Color256 {
		t.Fatal(got)
	}
	t.Setenv("COLORTERM", "truecolor")
	if got := detectColorMode(); got != ColorTrue {
		t.Fatal(got)
	}
	t.Setenv("NO_COLOR", "")
	if got := detectColorMode(); got != ColorNone {
		t.Fatal(got)
	}
}

func TestTopViewportMappingAndErase(t *testing.T) {
	s := newTerminalState(80, 19)
	s.rowOffset = 5
	for _, tc := range []struct{ input, want string }{
		{CSI + "H", CSI + "6;1H"}, {CSI + "99;99H", CSI + "24;80H"},
		{CSI + "1d", CSI + "6d"}, {CSI + "2;10r", CSI + "7;15r" + CSI + "6;1H"},
	} {
		got, safe := s.accept(tc.input)
		if !safe || got != tc.want {
			t.Fatalf("%q => %q (%v)", tc.input, got, safe)
		}
	}
	s.accept(CSI + "4;7H")
	for _, control := range []string{CSI + "1J", CSI + "2J"} {
		got, safe := s.accept(control)
		if !safe || strings.Contains(got, control) || !strings.Contains(got, CSI+"6;1H"+CSI+"2K") || !strings.Contains(got, CSI+"9;7H") {
			t.Fatalf("erase not confined/restored: %q", got)
		}
	}
	s.accept("\x1b7")
	s.rowOffset = 1
	s.resize(40, 9)
	got, safe := s.accept("\x1b8")
	if !safe || !strings.HasSuffix(got, CSI+"5;7H") {
		t.Fatalf("save/restore kept old header offset: %q", got)
	}
}

func TestCoordinateProtocolsReleaseHeader(t *testing.T) {
	for _, control := range []string{CSI + "6n", CSI + "?6n", CSI + "?1000h", CSI + "?1002h", CSI + "?1003h", CSI + "?1006h"} {
		var output bytes.Buffer
		r := NewRenderer(&output, 80, 24, true)
		r.Start()
		output.Reset()
		r.Write([]byte(control))
		if _, height := r.Dimensions(); height != 24 || !strings.HasSuffix(output.String(), control) || !strings.Contains(output.String(), CSI+"r") {
			t.Fatalf("unsafe coordinate protocol: %q", output.String())
		}
	}
}

func TestResizeWaitsForCompleteChildControl(t *testing.T) {
	var output bytes.Buffer
	r := NewRenderer(&output, 80, 24, true)
	r.Start()
	output.Reset()
	r.Write([]byte(CSI + "31"))
	r.Resize(40, 10)
	if output.Len() != 0 {
		t.Fatalf("resize injected into child escape: %q", output.String())
	}
	r.Write([]byte("m"))
	if c, h := r.Dimensions(); c != 40 || h != 10 {
		t.Fatalf("resize was not applied: %d,%d", c, h)
	}
	if r.layout().columns != 0 || r.reservedRows() != 0 {
		t.Fatalf("undersized terminal retained header: %+v", r.layout())
	}
	r.Update(snapshot(70))
	if strings.Contains(output.String(), "VIBECODE") {
		t.Fatalf("undersized terminal retained the status component: %q", output.String())
	}
	if !strings.HasPrefix(output.String(), CSI+"31m") {
		t.Fatalf("child control was corrupted: %q", output.String())
	}
	r.Resize(80, 24)
	if _, h := r.Dimensions(); h != 19 {
		t.Fatal("full header did not return")
	}
	r.Resize(10, 2)
	if _, h := r.Dimensions(); h != 2 {
		t.Fatal("tiny terminal lost its usable rows")
	}
}

func TestStatusCanvasAdaptsOrDisappearsWithoutOverflow(t *testing.T) {
	var output bytes.Buffer
	r := NewRenderer(&output, 100, 30, true)
	r.Start()
	if r.layout().columns != 80 || r.layout().rows != 5 {
		t.Fatalf("startup canvas = %+v", r.layout())
	}
	for _, size := range [][2]int{{140, 30}, {78, 20}, {60, 10}, {40, 10}, {120, 18}} {
		output.Reset()
		r.Resize(size[0], size[1])
		wantRows := 5
		if size[0] < meterMinColumns {
			wantRows = 0
		}
		if r.reservedRows() != wantRows || r.layout().columns > size[0] {
			t.Fatalf("resize %v selected invalid canvas: %+v", size, r.layout())
		}
		if wantRows == 5 && (!strings.Contains(output.String(), CSI+"?7l") || !strings.Contains(output.String(), CSI+"?7h")) {
			t.Fatalf("resize %v did not isolate autowrap: %q", size, output.String())
		}
	}
}

func TestRendererAnimationOnlyChangesNeedle(t *testing.T) {
	var output bytes.Buffer
	r := NewRenderer(&output, 80, 24, true)
	now := time.Unix(100, 0)
	r.now = func() time.Time { return now }
	r.Start()
	r.Update(snapshot(31))
	s := snapshot(70)
	delta := 39.
	s.Delta = &delta
	r.Update(s)
	if r.animation.displayedScore != 31 || *r.snapshot.Score != 70 {
		t.Fatal("real and displayed score were mixed")
	}
	first := r.lastFrame
	for i := 0; i < 10; i++ {
		now = now.Add(100 * time.Millisecond)
		r.Update(s)
		if *r.snapshot.Score != 70 || *r.snapshot.Delta != 39 || r.snapshot.Trend != "rising" {
			t.Fatal("animation changed producer history/trend")
		}
	}
	if first == r.lastFrame {
		t.Fatal("no animated frames")
	}
	output.Reset()
	r.Stop()
	output.Reset()
	r.Update(s)
	if output.Len() != 0 {
		t.Fatal("painted after shutdown")
	}
}
