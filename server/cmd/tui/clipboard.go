package main

import (
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

type clipboardAccess interface {
	ReadAll() (string, error)
	WriteAll(string) error
}
type systemClipboard struct{}

func (systemClipboard) ReadAll() (string, error) { return clipboard.ReadAll() }
func (systemClipboard) WriteAll(s string) error  { return clipboard.WriteAll(s) }

type clipboardResult struct {
	err       error
	copied    bool
	text      string
	requestID string
	question  int
}
type cellPoint struct{ x, y int }
type textSelection struct {
	lines              []string
	start, end         cellPoint
	dragging, selected bool
}

func (m *model) copyText() tea.Cmd {
	text := m.selectedText()
	if text == "" {
		m.clipboardNotice = "Drag over text, then Ctrl+C to copy"
		return nil
	}
	cb := m.clipboard
	return func() tea.Msg { return clipboardResult{copied: true, err: cb.WriteAll(text)} }
}
func (m *model) pasteText() tea.Cmd {
	m.pendingPastes++
	cb := m.clipboard
	id := ""
	question := 0
	if m.requestInputActive() {
		id = string(m.requests[0].message.ID)
		question = m.requests[0].question
	}
	return func() tea.Msg {
		s, err := cb.ReadAll()
		return clipboardResult{text: s, err: err, requestID: id, question: question}
	}
}

func (m *model) requestInputActive() bool {
	if len(m.requests) == 0 {
		return false
	}
	r := m.requests[0]
	if len(r.questions) > 0 {
		return r.selected >= len(r.questions[r.question].Options)
	}
	// Index 2 is "Send form data (JSON)" on mcpServer/elicitation/request.
	return r.message.Method == "mcpServer/elicitation/request" && r.selected == 2
}

func (m *model) syncRequestInputFocus() {
	if len(m.requests) == 0 {
		return
	}
	r := m.requests[0]
	if m.requestInputActive() {
		r.input.Focus()
		return
	}
	r.input.Blur()
}
func (m *model) selectionMouse(v tea.MouseMsg) bool {
	top := m.headerRows()
	s := &m.selection
	if v.Action == tea.MouseActionRelease && s.dragging {
		s.end = m.selectionPoint(v)
		s.dragging = false
		if s.start == s.end {
			m.selection = textSelection{}
		} else {
			s.selected = true
		}
		return true
	}
	if v.Button != tea.MouseButtonLeft {
		return false
	}
	if v.Action == tea.MouseActionPress {
		m.clipboardNotice = ""
		m.selection = textSelection{}
		if v.Y < top || v.Y >= top+m.viewport.Height {
			return false
		}
		if m.dirty {
			m.refresh()
		}
		p := cellPoint{max(0, min(v.X, m.width-1)), v.Y - top}
		m.selection = textSelection{lines: strings.Split(m.viewport.View(), "\n"), start: p, end: p, dragging: true}
		m.follow = false
		return true
	}
	if v.Action == tea.MouseActionMotion && s.dragging {
		s.end = m.selectionPoint(v)
		s.selected = s.start != s.end
		return true
	}
	return false
}
func (m *model) selectionPoint(v tea.MouseMsg) cellPoint {
	return cellPoint{max(0, min(v.X, m.width-1)), max(0, min(v.Y-m.headerRows(), len(m.selection.lines)-1))}
}
func (s textSelection) bounds() (cellPoint, cellPoint) {
	a, b := s.start, s.end
	if a.y > b.y || a.y == b.y && a.x > b.x {
		a, b = b, a
	}
	return a, b
}

// Snap endpoints to whole grapheme clusters, including wide emoji and accents.
func snapCells(s string, left, right int) (int, int) {
	pos := 0
	g := uniseg.NewGraphemes(ansi.Strip(s))
	for g.Next() {
		next := pos + ansi.StringWidth(g.Str())
		if left > pos && left < next {
			left = pos
		}
		if right > pos && right < next {
			right = next
		}
		pos = next
	}
	return left, right
}
func (m *model) selectedText() string {
	s := m.selection
	if !s.selected {
		return ""
	}
	a, b := s.bounds()
	var parts []string
	for y := a.y; y <= b.y && y < len(s.lines); y++ {
		line := ansi.Strip(s.lines[y])
		left, right := 0, ansi.StringWidth(line)
		if y == a.y {
			left = a.x
		}
		if y == b.y {
			right = b.x + 1
		}
		left, right = snapCells(line, left, right)
		parts = append(parts, strings.TrimRight(ansi.Cut(line, left, right), " \t"))
	}
	return strings.Join(parts, "\n")
}
func (m *model) conversationView() string {
	s := m.selection
	if len(s.lines) == 0 {
		return m.viewport.View()
	}
	lines := append([]string(nil), s.lines...)
	if s.selected {
		a, b := s.bounds()
		for y := a.y; y <= b.y && y < len(lines); y++ {
			line := lines[y]
			left, right := 0, ansi.StringWidth(line)
			if y == a.y {
				left = a.x
			}
			if y == b.y {
				right = b.x + 1
			}
			left, right = snapCells(line, left, right)
			// Reverse video also makes selection visible when text colors are disabled.
			selected := "\x1b[7m" + ansi.Strip(ansi.Cut(line, left, right)) + "\x1b[0m"
			lines[y] = ansi.Cut(line, 0, left) + selected + ansi.Cut(line, right, ansi.StringWidth(line))
		}
	}
	view := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Height(m.viewport.Height).MaxHeight(m.viewport.Height).Width(m.width).Render(view)
}

// A command line lets /quit work while a non-editable approval or history has
// focus. It never executes a command from pasted text.
func (m *model) quitKey(k tea.KeyMsg) (bool, tea.Cmd) {
	if !m.quitMode {
		return false, nil
	}
	switch k.String() {
	case "esc":
		m.quitMode = false
		m.quitDraft = ""
	case "backspace":
		r := []rune(m.quitDraft)
		if len(r) > 0 {
			m.quitDraft = string(r[:len(r)-1])
		}
	case "enter":
		if strings.TrimSpace(m.quitDraft) == "/quit" || strings.TrimSpace(m.quitDraft) == "/exit" {
			return true, tea.Quit
		}
		m.clipboardNotice = "Use /quit to exit or Esc to cancel"
	default:
		if k.Type == tea.KeyRunes && !k.Paste {
			m.quitDraft += string(k.Runes)
		}
	}
	return true, nil
}
