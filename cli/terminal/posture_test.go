package terminal

import (
	"bytes"
	"cortisol-cli/metrics"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestPostureDebouncesActualBandsAndClearsUnknown(t *testing.T) {
	now := time.Unix(100, 0)
	var a postureAnimation
	update := func(score float64, after time.Duration) postureView {
		return a.update(PrepareMeter(snapshot(score)), now.Add(after))
	}
	if update(33, 0).state != postureRelaxed {
		t.Fatal("initial pose")
	}
	if update(34, 100*time.Millisecond).state != postureRelaxed {
		t.Fatal("boundary flickered")
	}
	update(33, 500*time.Millisecond)
	update(34, 600*time.Millisecond)
	if update(35, 1500*time.Millisecond).state != postureRelaxed {
		t.Fatal("candidate was not reset")
	}
	if update(66, 1600*time.Millisecond).state != postureUpright {
		t.Fatal("continuous band did not settle")
	}
	if update(67, 1700*time.Millisecond).state != postureUpright {
		t.Fatal("high pose jumped")
	}
	if update(100, 2700*time.Millisecond).state != postureTense {
		t.Fatal("high pose missing")
	}
	if a.update(PrepareMeter(metrics.Snapshot{}), now.Add(3*time.Second)).state != postureUnknown {
		t.Fatal("unknown kept high pose")
	}
	if update(0, 3100*time.Millisecond).state != postureRelaxed {
		t.Fatal("known score after unknown delayed")
	}
}

func TestBlinkAndPostureAreIndependentOfNeedle(t *testing.T) {
	now := time.Unix(100, 0)
	var out bytes.Buffer
	r := NewRenderer(&out, 80, 24, true)
	r.now = func() time.Time { return now }
	r.Start()
	defer r.Stop()
	s := snapshot(33)
	r.Update(s)
	for i := 1; i <= 60; i++ {
		now = now.Add(100 * time.Millisecond)
		r.Update(s)
		if r.postureFrame.state != postureRelaxed || *r.snapshot.Score != 33 || r.snapshot.Trend != "rising" {
			t.Fatal("idle changed posture or producer data")
		}
	}
	var a postureAnimation
	v := PrepareMeter(snapshot(58))
	a.update(v, now)
	if a.update(v, now.Add(3700*time.Millisecond)).blink {
		t.Fatal("blink early")
	}
	closed := a.update(v, now.Add(3800*time.Millisecond))
	if !closed.blink || closed.state != postureUpright {
		t.Fatal("blink missing or changed posture")
	}
	if a.update(v, now.Add(4*time.Second)).blink {
		t.Fatal("blink did not end")
	}
	if strings.Join(renderPosture(closed, ColorNone, false), "") == strings.Join(renderPosture(postureView{state: postureUpright}, ColorNone, false), "") {
		t.Fatal("blink not visible")
	}
}

func TestPostureShapesLayoutAndWarnings(t *testing.T) {
	shapes := map[string]bool{}
	for state := postureUnknown; state <= postureTense; state++ {
		frame := strings.Join(renderPosture(postureView{state: state}, ColorNone, false), "")
		if shapes[frame] {
			t.Fatal("postures indistinguishable without color")
		}
		shapes[frame] = true
		if state != postureUnknown {
			seen := map[string]bool{frame: true}
			for i := 0; i < 80; i++ {
				seen[strings.Join(renderPosture(postureView{state: state, motion: uint8(i)}, ColorNone, false), "")] = true
			}
			if len(seen) < 5 {
				t.Fatalf("cat %d has too few distinct motion frames: %d", state, len(seen))
			}
		}
	}
	for _, score := range []float64{0, 33, 34, 66, 67, 100} {
		v := PrepareMeter(snapshot(score))
		for _, columns := range []int{54, 77, 78, 80, 120} {
			for _, ascii := range []bool{false, true} {
				lines := RenderMeter(v, 50, columns, ColorNone, ascii)
				if len(lines) != 5 {
					t.Fatal("posture added rows")
				}
				for _, line := range lines {
					if utf8.RuneCountInString(line) > columns-1 {
						t.Fatalf("overflow: %q", line)
					}
				}
				if columns >= postureMinColumns {
					sprite := renderPosture(postureView{state: postureFor(v)}, ColorNone, ascii)
					for i := range lines {
						if !strings.HasSuffix(lines[i], sprite[i]) {
							t.Fatal("sprite missing")
						}
					}
				}
			}
		}
	}
	v := PrepareMeter(snapshot(80))
	if !strings.Contains(strings.Join(RenderMeter(v, 0, 80, ColorNone, false), "\n"), "EMERGENCY PAUSE") {
		t.Fatal("actual high score should warn despite low needle")
	}
	if strings.Contains(strings.Join(renderMeter(v, 80, 80, ColorNone, false, postureView{state: postureTense}, false), "\n"), "EMERGENCY PAUSE") {
		t.Fatal("disabled warning shown")
	}
	v = PrepareMeter(snapshot(20))
	if strings.Contains(strings.Join(RenderMeter(v, 100, 80, ColorNone, false), "\n"), "EMERGENCY PAUSE") {
		t.Fatal("needle triggered warning")
	}
	v = PrepareMeter(metrics.Snapshot{})
	if !strings.Contains(strings.Join(RenderMeter(v, 80, 80, ColorNone, false), "\n"), "AWAITING METRICS") {
		t.Fatal("unknown missing neutral status")
	}
}

func TestPostureDisappearsWhenPhysicalAreaIsTooSmall(t *testing.T) {
	v := PrepareMeter(snapshot(20))
	pose := postureView{state: postureRelaxed}
	visible := renderMeter(v, 20, 80, ColorNone, false, pose, true)
	hidden := renderMeter(v, 20, postureMinColumns-1, ColorNone, false, pose, true)
	sprite := renderPosture(pose, ColorNone, false)
	for row := range visible {
		if !strings.HasSuffix(visible[row], sprite[row]) {
			t.Fatalf("wide row %d omitted the character", row)
		}
		if strings.HasSuffix(hidden[row], sprite[row]) || terminalCellWidth(hidden[row]) >= terminalCellWidth(visible[row]) {
			t.Fatalf("narrow row %d retained or reserved character space", row)
		}
	}

	var output bytes.Buffer
	r := NewRenderer(&output, 80, 24, true)
	r.color = ColorNone
	r.Start()
	r.Update(snapshot(20))
	output.Reset()
	r.Resize(postureMinColumns-1, 24)
	if strings.Contains(output.String(), sprite[0]) {
		t.Fatal("renderer emitted the character below its physical width threshold")
	}
	output.Reset()
	r.Resize(80, 24)
	otherSprite := renderPosture(postureView{state: postureRelaxed, motion: 1}, ColorNone, false)
	if !strings.Contains(output.String(), sprite[0]) && !strings.Contains(output.String(), otherSprite[0]) {
		t.Fatal("renderer did not restore the character when space returned")
	}
}
