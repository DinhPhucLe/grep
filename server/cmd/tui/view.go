package main

import (
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"path/filepath"
	"strings"
	"unicode"
)

// Strip controls before adding any client-owned ANSI styling. Raw protocol
// content stays untouched in the model and session log.
func safeText(s string) string {
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
func (m *model) role(user bool) string {
	if user {
		if m.opts.NoIcons {
			return "You"
		}
		return "👤 You"
	}
	if m.opts.NoIcons {
		return "Codex"
	}
	return "✦ Codex"
}
func (m *model) accent(s, color string) string {
	if m.opts.NoColor {
		return s
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).Render(s)
}
func (m *model) markdown(i *conversationItem) string {
	w := max(1, m.width-2)
	if i.cache != "" && i.cacheWidth == w && i.cacheRaw == i.raw {
		return i.cache
	}
	raw := safeText(i.raw)
	out := raw
	// Render the current buffer as Markdown at every width. Glamour handles
	// unfinished fences, and the raw source remains intact for subsequent deltas.
	if raw != "" {
		style := styles.DarkStyleConfig
		style.Document.Margin = nil
		yes := true
		style.Link.Underline = &yes
		renderer, err := glamour.NewTermRenderer(glamour.WithStyles(style), glamour.WithWordWrap(w))
		if err == nil {
			if rendered, err := renderer.Render(raw); err == nil {
				out = strings.TrimSpace(rendered)
			}
		}
	}
	out = ansi.Wrap(out, w, "")
	if m.opts.NoColor {
		out = ansi.Strip(out)
	}
	// Glamour pads rendered lines. Remove display padding so the live cursor
	// follows the last character instead of wrapping onto an empty line.
	lines := strings.Split(out, "\n")
	for n, line := range lines {
		width := ansi.StringWidth(strings.TrimRightFunc(ansi.Strip(line), unicode.IsSpace))
		lines[n] = ansi.Truncate(line, width, "")
	}
	for len(lines) > 0 && ansi.StringWidth(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	out = strings.Join(lines, "\n")
	i.cache = out
	i.cacheWidth = w
	i.cacheRaw = i.raw
	return out
}

func (m *model) refresh() {
	var b strings.Builder
	focusLine := 0
	for n, i := range m.items {
		if i.kind == "commandExecution" {
			continue
		}
		if n == m.focus {
			focusLine = strings.Count(b.String(), "\n")
		}
		switch i.kind {
		case "userMessage":
			b.WriteString(m.accent(m.role(true), "6") + "\n" + ansi.Wrap(safeText(i.raw), max(1, m.width-2), ""))
		case "agentMessage":
			body := m.markdown(i)
			if !i.done && m.busy && !m.opts.ReducedMotion && m.frame%30 < 15 {
				body = ansi.Wrap(body+"▌", max(1, m.width-2), "")
			}
			b.WriteString(m.accent(m.role(false), "13") + "\n" + body)
		default:
			prefix := "▸ "
			if m.opts.NoIcons {
				prefix = "+ "
			}
			if i.expanded {
				prefix = "- "
			}
			if n == m.focus {
				prefix = "> "
			}
			state := i.status
			if state == "" {
				state = "in progress"
			}
			title := i.command
			if i.kind == "fileChange" {
				title = fileChangeLabel(i)
			}
			if i.exitCode != nil {
				state += fmt.Sprintf(" (exit %d)", *i.exitCode)
			}
			b.WriteString(ansi.Wrap(safeText(prefix+title+" — "+state), max(1, m.width-2), ""))
			if i.expanded {
				b.WriteString("\n" + ansi.Wrap(safeText(i.raw+"\n"+i.output), max(1, m.width-2), ""))
			}
		}
		b.WriteString("\n\n")
	}
	m.viewport.SetContent(strings.TrimRight(b.String(), "\n"))
	if m.revealFocus {
		if focusLine < m.viewport.YOffset || focusLine >= m.viewport.YOffset+m.viewport.Height {
			m.viewport.SetYOffset(focusLine)
		}
		m.revealFocus = false
	}
	if m.follow {
		m.viewport.GotoBottom()
		m.newOutput = false
	}
	m.dirty = false
}
func (m *model) resize() {
	m.draft.SetWidth(max(1, m.width-4))
	lines := 1
	for _, line := range strings.Split(m.draft.Value(), "\n") {
		lines += max(1, (ansi.StringWidth(line)+max(1, m.width-4)-1)/max(1, m.width-4))
	}
	m.draft.SetHeight(min(6, max(1, lines-1)))
	m.viewport.Width = m.width
	bottom := lipgloss.Height(m.bottom())
	m.viewport.Height = max(1, m.height-bottom-3)
	if m.follow {
		m.viewport.GotoBottom()
	}
}
func (m *model) bottom() string {
	status := safeText(m.status)
	if status == "Ready" {
		status = ""
	}
	if len(m.requests) > 0 {
		status = "Codex is waiting for your choice"
	}
	if m.busy && len(m.requests) == 0 {
		if !m.opts.ReducedMotion {
			status = string([]rune("◐◓◑◒")[(m.frame/3)%4]) + " " + status
		}
		status += " · drafting allowed; send after turn"
	}
	if m.newOutput {
		if status == "" {
			status = "New output below"
		} else {
			status = "New output below · " + status
		}
	}
	footer := "Wheel/PgUp/PgDn scroll · Ctrl+End latest · Enter send · Alt+Enter newline"
	if m.focus != -1 {
		footer = "Wheel/↑↓/PgUp/PgDn scroll · End latest · Tab focus · Enter expand"
	}
	input := m.draft.View()
	if len(m.requests) > 0 {
		input = m.requestView()
		footer = "↑↓ choose · Enter confirm · ←→ scroll · Ctrl+D details"
		if len(m.requests[0].questions) > 0 {
			footer = "↑↓ choose · Enter answer · Ctrl+N/P read more"
		}
	}
	border := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Width(max(1, m.width-2))
	if !m.opts.NoColor {
		border = border.BorderForeground(lipgloss.Color("8"))
		if m.focus == -1 && len(m.requests) == 0 {
			border = border.BorderForeground(lipgloss.Color("6"))
		}
	}
	return ansi.Truncate(status, m.width, "") + "\n" + border.Render(input) + "\n" + m.shortcutStyle(ansi.Truncate(footer, m.width, ""))
}
func (m *model) View() string {
	conn := "connecting"
	if m.connected {
		conn = "connected"
	} else if strings.HasPrefix(m.status, "Disconnected") || strings.HasPrefix(m.status, "Failed") {
		conn = "offline"
	}
	header := "Cortisol · " + filepath.Base(m.workspace) + " · " + conn
	if m.width < 45 {
		header = "Cortisol · " + conn
	}
	bottom := m.bottom()
	if len(m.requests) > 0 {
		m.requests[0].barY = m.viewport.Height + 4 + m.requests[0].barRow
		if m.height < 12 {
			m.requests[0].barY = 3 + m.requests[0].barRow
		}
	}
	if m.height < 12 {
		return fitScreen(m.accent(header, "13")+"\n"+bottom, m.width, m.height)
	}
	return fitScreen(m.accent(ansi.Truncate(header, m.width, ""), "13")+"\n\n"+m.viewport.View()+"\n"+bottom, m.width, m.height)
}

func fileChangeLabel(i *conversationItem) string {
	var changes []struct {
		Path string
		Kind struct{ Type string }
	}
	if json.Unmarshal([]byte(i.raw), &changes) != nil || len(changes) == 0 {
		return "File changes"
	}
	var labels []string
	for _, c := range changes {
		verb := "Change proposed"
		if i.status == "completed" {
			verb = "Updated"
			switch c.Kind.Type {
			case "add":
				verb = "Created"
			case "delete":
				verb = "Deleted"
			}
		} else if i.status == "failed" {
			verb = "Change failed"
		} else if i.status == "declined" {
			verb = "Change declined"
		}
		labels = append(labels, verb+": "+c.Path)
	}
	return strings.Join(labels, "\n")
}
func (m *model) selectedStyle(s string) string {
	if m.opts.NoColor {
		return s
	}
	return lipgloss.NewStyle().Bold(true).Reverse(true).Render(s)
}
func (m *model) shortcutStyle(s string) string {
	parts := strings.Split(s, " · ")
	for n, p := range parts {
		key, desc, ok := strings.Cut(p, " ")
		if ok {
			if m.opts.NoColor {
				parts[n] = "[" + key + "] " + desc
			} else {
				parts[n] = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color("6")).Render(key) + " " + desc
			}
		}
	}
	return ansi.Truncate(strings.Join(parts, " · "), m.width, "")
}
func fitScreen(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for n, line := range lines {
		lines[n] = ansi.Truncate(line, w, "")
	}
	return strings.Join(lines, "\n")
}

// Short labels remain visible while long paths and choices pan horizontally.
func horizontalSlice(s string, offset, width int) string {
	if ansi.StringWidth(s) <= width {
		return s
	}
	return ansi.Cut(s, offset, offset+width)
}
