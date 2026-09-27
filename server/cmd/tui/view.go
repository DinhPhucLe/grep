package main

import (
	"encoding/json"
	"fmt"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
func (m *model) accent(s, color string) string {
	if m.opts.NoColor {
		return s
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).Render(s)
}
func (m *model) quizPanelView(raw string) string {
	width := max(1, m.width-2)
	text := strings.ReplaceAll(safeText(raw), "\t", "    ")
	border := lipgloss.NewStyle()
	if !m.opts.NoColor {
		border = border.Foreground(lipgloss.Color("#BFA65A"))
	}
	// Very narrow terminals still get separators without overflowing the viewport.
	if width < 5 {
		rule := border.Render(strings.Repeat("-", width))
		return rule + "\n" + ansi.Wrap(text, width, "") + "\n" + rule
	}
	inner := width - 4
	rule := border.Render("+" + strings.Repeat("-", width-2) + "+")
	side := border.Render("|")
	lines := strings.Split(ansi.Wrap(text, inner, ""), "\n")
	for n, line := range lines {
		lines[n] = side + " " + line + strings.Repeat(" ", max(0, inner-ansi.StringWidth(line))) + " " + side
	}
	return rule + "\n" + strings.Join(lines, "\n") + "\n" + rule
}

func (m *model) userMessageView(text string) string {
	w := max(1, m.width)
	inner := max(1, w-2)
	wrapped := ansi.Wrap(safeText(text), inner, "")
	if m.opts.NoColor {
		return "\n" + wrapped + "\n"
	}
	return lipgloss.NewStyle().
		Background(lipgloss.AdaptiveColor{Light: "#E8E8E8", Dark: "#454545"}).
		Foreground(lipgloss.AdaptiveColor{Light: "#1A1A1A", Dark: "#E8E8E8"}).
		Padding(1, 1).
		Width(w).
		Render(wrapped)
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
	quizLine := 0
	for n, i := range m.items {
		if i.kind == "commandExecution" {
			continue
		}
		if n == m.focus {
			focusLine = strings.Count(b.String(), "\n")
		}
		switch i.kind {
		case "quizPanel":
			if m.quiz != nil && i == m.quiz.panel {
				quizLine = strings.Count(b.String(), "\n")
			}
			b.WriteString(m.quizPanelView(i.raw))
		case "userMessage":
			b.WriteString(m.userMessageView(i.raw))
		case "agentMessage":
			body := m.markdown(i)
			if !i.done && m.busy && !m.opts.ReducedMotion && m.frame%30 < 15 {
				body = ansi.Wrap(body+"▌", max(1, m.width-2), "")
			}
			b.WriteString(body)
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
	if m.quizJump {
		m.viewport.SetYOffset(quizLine)
		m.quizJump = false
	}
	m.dirty = false
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
func (m *model) subtleHint(s string) string {
	if m.opts.NoColor {
		return ansi.Truncate(s, m.width, "")
	}
	return ansi.Truncate(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(s), m.width, "")
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
