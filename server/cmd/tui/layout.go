package main

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m *model) headerRows() int {
	if m.height >= 12 {
		return 1
	}
	return 0
}
func (m *model) borderRows() int {
	if m.height >= 10 && m.width >= 6 {
		return 2
	}
	return 0
}
func (m *model) footerRows() int {
	if m.height >= 3 {
		return 1
	}
	return 0
}
func (m *model) statusText() string {
	if len(m.requests) > 0 || m.height < 7 {
		return ""
	}
	s := safeText(m.status)
	if s == "Ready" {
		s = ""
	}
	if m.newOutput {
		s = "New output below"
	}
	if m.busy && !m.opts.ReducedMotion {
		s = string([]rune("◐◓◑◒")[(m.frame/3)%4]) + " " + s
	}
	return ansi.Truncate(s, m.width, "")
}
func (m *model) requestBudget() int {
	history := 0
	if m.height >= 8 {
		history = 2
	} else if m.height >= 5 {
		history = 1
	}
	return max(1, m.height-m.headerRows()-m.borderRows()-m.footerRows()-history)
}
func (m *model) resize() {
	inputWidth := max(1, m.width-m.borderRows()-2)
	m.draft.SetWidth(inputWidth)
	lines := 0
	for _, line := range strings.Split(m.draft.Value(), "\n") {
		lines += max(1, (ansi.StringWidth(line)+inputWidth-1)/inputWidth)
	}
	statusRows := 0
	if m.statusText() != "" {
		statusRows = 1
	}
	room := m.height - m.headerRows() - m.borderRows() - m.footerRows() - statusRows - 1
	m.draft.SetHeight(max(1, min(lines, 6, max(1, m.height/4), room)))
	m.viewport.Width = m.width
	m.viewport.Height = max(0, m.height-m.headerRows()-lipgloss.Height(m.bottom()))
	if m.follow {
		m.viewport.GotoBottom()
	}
}
func (m *model) footer() string {
	text := "Wheel/PgUp/PgDn scroll · Enter send · F1 help"
	if m.focus != -1 {
		text = "↑↓ scroll · End latest · Tab focus · F1 help"
	}
	if len(m.requests) > 0 {
		text = "↑↓ choose · Enter confirm · ←→ pan · F1 help"
	}
	if m.width < 55 {
		text = "Enter send · F1 help"
		if m.focus != -1 {
			text = "↑↓ scroll · F1 help"
		}
		if len(m.requests) > 0 {
			text = "Enter OK · F1 help"
		}
	}
	if m.width < 24 {
		text = "Enter · F1 help"
	}
	return m.shortcutStyle(text)
}
func (m *model) bottom() string {
	var parts []string
	if s := m.statusText(); s != "" {
		parts = append(parts, s)
	}
	input := m.draft.View()
	if len(m.requests) > 0 {
		input = m.requestView()
	}
	if m.borderRows() > 0 {
		border := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Width(max(1, m.width-2))
		if !m.opts.NoColor {
			color := "8"
			if m.focus == -1 && len(m.requests) == 0 {
				color = "6"
			}
			border = border.BorderForeground(lipgloss.Color(color))
		}
		input = border.Render(input)
	}
	parts = append(parts, input)
	if m.footerRows() > 0 {
		parts = append(parts, m.footer())
	}
	return strings.Join(parts, "\n")
}
func (m *model) View() string {
	if m.showHelp {
		return m.helpView()
	}
	var parts []string
	if m.headerRows() > 0 {
		state := "connecting"
		if m.connected {
			state = "connected"
		} else if m.connectionLost {
			state = "offline"
		}
		header := "Cortisol · " + filepath.Base(m.workspace) + " · " + state
		if m.width < 45 {
			header = "Cortisol · " + state
		}
		parts = append(parts, m.accent(ansi.Truncate(header, m.width, ""), "13"))
	}
	if m.viewport.Height > 0 {
		parts = append(parts, m.viewport.View())
	}
	bottom := m.bottom()
	if len(m.requests) > 0 {
		r := m.requests[0]
		r.barY = m.headerRows() + m.viewport.Height + r.barRow
		if m.borderRows() > 0 {
			r.barY++
		}
	}
	parts = append(parts, bottom)
	return fitScreen(strings.Join(parts, "\n"), m.width, m.height)
}

const keyboardHelp = `Keyboard shortcuts
Enter: send a prompt or confirm a choice
Alt+Enter: insert a newline
Wheel / PgUp / PgDn: scroll conversation
Tab / Shift+Tab: move focus
Up / Down: scroll history or choose an option
Ctrl+Home / Ctrl+End: first / latest message
End: latest message when history is focused
Left / Right: scroll long approval text
Alt+Left / Alt+Right: scroll while entering text
Ctrl+N / Ctrl+P: read approval details down / up
Ctrl+D: show exact approval request
Esc: interrupt the running task
Ctrl+C or /quit: exit
F1 or Esc: close this help`

func (m *model) helpView() string {
	lines := strings.Split(ansi.Wrap(keyboardHelp, max(1, m.width), ""), "\n")
	count := max(1, m.height-1)
	m.helpOffset = min(m.helpOffset, max(0, len(lines)-count))
	page := strings.Join(lines[m.helpOffset:min(len(lines), m.helpOffset+count)], "\n")
	if m.height > 1 {
		page += "\n" + m.shortcutStyle("↑↓ scroll · F1 close")
	}
	return fitScreen(page, m.width, m.height)
}
func (m *model) helpKey(k tea.KeyMsg) {
	switch k.String() {
	case "esc", "f1":
		m.showHelp = false
	case "up":
		m.helpOffset = max(0, m.helpOffset-1)
	case "down":
		m.helpOffset++
	case "pgup":
		m.helpOffset = max(0, m.helpOffset-max(1, m.height-1))
	case "pgdown":
		m.helpOffset += max(1, m.height-1)
	}
}
