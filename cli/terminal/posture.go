package terminal

import (
	"strings"
	"time"
)

const postureWidth = 16
const postureMinColumns = 78
const postureHold = time.Second

type postureState uint8

const (
	postureUnknown postureState = iota
	postureRelaxed
	postureUpright
	postureTense
)

type postureView struct {
	state  postureState
	blink  bool
	motion uint8
}

func postureFor(view MeterView) postureState {
	if !view.Known {
		return postureUnknown
	}
	switch DefaultRiskLabel(view.Score) {
	case "LOW":
		return postureRelaxed
	case "MODERATE":
		return postureUpright
	default:
		return postureTense
	}
}

// Posture uses real score bands only. A new band must persist for one second;
// motion and blinking are visual state and never enter score history.
type postureAnimation struct {
	current, candidate postureState
	since, epoch       time.Time
}

func (a *postureAnimation) update(view MeterView, now time.Time) postureView {
	desired := postureFor(view)
	if desired == postureUnknown {
		*a = postureAnimation{}
		return postureView{}
	}
	if a.current == postureUnknown {
		a.current, a.candidate, a.since, a.epoch = desired, desired, now, now
	} else if desired != a.candidate {
		a.candidate, a.since = desired, now
	} else if desired != a.current && now.Sub(a.since) >= postureHold {
		a.current = desired
		a.epoch = now
	}
	elapsed := max(0, now.Sub(a.epoch))
	cycle := 8 * time.Second
	switch a.current {
	case postureUpright:
		cycle = 6 * time.Second
	case postureTense:
		cycle = 4 * time.Second
	}
	return postureView{
		state:  a.current,
		blink:  elapsed%(4*time.Second) >= 3800*time.Millisecond,
		motion: uint8((elapsed % cycle) / (100 * time.Millisecond)),
	}
}

func postureCaption(view MeterView, warnings bool) string {
	switch postureFor(view) {
	case postureRelaxed:
		return "CHILL"
	case postureUpright:
		return "PAY ATTENTION"
	case postureTense:
		if warnings {
			return "EMERGENCY PAUSE"
		}
		return "HIGH ALERT"
	default:
		return "AWAITING METRICS"
	}
}

func renderPosture(view postureView, mode ColorMode, ascii bool) []string {
	sprite := catFrame(view)
	// Wide eye whites become cutouts in monochrome, preserving the alert
	// expression when fur and eye whites cannot have different colors.
	on := func(ch byte) bool { return ch != 0 && (mode != ColorNone || ch != 'w') }
	ink := func(ch byte) rgb { return catInk(view.state, ch) }
	lines := make([]string, 5)
	for row := range lines {
		var line strings.Builder
		for col := 0; col < postureWidth; col++ {
			a, b := sprite[row*2][col], sprite[row*2+1][col]
			top, bottom := on(a), on(b)
			switch {
			case !top && !bottom:
				line.WriteByte(' ')
			case ascii:
				ch := a
				if !top {
					ch = b
				}
				line.WriteString(mode.ink(ink(ch), false))
				switch ch {
				case 'd':
					line.WriteByte('.')
				case 'o', 'w':
					line.WriteByte('o')
				default:
					line.WriteByte('#')
				}
			case top && bottom:
				if mode == ColorNone {
					if a == 'd' || b == 'd' {
						line.WriteByte('.')
					} else {
						line.WriteString("█")
					}
				} else {
					line.WriteString(mode.ink(ink(a), false) + mode.ink(ink(b), true) + "▀")
				}
			case top:
				line.WriteString(mode.ink(ink(a), false) + "▀")
			case bottom:
				line.WriteString(mode.ink(ink(b), false) + "▄")
			}
			if mode != ColorNone && (top || bottom) {
				line.WriteString(CSI + "0m")
			}
		}
		lines[row] = line.String()
	}
	return lines
}
