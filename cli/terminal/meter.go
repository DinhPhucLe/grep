package terminal

import (
	"math"
	"strings"
)

const meterWidth = 33

type pixel struct {
	on     bool
	ink    rgb
	needle bool
}

// RenderMeter draws five rows. Text comes from real values in view; only the
// needle uses displayedScore. No terminal cursor controls or policy live here.
func RenderMeter(view MeterView, displayedScore float64, columns int, mode ColorMode, ascii bool) []string {
	return renderMeter(view, displayedScore, columns, mode, ascii, postureView{state: postureFor(view)}, true)
}

func renderMeter(view MeterView, displayedScore float64, columns int, mode ColorMode, ascii bool, posture postureView, warnings bool) []string {
	view.RiskLabel, view.Delta = cleanLabel(view.RiskLabel), cleanLabel(view.Delta)
	if view.Known && (math.IsNaN(view.Score) || math.IsInf(view.Score, 0) || view.Score < 0 || view.Score > 100) {
		view = MeterView{RiskLabel: "Invalid metrics"}
		posture = postureView{}
	}
	width := max(0, columns-1)
	if width < 53 {
		return []string{compactStatus(view, width)}
	}
	var pixels [6][meterWidth]pixel
	point := func(score float64) (int, int) {
		p := ScoreToNeedlePosition(score)
		return 16 + int(math.Round(p.X*15)), 5 + int(math.Round(p.Y*5))
	}
	for score := 0.; score <= 100; score += .25 {
		x, y := point(score)
		pixels[y][x] = pixel{on: true, ink: riskColor(score)}
	}
	if view.Known {
		x, y := point(displayedScore)
		// Integer line rasterization from one fixed center to the moving tip.
		dx, dy := x-16, y-5
		steps := max(abs(dx), abs(dy))
		for i := 0; i <= steps; i++ {
			f := float64(i) / float64(max(1, steps))
			px, py := 16+int(math.Round(float64(dx)*f)), 5+int(math.Round(float64(dy)*f))
			pixels[py][px] = pixel{on: true, ink: rgb{235, 241, 248}, needle: true}
		}
	}
	side := []string{"VIBECODE", "VIBE " + view.number(), view.RiskLabel, view.trend(ascii), postureCaption(view, warnings)}
	if !view.Known {
		side[3] = ""
	}
	plot := []string{" LOW            MID         HIGH ", "", "", "", " 0              50           100 "}
	for row := 0; row < 3; row++ {
		var line strings.Builder
		for col := 0; col < meterWidth; col++ {
			top, bottom := pixels[row*2][col], pixels[row*2+1][col]
			if row == 2 && col == 16 && view.Known {
				line.WriteString(mode.ink(rgb{235, 241, 248}, false))
				if ascii {
					line.WriteByte('+')
				} else {
					line.WriteString("\u25cf")
				}
				if mode != ColorNone {
					line.WriteString(CSI + "0m")
				}
				continue
			}
			if !top.on && !bottom.on {
				line.WriteByte(' ')
				continue
			}
			if ascii {
				p := top
				if !p.on || bottom.needle {
					p = bottom
				}
				line.WriteString(mode.ink(p.ink, false))
				if p.needle {
					line.WriteByte('*')
				} else {
					line.WriteByte('#')
				}
			} else {
				switch {
				case top.on && bottom.on:
					if mode == ColorNone {
						line.WriteString("█")
					} else {
						line.WriteString(mode.ink(top.ink, false) + mode.ink(bottom.ink, true) + "▀")
					}
				case top.on:
					line.WriteString(mode.ink(top.ink, false) + "▀")
				default:
					line.WriteString(mode.ink(bottom.ink, false) + "▄")
				}
			}
			if mode != ColorNone {
				line.WriteString(CSI + "0m")
			}
		}
		plot[row+1] = line.String()
	}
	lines := make([]string, 5)
	textWidth := width - meterWidth - 2
	var sprite []string
	if columns >= postureMinColumns {
		sprite = renderPosture(posture, mode, ascii)
		textWidth -= postureWidth + 2
	}
	for i := range lines {
		// Plot rows have a fixed cell width despite their ANSI byte lengths.
		pad := max(0, meterWidth-terminalCellWidth(plot[i]))
		text := truncate(cleanMeterText(side[i]), textWidth)
		lines[i] = plot[i] + strings.Repeat(" ", pad) + "  " + text
		if sprite != nil {
			lines[i] += strings.Repeat(" ", max(0, textWidth-terminalCellWidth(text))) + "  " + sprite[i]
		}
	}
	return lines
}

// terminalCellWidth ignores ANSI controls instead of counting their bytes or
// runes as visible columns. All meter-owned printable glyphs have stable width.
func terminalCellWidth(value string) int {
	var stream ansiStream
	width := 0
	for _, token := range stream.push(value) {
		if token == "" || token[0] == '\x1b' {
			continue
		}
		for _, r := range token {
			if cells, safe := runeWidth(r); safe {
				width += cells
			}
		}
	}
	return width
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
func truncate(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 0 {
		return ""
	}
	return string(runes[:width-1]) + "~"
}

// Preserve only the three renderer-owned arrows; producer labels were already
// sanitized by the presentation adapter, and public callers are sanitized here.
func cleanMeterText(s string) string {
	for _, arrow := range []string{"↑", "→", "↓"} {
		if strings.HasPrefix(s, arrow+" ") {
			return arrow + " " + cleanLabel(strings.TrimPrefix(s, arrow+" "))
		}
	}
	return cleanLabel(s)
}
