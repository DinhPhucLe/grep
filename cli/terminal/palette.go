package terminal

import (
	"fmt"
	"os"
	"strings"
)

type ColorMode uint8

const (
	ColorNone ColorMode = iota
	ColorBasic
	Color256
	ColorTrue
)

func detectColorMode() ColorMode {
	if _, present := os.LookupEnv("NO_COLOR"); present || os.Getenv("TERM") == "dumb" {
		return ColorNone
	}
	term, color := strings.ToLower(os.Getenv("TERM")), strings.ToLower(os.Getenv("COLORTERM"))
	if color == "truecolor" || color == "24bit" || os.Getenv("WT_SESSION") != "" || strings.Contains(term, "direct") || strings.Contains(term, "truecolor") {
		return ColorTrue
	}
	if strings.Contains(term, "256color") {
		return Color256
	}
	return ColorBasic
}

type rgb struct{ r, g, b int }

func riskColor(score float64) rgb {
	stops := [...]float64{0, 33, 66, 100}
	colors := [...]rgb{{83, 211, 119}, {255, 212, 80}, {255, 143, 61}, {255, 78, 78}}
	score = max(0, min(100, score))
	for i := 1; i < len(stops); i++ {
		if score <= stops[i] {
			a, b := colors[i-1], colors[i]
			f := (score - stops[i-1]) / (stops[i] - stops[i-1])
			mix := func(x, y int) int { return int(float64(x) + float64(y-x)*f + .5) }
			return rgb{mix(a.r, b.r), mix(a.g, b.g), mix(a.b, b.b)}
		}
	}
	return colors[3]
}

func (mode ColorMode) ink(c rgb, background bool) string {
	base := 38
	if background {
		base = 48
	}
	switch mode {
	case ColorTrue:
		return fmt.Sprintf("%s%d;2;%d;%d;%dm", CSI, base, c.r, c.g, c.b)
	case Color256:
		cube := func(v int) int { return (v*5 + 127) / 255 }
		return fmt.Sprintf("%s%d;5;%dm", CSI, base, 16+36*cube(c.r)+6*cube(c.g)+cube(c.b))
	case ColorBasic:
		code := 97 // neutral needle, bright white
		switch {
		case c.r < 70 && c.g < 70 && c.b < 70:
			code = 30
		case abs(c.r-c.g) < 35 && abs(c.g-c.b) < 35:
			if c.r < 190 {
				code = 90
			}
		case c.r < 150:
			code = 32
		case c.g > 180 && c.b < 150:
			code = 93
		case c.g > 100 && c.b < 150:
			code = 33
		case c.g < 110:
			code = 91
		}
		if background {
			code += 10
		}
		return fmt.Sprintf("%s%dm", CSI, code)
	default:
		return ""
	}
}
