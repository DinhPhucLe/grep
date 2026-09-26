package terminal

import (
	"math"
	"strconv"
	"strings"

	"cortisol-cli/metrics"
)

// MeterView is the prepared text and real score given to the pixel renderer.
// Its labels and trend are independent of the animated needle position.
type MeterView struct {
	Known                   bool
	Score                   float64
	RiskLabel, Trend, Delta string
}

// DefaultRiskLabel is temporary presentation policy, kept outside geometry and
// terminal painting. Fractional scores use the same boundaries: <=33, <=66.
func DefaultRiskLabel(score float64) string {
	switch {
	case score <= 33:
		return "LOW"
	case score <= 66:
		return "MODERATE"
	default:
		return "HIGH"
	}
}

// PrepareMeter preserves supplied labels/trends. Only an omitted risk label
// receives the temporary display default. Trends/deltas remain producer-owned;
// this component never derives them from animation or saves score history.
func PrepareMeter(snapshot metrics.Snapshot) MeterView {
	if snapshot.Validate() != nil {
		return MeterView{RiskLabel: "Invalid metrics"}
	}
	v := MeterView{RiskLabel: cleanLabel(snapshot.RiskLabel), Trend: snapshot.Trend}
	if snapshot.Score == nil {
		if v.RiskLabel == "" {
			v.RiskLabel = "waiting for metrics"
		}
		return v
	}
	v.Known, v.Score = true, *snapshot.Score
	if v.RiskLabel == "" {
		v.RiskLabel = DefaultRiskLabel(v.Score)
	}
	if snapshot.Delta != nil {
		v.Delta = strings.TrimSuffix(strconv.FormatFloat(*snapshot.Delta, 'f', 1, 64), ".0")
		if *snapshot.Delta > 0 {
			v.Delta = "+" + v.Delta
		}
	}
	return v
}

func (v MeterView) number() string {
	if !v.Known {
		return "--"
	}
	return strconv.FormatFloat(math.Round(v.Score), 'f', 0, 64)
}

func (v MeterView) trend(ascii bool) string {
	var arrow, word string
	switch v.Trend {
	case "rising":
		arrow, word = "↑", "rising"
	case "falling":
		arrow, word = "↓", "falling"
	case "stable":
		arrow, word = "→", "stable"
	default:
		return "trend --"
	}
	if ascii {
		switch v.Trend {
		case "rising":
			arrow = "^"
		case "falling":
			arrow = "v"
		case "stable":
			arrow = ">"
		}
	}
	return joinNonempty(" ", arrow, word, v.Delta)
}
