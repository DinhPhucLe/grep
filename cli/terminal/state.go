package terminal

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const CSI = "\x1b["

type cursor struct {
	x, y  int
	style string
	wrap  bool
}

// terminalState tracks just the controls needed to restore a child cursor.
// Any unrecognized stateful control releases the status row before forwarding.
type terminalState struct {
	columns, rows int
	rowOffset     int // physical header height; all tracked coordinates stay logical
	cursor
	top, bottom  int
	visible      bool
	autoWrap     bool
	synchronized bool
	alternate    bool
	saved        cursor
	normal       cursor
	normalSaved  cursor
	normalTop    int
	normalBottom int
	recentSizes  []string
	screen       childScreen
	ownedScreen  bool
}

func newTerminalState(columns, rows int) *terminalState {
	s := &terminalState{columns: columns, rows: rows, cursor: cursor{x: 1, y: 1}, top: 1, bottom: rows, visible: true, autoWrap: true, saved: cursor{x: 1, y: 1}}
	s.rememberSize(columns, rows)
	s.screen.resize(columns, rows)
	return s
}

func (s *terminalState) rememberSize(columns, rows int) {
	s.recentSizes = append(s.recentSizes, fmt.Sprintf("%dx%d", columns, rows))
	if len(s.recentSizes) > 64 {
		s.recentSizes = s.recentSizes[len(s.recentSizes)-64:]
	}
}

func (s *terminalState) resize(columns, rows int) {
	s.screen.resize(columns, rows)
	s.rememberSize(columns, rows)
	s.columns, s.rows = columns, rows
	s.top, s.bottom = 1, rows
	s.normalTop, s.normalBottom = 1, rows
	s.normal.wrap, s.normalSaved.wrap, s.saved.wrap = false, false, false
	s.x, s.y, s.wrap = min(s.x, columns), min(s.y, rows), false
}

func (s *terminalState) restore(c cursor) {
	s.cursor = c
	s.x, s.y = min(c.x, s.columns), min(c.y, s.rows)
}

func (s *terminalState) position() string { return fmt.Sprintf("%s%d;%dH", CSI, s.y+s.rowOffset, s.x) }
func (s *terminalState) margins() string {
	return fmt.Sprintf("%s%d;%dr", CSI, s.top+s.rowOffset, s.bottom+s.rowOffset)
}

func (s *terminalState) lineFeed() {
	if s.y != s.bottom {
		s.y = min(s.rows, s.y+1)
	}
	s.wrap = false
}

func (s *terminalState) accept(token string) (string, bool) {
	before, alternate := s.cursor, s.alternate
	output, safe := s.acceptToken(token)
	if safe {
		s.screen.accept(token, before, alternate, s)
		if s.ownedScreen && alternate != s.alternate {
			output = CSI + "?7l" + s.screen.paint(s.rowOffset) + s.margins() + s.position() + CSI + "0m" + s.style
			if s.autoWrap {
				output += CSI + "?7h"
			}
		}
	}
	return output, safe
}

func (s *terminalState) acceptToken(token string) (string, bool) {
	switch token {
	case "\r":
		s.x, s.wrap = 1, false
	case "\n", "\v", "\f", "\x1bD":
		s.lineFeed()
	case "\b":
		s.x, s.wrap = max(1, s.x-1), false
	case "\t":
		s.x, s.wrap = min(s.columns, ((s.x-1)/8+1)*8+1), false
	case "\x07", "\x00", "\x1b(B", "\x1b=", "\x1b>":
	case "\x1b7":
		s.saved = s.cursor
	case "\x1b8":
		s.restore(s.saved)
		if s.rowOffset > 0 && !s.wrap {
			return token + s.position(), true
		}
	case "\x1bM":
		if s.y != s.top {
			s.y = max(1, s.y-1)
		}
		s.wrap = false
	case "\x1bE":
		s.x = 1
		s.lineFeed()
	default:
		if strings.HasPrefix(token, "\x1b]") {
			// Title, palette, and clipboard protocols have no persistent text state.
			for _, prefix := range []string{"0;", "1;", "2;", "4;", "10;", "11;", "12;", "52;"} {
				if strings.HasPrefix(token[2:], prefix) {
					return token, true
				}
			}
			return "", false
		}
		if strings.HasPrefix(token, CSI) {
			return s.acceptCSI(token)
		}
		if !utf8.ValidString(token) {
			return "", false
		}
		// First validate the entire token so unsupported Unicode does not
		// partially advance the state before the original bytes are forwarded.
		for _, r := range token {
			if _, safe := runeWidth(r); !safe {
				return "", false
			}
		}
		for _, r := range token {
			width, _ := runeWidth(r)
			if width == 0 {
				continue
			}
			if (s.wrap || s.x+width-1 > s.columns) && s.autoWrap {
				s.x = 1
				s.lineFeed()
			}
			s.x += width
			s.wrap = s.x > s.columns && s.autoWrap
			s.x = min(s.columns, s.x)
		}
	}
	return token, true
}

var csiPattern = regexp.MustCompile("^\\x1b\\[([?<>=]?)([0-9;:]*)([ !\"#$&'*+,-./]*)([@-~])$")

func (s *terminalState) acceptCSI(token string) (string, bool) {
	m := csiPattern.FindStringSubmatch(token)
	if m == nil {
		return "", false
	}
	prefix, raw, intermediate, command := m[1], m[2], m[3], m[4]
	if intermediate != "" {
		return token, intermediate == " " && command == "q"
	}
	if strings.Contains(raw, ":") && command != "m" {
		return "", false
	}
	var params []int
	for _, p := range strings.Split(raw, ";") {
		n := 0
		if p != "" {
			var err error
			n, err = strconv.Atoi(p)
			if err != nil && command != "m" {
				return "", false
			}
		}
		params = append(params, n)
	}
	value := func(i, fallback int) int {
		if i >= len(params) || params[i] == 0 {
			return fallback
		}
		return params[i]
	}
	first := value(0, 1)
	row := func(v int) int { return max(1, min(s.rows, v)) }
	if prefix == "?" && (command == "h" || command == "l") {
		enabled := command == "h"
		for _, mode := range params {
			// Mouse coordinates are reported in physical rows. Preserve the
			// existing input path by releasing the header before enabling them.
			if s.rowOffset > 0 && enabled && (mode == 1000 || mode == 1002 || mode == 1003 || mode == 1006) {
				return "", false
			}
			switch mode {
			case 1, 7, 12, 25, 1048, 1049, 1003, 1004, 1006, 2004, 2026, 9001:
			default:
				return "", false
			}
		}
		var result strings.Builder
		for _, mode := range params {
			if !s.ownedScreen || mode != 1049 {
				fmt.Fprintf(&result, "%s?%d%s", CSI, mode, command)
			}
			switch mode {
			case 7:
				s.autoWrap = enabled
			case 25:
				s.visible = enabled
			case 2026:
				s.synchronized = enabled
			case 1048:
				if enabled {
					s.saved = s.cursor
				} else {
					s.restore(s.saved)
					if s.rowOffset > 0 && !s.wrap {
						result.WriteString(s.position())
					}
				}
			case 1049:
				if enabled && !s.alternate {
					s.normal, s.normalSaved = s.cursor, s.cursor
					s.normalTop, s.normalBottom = s.top, s.bottom
					s.wrap = false
					s.saved = cursor{x: 1, y: 1}
					s.top, s.bottom = 1, s.rows
				} else if !enabled && s.alternate {
					s.restore(s.normal)
					s.saved = s.normalSaved
					s.top, s.bottom = s.normalTop, s.normalBottom
				}
				s.alternate = enabled
				result.WriteString(s.margins() + s.position())
			}
		}
		return result.String(), true
	}
	// A cursor report would expose the header offset to the unmodified input
	// stream. Fall back before forwarding the request instead of lying to Codex.
	if command == "n" && first == 6 && s.rowOffset > 0 {
		return "", false
	}
	if command == "c" || command == "n" || command == "u" && prefix != "" {
		return token, true
	}
	if prefix != "" {
		return "", false
	}
	if command == "t" && len(params) == 3 && params[0] == 8 {
		size := fmt.Sprintf("%dx%d", params[2], params[1])
		for _, previous := range s.recentSizes {
			if previous == size {
				return "", true // ConPTY resize acknowledgement, not host resize.
			}
		}
	}
	switch command {
	case "m":
		if raw == "" || raw == "0" {
			s.style = ""
		} else if len(s.style)+len(token) > 8192 {
			return "", false
		} else {
			s.style += token
		}
	case "s":
		if raw != "" {
			return "", false
		}
		s.saved = s.cursor
	case "u":
		if raw != "" {
			return "", false
		}
		s.restore(s.saved)
		if s.rowOffset > 0 && !s.wrap {
			return token + s.position(), true
		}
	case "r":
		top, bottom := row(first), row(value(1, s.rows))
		if top >= bottom {
			return "", true
		}
		s.top, s.bottom, s.x, s.y, s.wrap = top, bottom, 1, 1, false
		return s.margins() + s.position(), true
	case "H", "f":
		s.y, s.x, s.wrap = row(first), min(s.columns, value(1, 1)), false
		return s.position(), true
	case "d":
		s.y, s.wrap = row(first), false
		return fmt.Sprintf("%s%dd", CSI, s.y+s.rowOffset), true
	case "G", "`":
		s.x, s.wrap = min(s.columns, first), false
		return fmt.Sprintf("%s%dG", CSI, s.x), true
	case "A", "B", "C", "D", "E", "F", "a", "e":
		if command == "A" || command == "F" {
			lower := 1
			if s.y >= s.top {
				lower = s.top
			}
			s.y -= min(first, s.y-lower)
		}
		if command == "B" || command == "E" || command == "e" {
			upper := s.rows
			if s.y <= s.bottom {
				upper = s.bottom
			}
			s.y += min(first, upper-s.y)
		}
		if command == "C" || command == "a" {
			s.x += min(first, s.columns-s.x)
		}
		if command == "D" {
			s.x -= min(first, s.x-1)
		}
		if command == "E" || command == "F" {
			s.x = 1
		}
		s.wrap = false
		return s.position(), true
	case "J":
		s.wrap = false
		if s.rowOffset > 0 {
			return s.eraseDisplay(params[0])
		}
	case "K", "X", "P", "@", "L", "M", "S", "T":
		s.wrap = false
	default:
		return "", false
	}
	return token, true
}

// ED 1/2 normally erase the header too. Confine erasure to the child viewport
// without borrowing its save slot. ED 0 and scrollback-only ED 3 are unchanged.
func (s *terminalState) eraseDisplay(mode int) (string, bool) {
	if mode == 0 || mode == 3 {
		return fmt.Sprintf("%s%dJ", CSI, mode), true
	}
	if mode != 1 && mode != 2 {
		return "", false
	}
	end := s.rows
	if mode == 1 {
		end = s.y - 1
	}
	var out strings.Builder
	for row := 1; row <= end; row++ {
		fmt.Fprintf(&out, "%s%d;1H%s2K", CSI, row+s.rowOffset, CSI)
	}
	out.WriteString(s.position())
	if mode == 1 {
		out.WriteString(CSI + "1K")
	}
	return out.String(), true
}

func runeWidth(r rune) (int, bool) {
	if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == 0x2028 || r == 0x2029 || r == 0xfffd || r == 0xfe0e || r == 0xfe0f || r == 0x20e3 {
		return 0, false
	}
	// Emoji and joined glyphs differ across terminal/font versions; never
	// restore the child's cursor to a guessed position for those characters.
	if r >= 0x1f000 && r <= 0x1faff || r >= 0x2600 && r <= 0x27bf || r == 0x00a9 || r == 0x00ae || r == 0x2122 || r >= 0x231a && r <= 0x231b || r >= 0x23e9 && r <= 0x23fa {
		return 0, false
	}
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
		return 0, true
	}
	if r >= 0x1100 && (r <= 0x115f || r == 0x2329 || r == 0x232a || r >= 0x2e80 && r <= 0xa4cf && r != 0x303f || r >= 0xac00 && r <= 0xd7a3 || r >= 0xf900 && r <= 0xfaff || r >= 0xfe10 && r <= 0xfe19 || r >= 0xfe30 && r <= 0xfe6f || r >= 0xff00 && r <= 0xff60 || r >= 0xffe0 && r <= 0xffe6 || r >= 0x20000 && r <= 0x3fffd) {
		return 2, true
	}
	return 1, true
}
