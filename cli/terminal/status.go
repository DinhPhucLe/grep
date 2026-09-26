package terminal

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"cortisol-cli/metrics"
)

// RenderStatusLine is the plain, single-row fallback used by small terminals
// and --render. Numeric scores are never truncated. The color argument remains
// for API compatibility; the compact summary is intentionally plain text.
func RenderStatusLine(snapshot metrics.Snapshot, columns int, color bool) string {
	return compactStatus(PrepareMeter(snapshot), max(0, columns-1))
}

func compactStatus(view MeterView, width int) string {
	if !view.Known {
		for _, line := range []string{"VIBE -- " + view.RiskLabel, "VIBE --", "--"} {
			if len(line) <= width {
				return line
			}
		}
		return ""
	}
	arrow := ""
	switch view.Trend {
	case "rising":
		arrow = "^"
	case "falling":
		arrow = "v"
	case "stable":
		arrow = ">"
	}
	gap := " "
	if width >= 60 {
		gap = "  "
	}
	number := view.number()
	for _, line := range []string{
		joinNonempty(gap, "VIBE", number, view.RiskLabel, arrow, view.Delta),
		joinNonempty(gap, "VIBE", number, view.RiskLabel, arrow),
		joinNonempty(gap, "VIBE", number, view.RiskLabel),
		joinNonempty(" ", "VIBE", number, arrow),
		joinNonempty(" ", "V", number, arrow),
		joinNonempty(" ", number, arrow), number + arrow, number,
	} {
		if len(line) <= width {
			return line
		}
	}
	return ""
}

func joinNonempty(separator string, parts ...string) string {
	kept := parts[:0]
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, separator)
}

func cleanLabel(value string) string {
	var stream ansiStream
	var result strings.Builder
	for _, token := range stream.push(value) {
		// String controls, including C1 forms, must disappear as a whole.
		first, _ := utf8.DecodeRuneInString(token)
		if first >= 0x80 && first <= 0x9f {
			continue
		}
		if token == "" || token[0] == '\x1b' || token[0] < 0x20 && token != "\t" && token != "\n" && token != "\r" {
			continue
		}
		for _, r := range token {
			if unicode.IsSpace(r) {
				result.WriteByte(' ')
			} else if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
				continue
			} else if r < 127 {
				result.WriteRune(r)
			} else {
				result.WriteByte('?')
			}
		}
	}
	return strings.Join(strings.Fields(result.String()), " ")
}
