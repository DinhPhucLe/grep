package main

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const knowledgePickerMaxHits = 5

type knowledgeHit struct {
	ID      string
	Content string
	Topics  []string
	Author  string
	Score   float64
}

type knowledgeSearchPayload struct {
	Items []struct {
		ID      string   `json:"id"`
		Content string   `json:"content"`
		Topics  []string `json:"topics"`
		Authors []struct {
			UserID string `json:"userId"`
			Name   string `json:"name"`
		} `json:"authors"`
	} `json:"items"`
	Scores []float64 `json:"scores"`
}

func parseKnowledgeSearchPayload(raw []byte) ([]knowledgeHit, bool) {
	var payload knowledgeSearchPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false
	}
	if len(payload.Items) == 0 {
		return nil, false
	}
	n := len(payload.Items)
	if n > knowledgePickerMaxHits {
		n = knowledgePickerMaxHits
	}
	hits := make([]knowledgeHit, 0, n)
	for i := 0; i < n; i++ {
		item := payload.Items[i]
		if strings.TrimSpace(item.Content) == "" {
			continue
		}
		author := ""
		if len(item.Authors) > 0 {
			author = item.Authors[0].Name
			if author == "" {
				author = item.Authors[0].UserID
			}
		}
		hit := knowledgeHit{
			ID:      item.ID,
			Content: item.Content,
			Topics:  item.Topics,
			Author:  author,
		}
		if i < len(payload.Scores) {
			hit.Score = payload.Scores[i]
		}
		hits = append(hits, hit)
	}
	if len(hits) == 0 {
		return nil, false
	}
	return hits, true
}

// TryOpenKnowledgePicker opens the right-rail picker when raw is a knowledge_search result.
func (m *model) TryOpenKnowledgePicker(raw []byte) bool {
	hits, ok := parseKnowledgeSearchPayload(raw)
	if !ok {
		return false
	}
	m.knowledgeHits = hits
	m.knowledgeSelected = 0
	m.knowledgePickerOpen = true
	m.draft.Blur()
	m.resize()
	m.dirty = true
	return true
}

func (m *model) closeKnowledgePicker(keepSelection bool) {
	if !keepSelection {
		m.selectedKnowledge = nil
	}
	m.knowledgePickerOpen = false
	m.knowledgeHits = nil
	m.knowledgeSelected = 0
	if len(m.requests) == 0 {
		m.draft.Focus()
	}
	m.resize()
	m.dirty = true
}

func (m *model) knowledgePickerKey(msg tea.KeyMsg) bool {
	if !m.knowledgePickerOpen {
		return false
	}
	switch msg.String() {
	case "up", "k":
		if m.knowledgeSelected > 0 {
			m.knowledgeSelected--
		}
		return true
	case "down", "j":
		if m.knowledgeSelected < len(m.knowledgeHits)-1 {
			m.knowledgeSelected++
		}
		return true
	case "enter":
		if m.knowledgeSelected >= 0 && m.knowledgeSelected < len(m.knowledgeHits) {
			hit := m.knowledgeHits[m.knowledgeSelected]
			m.selectedKnowledge = &hit
			m.status = "Knowledge: " + truncateRunes(hit.Author+": "+hit.Content, 80)
		}
		m.closeKnowledgePicker(true)
		return true
	case "esc":
		m.closeKnowledgePicker(false)
		return true
	}
	return false
}

func (m *model) knowledgePickerView(width, height int) string {
	if width < 8 || height < 1 {
		return ""
	}
	title := m.accent(ansi.Truncate("Knowledge", width, ""), "6")
	lines := []string{title}
	for i, hit := range m.knowledgeHits {
		marker := " "
		if i == m.knowledgeSelected {
			marker = ">"
		}
		author := hit.Author
		if author == "" {
			author = "unknown"
		}
		snippet := truncateRunes(strings.ReplaceAll(hit.Content, "\n", " "), max(8, width-2))
		row := marker + author
		if ansi.StringWidth(row) > width {
			row = ansi.Truncate(row, width, "")
		}
		lines = append(lines, row)
		body := "  " + snippet
		if ansi.StringWidth(body) > width {
			body = ansi.Truncate(body, width, "")
		}
		lines = append(lines, body)
		if len(lines) >= height {
			break
		}
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	out := strings.Join(lines, "\n")
	if !m.opts.NoColor {
		border := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("8")).Width(max(1, width-2)).Height(height)
		return border.Render(out)
	}
	return out
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	if max == 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}

// maybeOpenKnowledgeFromText scans agent text for an embedded knowledge search JSON object.
func (m *model) maybeOpenKnowledgeFromText(text string) {
	if m.knowledgePickerOpen {
		return
	}
	start := strings.Index(text, `"items"`)
	if start < 0 {
		return
	}
	brace := strings.LastIndex(text[:start], "{")
	if brace < 0 {
		return
	}
	raw := extractJSONObject(text[brace:])
	if raw == "" {
		return
	}
	_ = m.TryOpenKnowledgePicker([]byte(raw))
}

func extractJSONObject(s string) string {
	depth := 0
	inString := false
	escape := false
	for i, r := range s {
		if inString {
			if escape {
				escape = false
				continue
			}
			if r == '\\' {
				escape = true
				continue
			}
			if r == '"' {
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return ""
}
