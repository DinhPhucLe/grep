package terminal

import (
	"math"
	"time"
)

// NeedlePosition is a normalized point on the upper semicircle. The pivot is
// always (0, 0); screen coordinates have positive Y pointing down.
type NeedlePosition struct{ X, Y float64 }

// ScoreToNeedlePosition contains geometry only: 0 is left, 50 up, 100 right.
func ScoreToNeedlePosition(score float64) NeedlePosition {
	if math.IsNaN(score) {
		score = 0
	}
	angle := math.Pi * max(0, min(100, score)) / 100
	return NeedlePosition{-math.Cos(angle), -math.Sin(angle)}
}

// needleAnimation owns presentation values only, never producer history.
type needleAnimation struct {
	actualScore, displayedScore, targetScore float64
	known                                    bool
	last, settled                            time.Time
}

func (a *needleAnimation) update(score *float64, now time.Time) {
	if score == nil {
		*a = needleAnimation{}
		return
	}
	if !a.known {
		*a = needleAnimation{actualScore: *score, displayedScore: *score, targetScore: *score, known: true, last: now, settled: now}
		return
	}
	if *score != a.actualScore {
		a.actualScore, a.targetScore, a.settled = *score, *score, time.Time{}
	}
	dt := max(0, min(.2, now.Sub(a.last).Seconds()))
	a.last = now
	if a.settled.IsZero() && math.Abs(a.displayedScore-a.targetScore) < .15 {
		a.settled = now
	}
	if !a.settled.IsZero() {
		idle := max(0, now.Sub(a.settled).Seconds()-.5)
		// Slowly introduce a periodic +/-1.25 point sway; no randomness.
		a.targetScore = max(0, min(100, a.actualScore+1.25*min(1, idle)*math.Sin(idle*2*math.Pi/3)))
	}
	a.displayedScore += (a.targetScore - a.displayedScore) * (1 - math.Exp(-dt/.28))
}
