package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	knowledgeResultsMaxHits  = 8
	knowledgePanelMaxRows    = 8
	knowledgeOverlayMaxRows  = 6
	knowledgePreviewMaxRows  = 10
	knowledgePingFrames      = 12
	knowledgeHighlightFrames = 18
	knowledgeFocusResults    = -3
	knowledgeFocusLinks      = -4
	knowledgeFocusToggle     = -5
)

type knowledgeHit struct {
	ID        string
	Content   string
	Title     string
	Source    string
	Snippet   string
	Topics    []string
	Author    string
	Score     float64
	Connected bool
}

type knowledgeSearchPayload struct {
	Items []struct {
		ID      string   `json:"id"`
		Content string   `json:"content"`
		Topics  []string `json:"topics"`
		Properties map[string]string `json:"properties"`
		Authors []struct {
			UserID string `json:"userId"`
			Name   string `json:"name"`
		} `json:"authors"`
	} `json:"items"`
	Scores []float64 `json:"scores"`
}

type mcpToolCallItem struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Server    string          `json:"server"`
	Tool      string          `json:"tool"`
	Status    string          `json:"status"`
	Arguments json.RawMessage `json:"arguments"`
	Result    *struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"result"`
}

func parseKnowledgeSearchPayload(raw []byte) ([]knowledgeHit, bool) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, false
	}
	if _, hasItems := probe["items"]; !hasItems {
		return nil, false
	}
	var payload knowledgeSearchPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false
	}
	if len(payload.Items) == 0 {
		return nil, true
	}
	n := len(payload.Items)
	if n > knowledgeResultsMaxHits {
		n = knowledgeResultsMaxHits
	}
	hits := make([]knowledgeHit, 0, n)
	for i := 0; i < n; i++ {
		item := payload.Items[i]
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		author := ""
		if len(item.Authors) > 0 {
			author = item.Authors[0].Name
			if author == "" {
				author = item.Authors[0].UserID
			}
		}
		source := author
		if source == "" && item.Properties != nil {
			source = item.Properties["repo"]
		}
		if source == "" {
			source = "unknown"
		}
		title, snippet := splitTitleSnippet(content)
		hit := knowledgeHit{
			ID:      item.ID,
			Content: content,
			Title:   title,
			Source:  source,
			Snippet: snippet,
			Topics:  item.Topics,
			Author:  author,
		}
		if i < len(payload.Scores) {
			hit.Score = payload.Scores[i]
		}
		hits = append(hits, hit)
	}
	return hits, true
}

func splitTitleSnippet(content string) (title, snippet string) {
	lines := strings.Split(content, "\n")
	title = strings.TrimSpace(lines[0])
	if title == "" {
		title = truncateRunes(strings.ReplaceAll(content, "\n", " "), 48)
	}
	rest := strings.TrimSpace(strings.TrimPrefix(content, lines[0]))
	snippet = truncateRunes(strings.ReplaceAll(rest, "\n", " "), 72)
	return title, snippet
}

func extractKnowledgeSearchJSON(resultText string) []byte {
	text := strings.TrimSpace(resultText)
	if text == "" {
		return nil
	}
	if strings.HasPrefix(text, "{") {
		if raw := extractJSONObject(text); raw != "" {
			return []byte(raw)
		}
	}
	start := strings.Index(text, `"items"`)
	if start < 0 {
		return nil
	}
	brace := strings.LastIndex(text[:start], "{")
	if brace < 0 {
		return nil
	}
	raw := extractJSONObject(text[brace:])
	if raw == "" {
		return nil
	}
	return []byte(raw)
}

func (m *model) handleMcpToolCall(method string, rawItem json.RawMessage) {
	var item mcpToolCallItem
	if json.Unmarshal(rawItem, &item) != nil {
		return
	}
	if !strings.EqualFold(item.Tool, "knowledge_search") {
		return
	}
	completed := method == "item/completed" || item.Status == "completed" || item.Status == "failed"
	if !completed {
		m.beginKnowledgeSearch()
		return
	}
	if item.Status == "failed" {
		m.finishKnowledgeSearch(nil, true)
		return
	}
	var text string
	if item.Result != nil {
		for _, part := range item.Result.Content {
			if part.Type == "text" || part.Type == "" {
				text += part.Text
			}
		}
	}
	raw := extractKnowledgeSearchJSON(text)
	if raw == nil {
		m.finishKnowledgeSearch(nil, true)
		return
	}
	hits, ok := parseKnowledgeSearchPayload(raw)
	if !ok {
		m.finishKnowledgeSearch(nil, true)
		return
	}
	for i := range hits {
		if _, connected := m.knowledgeConnected[hits[i].ID]; connected {
			hits[i].Connected = true
		}
	}
	m.finishKnowledgeSearch(hits, false)
}

func (m *model) beginKnowledgeSearch() {
	m.knowledgeSearchGen++
	m.knowledgeSearching = true
	m.knowledgePanelOpen = true
	m.knowledgeSearchFailed = false
	m.knowledgeExpanded = false
	m.knowledgeHits = nil
	m.knowledgeSelected = 0
	m.knowledgeOffset = 0
	m.knowledgePingLeft = 0
	m.knowledgeHighlightLeft = 0
	m.closeKnowledgePreview()
	m.dirty = true
	m.resize()
}

func (m *model) finishKnowledgeSearch(hits []knowledgeHit, failed bool) {
	m.knowledgeSearching = false
	m.knowledgePanelOpen = true
	m.knowledgeHits = hits
	m.knowledgeSelected = 0
	m.knowledgeOffset = 0
	m.knowledgeExpanded = false
	m.knowledgeSearchFailed = failed
	m.closeKnowledgePreview()
	if failed {
		m.knowledgePingLeft = 0
		m.knowledgeHighlightLeft = 0
	} else if len(hits) > 0 {
		m.knowledgePingLeft = knowledgePingFrames
		m.knowledgeHighlightLeft = knowledgeHighlightFrames
		m.focus = knowledgeFocusToggle
		m.draft.Blur()
	} else {
		m.knowledgePingLeft = 0
		m.knowledgeHighlightLeft = 0
	}
	m.dirty = true
	m.resize()
}

func (m *model) knowledgeLinkCount() int {
	return len(m.knowledgeConnectedOrder)
}

func (m *model) useKnowledgeAt(idx int) bool {
	if idx < 0 || idx >= len(m.knowledgeHits) {
		return false
	}
	hit := m.knowledgeHits[idx]
	if strings.TrimSpace(hit.ID) == "" || strings.TrimSpace(hit.Content) == "" {
		return false
	}
	if m.knowledgeConnected == nil {
		m.knowledgeConnected = map[string]knowledgeHit{}
	}
	if hit.Connected || m.knowledgeConnected[hit.ID].ID != "" {
		m.unlinkKnowledge(hit.ID)
		return true
	}
	m.knowledgeConnected[hit.ID] = hit
	m.knowledgeConnectedOrder = append(m.knowledgeConnectedOrder, hit.ID)
	m.knowledgeHits[idx].Connected = true
	m.resize()
	m.pendingKnowledgeConnect = m.postKnowledgeConnect(hit.ID)
	return true
}

func (m *model) postKnowledgeConnect(documentID string) tea.Cmd {
	if !m.ensureAuth() {
		return nil
	}
	sessionID := strings.TrimSpace(m.threadID)
	if sessionID == "" || strings.TrimSpace(documentID) == "" {
		return nil
	}
	token := m.auth.Token
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = postJSON(ctx, cortisolServerURL()+"/api/v1/knowledge/connects", map[string]string{
			"documentId": documentID,
			"sessionId":  sessionID,
		}, token, nil)
		return nil
	}
}

func (m *model) unlinkKnowledge(id string) {
	if id == "" || m.knowledgeConnected == nil {
		return
	}
	delete(m.knowledgeConnected, id)
	out := m.knowledgeConnectedOrder[:0]
	for _, existing := range m.knowledgeConnectedOrder {
		if existing != id {
			out = append(out, existing)
		}
	}
	m.knowledgeConnectedOrder = out
	for i := range m.knowledgeHits {
		if m.knowledgeHits[i].ID == id {
			m.knowledgeHits[i].Connected = false
		}
	}
	if m.knowledgeLinkCount() == 0 {
		m.knowledgeLinksOverlay = false
	}
	m.resize()
}

func (m *model) promptWithConnectedKnowledge(text string) string {
	if len(m.knowledgeConnectedOrder) == 0 {
		return text
	}
	var b strings.Builder
	b.WriteString("Connected org knowledge for this session:\n")
	for _, id := range m.knowledgeConnectedOrder {
		hit, ok := m.knowledgeConnected[id]
		if !ok {
			continue
		}
		b.WriteString("\n---\n")
		if hit.Source != "" {
			b.WriteString("Source: ")
			b.WriteString(hit.Source)
			b.WriteString("\n")
		}
		b.WriteString(hit.Content)
		b.WriteString("\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(text)
	return b.String()
}

func (m *model) knowledgeResultsVisible() bool {
	return m.knowledgePanelOpen || m.knowledgeSearching
}

func (m *model) knowledgeToggleLabel() string {
	n := len(m.knowledgeHits)
	if m.knowledgeExpanded {
		return "Hide matches"
	}
	if n == 1 {
		return "1 match"
	}
	return fmt.Sprintf("%d matches", n)
}

func (m *model) knowledgeToggleLineHeight() int {
	if m.knowledgeSearching || m.knowledgeSearchFailed || !m.knowledgePanelOpen {
		return 0
	}
	if len(m.knowledgeHits) == 0 {
		return 0
	}
	return 1
}

func (m *model) knowledgeListHeight() int {
	if !m.knowledgeResultsVisible() {
		return 0
	}
	if m.knowledgeSearching {
		return 3
	}
	if m.knowledgeSearchFailed || len(m.knowledgeHits) == 0 {
		return 3
	}
	if !m.knowledgeExpanded {
		return 0
	}
	// Each hit: title (+ optional snippet) + blank separator; + border
	per := 2
	for _, h := range m.knowledgeHits {
		if strings.TrimSpace(h.Snippet) != "" {
			per = 3
			break
		}
	}
	body := min(knowledgePanelMaxRows, len(m.knowledgeHits)*per)
	return min(body+2, max(5, m.height/3))
}

func (m *model) knowledgePanelHeight() int {
	return m.knowledgeToggleLineHeight() + m.knowledgeListHeight()
}

func (m *model) knowledgeLinksLineHeight() int {
	if m.knowledgeLinkCount() == 0 && !m.knowledgeLinksOverlay {
		return 0
	}
	return 1
}

func (m *model) knowledgeOverlayHeight() int {
	if !m.knowledgeLinksOverlay {
		return 0
	}
	n := min(knowledgeOverlayMaxRows, max(1, m.knowledgeLinkCount()))
	return n + 2
}

func (m *model) knowledgePreviewHeight() int {
	if m.knowledgePreviewIdx < 0 {
		return 0
	}
	return min(knowledgePreviewMaxRows+2, max(6, m.height/3))
}

func (m *model) cloudConnectingView(width int) string {
	dots := strings.Repeat(".", (m.frame/8)%4)
	label := "Connecting to cloud" + dots
	cloud := "☁"
	if !m.opts.ReducedMotion {
		frames := []string{"☁", "☁ ", " ☁", "☁"}
		cloud = frames[(m.frame/6)%len(frames)]
	}
	if !m.opts.NoColor {
		return ansi.Truncate(
			lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Render(cloud)+" "+
				lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(label),
			width, "")
	}
	return ansi.Truncate(cloud+" "+label, width, "")
}

func (m *model) knowledgeToggleView(width int) string {
	if m.knowledgeToggleLineHeight() == 0 {
		return ""
	}
	label := m.knowledgeToggleLabel()
	if m.opts.NoColor {
		if m.focus == knowledgeFocusToggle {
			return ansi.Truncate("["+label+"]", width, "")
		}
		return ansi.Truncate(label, width, "")
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Underline(true)
	if m.focus == knowledgeFocusToggle {
		style = style.Bold(true)
	}
	return ansi.Truncate(style.Render(label), width, "")
}

func (m *model) toggleKnowledgeExpanded() {
	if len(m.knowledgeHits) == 0 {
		return
	}
	m.knowledgeExpanded = !m.knowledgeExpanded
	if !m.knowledgeExpanded {
		m.closeKnowledgePreview()
		m.focus = knowledgeFocusToggle
	} else {
		m.focus = knowledgeFocusResults
		m.clampKnowledgeSelection()
	}
	m.draft.Blur()
	m.resize()
}

func (m *model) openKnowledgePreview(idx int) {
	if idx < 0 || idx >= len(m.knowledgeHits) {
		return
	}
	m.knowledgePreviewIdx = idx
	m.knowledgePreviewOffset = 0
	m.knowledgeSelected = idx
	m.focus = knowledgeFocusResults
	m.draft.Blur()
	m.resize()
}

func (m *model) closeKnowledgePreview() {
	m.knowledgePreviewIdx = -1
	m.knowledgePreviewOffset = 0
}

func (m *model) knowledgeListView(width, height int) string {
	if width < 8 || height < 1 {
		return ""
	}
	innerW := max(1, width-2)
	var lines []string
	if m.knowledgeSearching {
		lines = append(lines, m.cloudConnectingView(innerW))
	} else if m.knowledgeSearchFailed {
		lines = append(lines, ansi.Truncate("Knowledge search failed", innerW, ""))
	} else if len(m.knowledgeHits) == 0 {
		lines = append(lines, ansi.Truncate("No knowledge matches", innerW, ""))
	} else {
		hint := "Space view · Enter use"
		if m.opts.NoColor {
			lines = append(lines, ansi.Truncate(strings.Repeat(" ", max(0, innerW-ansi.StringWidth(hint)))+hint, innerW, ""))
		} else {
			styled := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(hint)
			pad := max(0, innerW-ansi.StringWidth(hint))
			lines = append(lines, strings.Repeat(" ", pad)+styled)
		}
		m.clampKnowledgeSelection()
		// Rough visible hit count for scrolling (hint takes one line)
		visibleHits := max(1, (height-3)/2)
		if m.knowledgeSelected < m.knowledgeOffset {
			m.knowledgeOffset = m.knowledgeSelected
		}
		if m.knowledgeSelected >= m.knowledgeOffset+visibleHits {
			m.knowledgeOffset = m.knowledgeSelected - visibleHits + 1
		}
		end := min(len(m.knowledgeHits), m.knowledgeOffset+visibleHits+2)
		for i := m.knowledgeOffset; i < end; i++ {
			hit := m.knowledgeHits[i]
			marker := " "
			if i == m.knowledgeSelected {
				marker = ">"
			}
			useAction := "Use"
			if hit.Connected {
				useAction = "Connected"
			}
			actions := "View " + useAction
			title := truncateRunes(hit.Title, max(8, innerW/2))
			source := truncateRunes(hit.Source, max(4, innerW/5))
			row1 := fmt.Sprintf("%s %s  %s", marker, title, source)
			pad := max(0, innerW-ansi.StringWidth(row1)-ansi.StringWidth(actions)-1)
			row1 = ansi.Truncate(row1+strings.Repeat(" ", pad)+" "+actions, innerW, "")
			if i == m.knowledgeSelected && m.knowledgeHighlightLeft > 0 && !m.opts.NoColor {
				row1 = lipgloss.NewStyle().Background(lipgloss.Color("240")).Foreground(lipgloss.Color("15")).Render(row1)
			} else if m.knowledgeHighlightLeft > 0 && !m.opts.NoColor {
				row1 = lipgloss.NewStyle().Background(lipgloss.Color("236")).Render(row1)
			}
			lines = append(lines, row1)
			if snip := strings.TrimSpace(hit.Snippet); snip != "" {
				lines = append(lines, ansi.Truncate("  "+snip, innerW, ""))
			}
			lines = append(lines, "")
			if len(lines) >= height-2 {
				break
			}
		}
	}
	for len(lines) < max(0, height-2) {
		lines = append(lines, "")
	}
	if len(lines) > max(0, height-2) {
		lines = lines[:max(0, height-2)]
	}
	body := strings.Join(lines, "\n")
	borderColor := "8"
	if m.knowledgePingLeft > 0 && !m.opts.NoColor {
		borderColor = "15"
	}
	if m.focus == knowledgeFocusResults && !m.opts.NoColor {
		borderColor = "6"
	}
	if m.opts.NoColor {
		out := body
		for lipgloss.Height(out) < height {
			out += "\n"
		}
		return fitScreen(out, width, height)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		Width(max(1, width-2)).
		Height(max(1, height-2)).
		Render(body)
}

func (m *model) knowledgeResultsView(width, height int) string {
	if height < 1 || width < 1 {
		return ""
	}
	var parts []string
	toggleH := m.knowledgeToggleLineHeight()
	listH := m.knowledgeListHeight()
	if toggleH > 0 {
		parts = append(parts, m.knowledgeToggleView(width))
	}
	if listH > 0 {
		parts = append(parts, m.knowledgeListView(width, listH))
	}
	out := strings.Join(parts, "\n")
	for lipgloss.Height(out) < height {
		out += "\n"
	}
	return fitScreen(out, width, height)
}

func (m *model) knowledgePreviewView(width, height int) string {
	if m.knowledgePreviewIdx < 0 || m.knowledgePreviewIdx >= len(m.knowledgeHits) || width < 8 || height < 1 {
		return ""
	}
	hit := m.knowledgeHits[m.knowledgePreviewIdx]
	innerW := max(1, width-2)
	bodyLines := strings.Split(ansi.Wrap(safeText(hit.Content), innerW, ""), "\n")
	m.knowledgePreviewOffset = min(m.knowledgePreviewOffset, max(0, len(bodyLines)-1))
	visible := max(1, height-2)
	end := min(len(bodyLines), m.knowledgePreviewOffset+visible)
	lines := append([]string{}, bodyLines[m.knowledgePreviewOffset:end]...)
	for len(lines) < height-2 {
		lines = append(lines, "")
	}
	if len(lines) > height-2 {
		lines = lines[:height-2]
	}
	body := strings.Join(lines, "\n")
	if m.opts.NoColor {
		out := body
		for lipgloss.Height(out) < height {
			out += "\n"
		}
		return fitScreen(out, width, height)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("6")).
		Width(max(1, width-2)).
		Height(max(1, height-2)).
		Render(body)
}

func (m *model) clampKnowledgeSelection() {
	if len(m.knowledgeHits) == 0 {
		m.knowledgeSelected = 0
		return
	}
	if m.knowledgeSelected < 0 {
		m.knowledgeSelected = 0
	}
	if m.knowledgeSelected >= len(m.knowledgeHits) {
		m.knowledgeSelected = len(m.knowledgeHits) - 1
	}
}

func (m *model) knowledgeLinksLabel() string {
	n := m.knowledgeLinkCount()
	if n == 1 {
		return "1 new link"
	}
	return fmt.Sprintf("%d new links", n)
}

func (m *model) knowledgeLinksLineView(width int) string {
	if m.knowledgeLinksLineHeight() == 0 {
		return ""
	}
	label := m.knowledgeLinksLabel()
	if m.opts.NoColor {
		if m.focus == knowledgeFocusLinks || m.knowledgeLinksOverlay {
			return ansi.Truncate("["+label+"]", width, "")
		}
		return ansi.Truncate(label, width, "")
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Underline(true)
	if m.focus == knowledgeFocusLinks || m.knowledgeLinksOverlay {
		style = style.Bold(true)
	}
	return ansi.Truncate(style.Render(label), width, "")
}

func (m *model) knowledgeOverlayView(width, height int) string {
	if !m.knowledgeLinksOverlay || width < 8 || height < 1 {
		return ""
	}
	innerW := max(1, width-2)
	lines := []string{ansi.Truncate("Connected this session", innerW, "")}
	ids := m.knowledgeConnectedOrder
	if m.knowledgeLinksOffset > len(ids) {
		m.knowledgeLinksOffset = 0
	}
	visible := max(1, height-2)
	end := min(len(ids), m.knowledgeLinksOffset+visible)
	for i := m.knowledgeLinksOffset; i < end; i++ {
		hit := m.knowledgeConnected[ids[i]]
		row := truncateRunes(hit.Title, max(8, innerW*2/3)) + "  " + truncateRunes(hit.Source, max(4, innerW/3))
		lines = append(lines, ansi.Truncate(row, innerW, ""))
	}
	for len(lines) < height-2 {
		lines = append(lines, "")
	}
	if len(lines) > height-2 {
		lines = lines[:height-2]
	}
	body := strings.Join(lines, "\n")
	if m.opts.NoColor {
		return fitScreen(body, width, height)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("8")).
		Width(max(1, width-2)).
		Height(max(1, height-2)).
		Render(body)
}

func (m *model) openKnowledgeLinksOverlay() {
	if m.knowledgeLinkCount() == 0 {
		return
	}
	m.knowledgeLinksOverlay = true
	m.knowledgeLinksOffset = 0
	m.focus = knowledgeFocusLinks
	m.draft.Blur()
	m.resize()
}

func (m *model) closeKnowledgeLinksOverlay() {
	m.knowledgeLinksOverlay = false
	m.knowledgeLinksOffset = 0
	if len(m.requests) == 0 {
		m.focus = -1
		m.draft.Focus()
	}
	m.resize()
}

func (m *model) knowledgeKey(msg tea.KeyMsg) bool {
	if m.knowledgePreviewIdx >= 0 {
		switch msg.String() {
		case "esc":
			m.closeKnowledgePreview()
			m.focus = knowledgeFocusResults
			m.draft.Blur()
			m.resize()
			return true
		case "up", "k":
			m.knowledgePreviewOffset = max(0, m.knowledgePreviewOffset-1)
			return true
		case "down", "j":
			m.knowledgePreviewOffset++
			return true
		case "pgup":
			m.knowledgePreviewOffset = max(0, m.knowledgePreviewOffset-knowledgePreviewMaxRows)
			return true
		case "pgdown":
			m.knowledgePreviewOffset += knowledgePreviewMaxRows
			return true
		case "enter", " ":
			return true // do not Use while previewing
		}
		return true
	}
	if m.knowledgeLinksOverlay {
		switch msg.String() {
		case "esc":
			m.closeKnowledgeLinksOverlay()
			return true
		case "up", "k":
			m.knowledgeLinksOffset = max(0, m.knowledgeLinksOffset-1)
			return true
		case "down", "j":
			maxOff := max(0, m.knowledgeLinkCount()-1)
			if m.knowledgeLinksOffset < maxOff {
				m.knowledgeLinksOffset++
			}
			return true
		case "pgup":
			m.knowledgeLinksOffset = max(0, m.knowledgeLinksOffset-knowledgeOverlayMaxRows)
			return true
		case "pgdown":
			m.knowledgeLinksOffset = min(max(0, m.knowledgeLinkCount()-1), m.knowledgeLinksOffset+knowledgeOverlayMaxRows)
			return true
		case "enter":
			return true
		}
		return true
	}
	if m.focus == knowledgeFocusLinks {
		switch msg.String() {
		case "enter", " ":
			m.openKnowledgeLinksOverlay()
			return true
		case "esc":
			m.focus = -1
			m.draft.Focus()
			return true
		}
	}
	if m.focus == knowledgeFocusToggle && len(m.knowledgeHits) > 0 {
		switch msg.String() {
		case "enter", " ":
			m.toggleKnowledgeExpanded()
			return true
		case "esc":
			m.focus = -1
			m.draft.Focus()
			return true
		}
	}
	if m.focus != knowledgeFocusResults || !m.knowledgeResultsVisible() || m.knowledgeSearching || !m.knowledgeExpanded {
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
	case " ":
		fallthrough
	case "v":
		m.openKnowledgePreview(m.knowledgeSelected)
		return true
	case "enter":
		_ = m.useKnowledgeAt(m.knowledgeSelected)
		return true
	case "esc":
		m.knowledgeExpanded = false
		m.closeKnowledgePreview()
		m.focus = knowledgeFocusToggle
		m.resize()
		return true
	}
	return false
}

func (m *model) knowledgeMouse(v tea.MouseMsg) bool {
	if v.Action != tea.MouseActionPress {
		return false
	}
	top := m.headerRows() + m.viewport.Height
	if m.statusText() != "" {
		top++
	}
	y := v.Y

	toggleH := m.knowledgeToggleLineHeight()
	listH := m.knowledgeListHeight()
	previewH := m.knowledgePreviewHeight()
	overlayH := m.knowledgeOverlayHeight()
	linksH := m.knowledgeLinksLineHeight()

	toggleTop := top
	listTop := toggleTop + toggleH
	previewTop := listTop + listH
	overlayTop := previewTop + previewH
	linksTop := overlayTop + overlayH

	if v.Button == tea.MouseButtonWheelUp || v.Button == tea.MouseButtonWheelDown {
		delta := 1
		if v.Button == tea.MouseButtonWheelUp {
			delta = -1
		}
		if m.knowledgePreviewIdx >= 0 && y >= previewTop && y < previewTop+previewH {
			m.knowledgePreviewOffset = max(0, m.knowledgePreviewOffset+delta)
			return true
		}
		if m.knowledgeLinksOverlay && y >= overlayTop && y < linksTop {
			m.knowledgeLinksOffset = max(0, min(max(0, m.knowledgeLinkCount()-1), m.knowledgeLinksOffset+delta))
			return true
		}
		if m.knowledgeExpanded && listH > 0 && y >= listTop && y < listTop+listH {
			if delta < 0 && m.knowledgeSelected > 0 {
				m.knowledgeSelected--
			} else if delta > 0 && m.knowledgeSelected < len(m.knowledgeHits)-1 {
				m.knowledgeSelected++
			}
			m.focus = knowledgeFocusResults
			m.draft.Blur()
			return true
		}
		return false
	}
	if v.Button != tea.MouseButtonLeft {
		return false
	}
	if linksH > 0 && y == linksTop {
		if m.knowledgeLinksOverlay {
			m.closeKnowledgeLinksOverlay()
		} else {
			m.openKnowledgeLinksOverlay()
		}
		return true
	}
	if m.knowledgePreviewIdx >= 0 && y >= previewTop && y < previewTop+previewH {
		return true
	}
	if m.knowledgeLinksOverlay && y >= overlayTop && y < linksTop {
		return true
	}
	if toggleH > 0 && y == toggleTop {
		m.toggleKnowledgeExpanded()
		return true
	}
	if m.knowledgeExpanded && listH > 0 && y >= listTop && y < listTop+listH {
		innerY := y - listTop - 1
		if innerY < 0 {
			m.focus = knowledgeFocusResults
			m.draft.Blur()
			return true
		}
		// Approximate row index accounting for blank separators
		row := m.knowledgeOffset
		cursor := 0
		for i := m.knowledgeOffset; i < len(m.knowledgeHits); i++ {
			block := 2 // title + blank
			if strings.TrimSpace(m.knowledgeHits[i].Snippet) != "" {
				block = 3
			}
			if innerY >= cursor && innerY < cursor+block {
				row = i
				break
			}
			cursor += block
			row = i
		}
		if row >= 0 && row < len(m.knowledgeHits) {
			m.knowledgeSelected = row
			m.focus = knowledgeFocusResults
			m.draft.Blur()
			actionW := ansi.StringWidth("View Connected")
			if v.X >= m.width-actionW-2 {
				useStart := m.width - ansi.StringWidth("Connected") - 1
				if m.knowledgeHits[row].Connected {
					useStart = m.width - ansi.StringWidth("Connected") - 1
				} else {
					useStart = m.width - ansi.StringWidth("Use") - 1
				}
				viewStart := useStart - ansi.StringWidth("View ")
				if v.X >= useStart {
					_ = m.useKnowledgeAt(row)
				} else if v.X >= viewStart {
					m.openKnowledgePreview(row)
				}
			}
			return true
		}
		m.focus = knowledgeFocusResults
		m.draft.Blur()
		return true
	}
	return false
}

func (m *model) tickKnowledgeEffects() {
	if m.knowledgeSearching && !m.opts.ReducedMotion {
		m.dirty = true
	}
	if m.knowledgePingLeft > 0 {
		m.knowledgePingLeft--
		m.dirty = true
	}
	if m.knowledgeHighlightLeft > 0 {
		m.knowledgeHighlightLeft--
		m.dirty = true
	}
}

func truncateRunes(s string, maxN int) string {
	if maxN <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxN {
		return s
	}
	runes := []rune(s)
	if maxN == 1 {
		return "…"
	}
	return string(runes[:maxN-1]) + "…"
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
