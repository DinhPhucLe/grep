package terminal

import (
	"strings"
	"unicode/utf8"
)

const maxPending = 64 * 1024

// ansiStream preserves controls and UTF-8 characters across arbitrary PTY reads.
// No renderer bytes may be inserted while a control or character is incomplete.
type ansiStream struct {
	state      byte
	pending    strings.Builder
	utf8Tail   string
	osc        bool
	overflowed bool
}

const (
	ground byte = iota
	escape
	intermediate
	csi
	controlString
	stringEscape
)

func (s *ansiStream) complete() bool {
	return !s.overflowed && s.state == ground && s.utf8Tail == ""
}

func (s *ansiStream) push(data string) []string {
	if s.overflowed {
		return []string{data}
	}
	data = s.utf8Tail + data
	s.utf8Tail = ""
	var tokens []string
	var plain strings.Builder
	drain := func() {
		if plain.Len() != 0 {
			tokens = append(tokens, plain.String())
			plain.Reset()
		}
	}
	finish := func() {
		if s.pending.Len() != 0 {
			tokens = append(tokens, s.pending.String())
			s.pending.Reset()
		}
		s.state = ground
	}
	start := func(raw string, r rune) {
		s.pending.WriteString(raw)
		if r == '\x1b' {
			s.state = escape
		} else if r == '\u009b' {
			s.state = csi
		} else {
			s.state = controlString
			s.osc = r == '\u009d'
		}
	}
	for len(data) != 0 {
		if !utf8.FullRuneInString(data) {
			s.utf8Tail = data
			break
		}
		r, n := utf8.DecodeRuneInString(data)
		raw := data[:n]
		data = data[n:]
		// Raw C1 bytes are controls too; valid UTF-8 continuation bytes are
		// consumed as part of their complete rune before this branch.
		if r == utf8.RuneError && n == 1 {
			r = rune(raw[0])
		}
		introducer := r == '\x1b' || r == 0x90 || r == 0x98 || r == 0x9b || r == 0x9d || r == 0x9e || r == 0x9f
		switch s.state {
		case ground:
			if introducer {
				drain()
				start(raw, r)
			} else if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
				drain()
				tokens = append(tokens, raw)
			} else {
				plain.WriteString(raw)
			}
		case controlString, stringEscape:
			escaped := s.state == stringEscape
			s.pending.WriteString(raw)
			if r == 0x9c || r == 0x18 || r == 0x1a || (s.osc && r == 7) || (escaped && r == '\\') {
				finish()
			} else if r == '\x1b' {
				s.state = stringEscape
			} else {
				s.state = controlString
			}
		default:
			if introducer {
				finish()
				start(raw, r)
				break
			}
			previous := s.state
			s.pending.WriteString(raw)
			if r == 0x18 || r == 0x1a {
				finish()
			} else if r < 0x20 || r == 0x7f {
				// Keep embedded C0 bytes in order within their control.
			} else if previous == escape && r == '[' {
				s.state = csi
			} else if previous == escape && strings.ContainsRune("]P_^X", r) {
				s.state = controlString
				s.osc = r == ']'
			} else if previous == csi {
				if r >= 0x40 && r <= 0x7e || r > 0x3f {
					finish()
				}
			} else if r >= 0x20 && r <= 0x2f {
				s.state = intermediate
			} else {
				finish()
			}
		}
		if s.pending.Len() > maxPending {
			drain()
			tokens = append(tokens, s.pending.String()+data)
			s.pending.Reset()
			s.overflowed = true
			return tokens
		}
	}
	drain()
	return tokens
}

func (s *ansiStream) flush() string {
	tail := s.pending.String() + s.utf8Tail
	s.pending.Reset()
	s.utf8Tail = ""
	s.state = ground
	return tail
}
