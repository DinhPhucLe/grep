package main

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

const knowledgeFixtureJSON = `{
  "items": [
    {
      "id": "1",
      "content": "Use exponential backoff with jitter on 429/503. Cap at five attempts.",
      "topics": ["payments", "retries"],
      "properties": {"repo": "novapay/novapay-api"},
      "authors": [{"userId": "alexr", "name": "Alex Rivera"}],
      "createdAt": "2025-01-01T00:00:00Z",
      "updatedAt": "2025-01-01T00:00:00Z",
      "organizationId": "org-novapay"
    },
    {
      "id": "2",
      "content": "Verify HMAC-SHA256 over the raw request body with the endpoint secret.",
      "topics": ["webhooks", "security"],
      "properties": {},
      "authors": [{"userId": "jordank", "name": "Jordan Kim"}],
      "createdAt": "2025-01-02T00:00:00Z",
      "updatedAt": "2025-01-02T00:00:00Z",
      "organizationId": "org-novapay"
    },
    {
      "id": "3",
      "content": "Always filter by organizationId before name match for FHIR patient search.",
      "topics": ["fhir", "tenancy"],
      "properties": {},
      "authors": [{"userId": "caseyn", "name": "Casey Nguyen"}],
      "createdAt": "2025-01-03T00:00:00Z",
      "updatedAt": "2025-01-03T00:00:00Z",
      "organizationId": "org-atlas"
    },
    {
      "id": "4",
      "content": "Idempotency keys must be unique per merchant for payment intents.",
      "topics": ["payments"],
      "properties": {},
      "authors": [{"userId": "rileyp", "name": "Riley Park"}],
      "createdAt": "2025-01-04T00:00:00Z",
      "updatedAt": "2025-01-04T00:00:00Z",
      "organizationId": "org-novapay"
    },
    {
      "id": "5",
      "content": "Circuit breakers open after five consecutive processor timeouts.",
      "topics": ["reliability"],
      "properties": {},
      "authors": [{"userId": "samok", "name": "Sam Okonkwo"}],
      "createdAt": "2025-01-05T00:00:00Z",
      "updatedAt": "2025-01-05T00:00:00Z",
      "organizationId": "org-novapay"
    }
  ],
  "scores": [0.95, 0.88, 0.8, 0.7, 0.65]
}`

func knowledgeSearchEvent(method, status, resultJSON string) wireMessage {
	item := map[string]any{
		"id":     "mcp-1",
		"type":   "mcpToolCall",
		"server": "cortisol",
		"tool":   "knowledge_search",
		"status": status,
		"arguments": map[string]any{
			"query": "payment retries",
		},
	}
	if resultJSON != "" {
		item["result"] = map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": resultJSON},
			},
		}
	}
	params, _ := json.Marshal(map[string]any{
		"threadId": "t",
		"turnId":   "1",
		"item":     item,
	})
	return wireMessage{Method: method, Params: params}
}

func TestKnowledgeSearchShowsConnectingThenResults(t *testing.T) {
	m := newModel(nil, "workspace", uiOptions{NoIcons: true, NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.threadID = "t"
	m.status = "Ready"
	m.connected = true

	m.reduce(knowledgeSearchEvent("item/started", "inProgress", ""))
	if !m.knowledgeSearching {
		t.Fatal("expected searching")
	}
	view := m.View()
	if !strings.Contains(view, "Connecting to cloud") {
		t.Fatalf("missing connecting: %s", view)
	}
	if strings.Contains(view, "Use") {
		t.Fatal("Use shown before results")
	}

	m.reduce(knowledgeSearchEvent("item/completed", "completed", knowledgeFixtureJSON))
	if m.knowledgeSearching {
		t.Fatal("still searching")
	}
	if len(m.knowledgeHits) != 5 {
		t.Fatalf("hits=%d", len(m.knowledgeHits))
	}
	if m.knowledgeExpanded {
		t.Fatal("should start collapsed")
	}
	view = m.View()
	if !strings.Contains(view, "5 matches") {
		t.Fatalf("missing toggle: %s", view)
	}
	if strings.Contains(view, "View") && strings.Contains(view, "Alex Rivera") {
		// expanded list should not show until toggled
		t.Fatalf("results visible while collapsed: %s", view)
	}
}

func TestKnowledgeSearchFastCompleteWithoutStarted(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.threadID = "t"
	m.reduce(knowledgeSearchEvent("item/completed", "completed", knowledgeFixtureJSON))
	if m.knowledgeSearching || len(m.knowledgeHits) != 5 {
		t.Fatalf("searching=%v hits=%d", m.knowledgeSearching, len(m.knowledgeHits))
	}
}

func TestKnowledgeSearchEmptyAndFailed(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.threadID = "t"

	m.reduce(knowledgeSearchEvent("item/started", "inProgress", ""))
	m.reduce(knowledgeSearchEvent("item/completed", "completed", `{"items":[],"scores":[]}`))
	if m.knowledgeSearching || len(m.knowledgeHits) != 0 {
		t.Fatal("empty search")
	}
	if m.knowledgePingLeft != 0 {
		t.Fatal("empty should not ping")
	}
	if !strings.Contains(m.View(), "No knowledge matches") {
		t.Fatalf("view=%s", m.View())
	}

	m.reduce(knowledgeSearchEvent("item/started", "inProgress", ""))
	m.reduce(knowledgeSearchEvent("item/completed", "failed", ""))
	if m.knowledgeSearching || len(m.knowledgeHits) != 0 {
		t.Fatal("failed search")
	}
	if m.knowledgePingLeft != 0 {
		t.Fatal("failed should not ping")
	}
}

func TestKnowledgeUseConnectsOnceAndPrependsPrompt(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.threadID = "t"
	m.connected = true
	m.reduce(knowledgeSearchEvent("item/completed", "completed", knowledgeFixtureJSON))

	m.knowledgeExpanded = true
	m.resize()
	if !m.useKnowledgeAt(0) {
		t.Fatal("use failed")
	}
	if !m.knowledgeHits[0].Connected || m.knowledgeLinkCount() != 1 {
		t.Fatal("not connected")
	}
	if !m.useKnowledgeAt(0) {
		t.Fatal("toggle unlink should succeed")
	}
	if m.knowledgeHits[0].Connected || m.knowledgeLinkCount() != 0 {
		t.Fatal("expected unlink on second activation")
	}
	if !m.useKnowledgeAt(0) {
		t.Fatal("re-use failed")
	}
	if m.knowledgeLinkCount() != 1 || !m.knowledgeHits[0].Connected {
		t.Fatal("re-connect failed")
	}
	view := m.View()
	if !strings.Contains(view, "Connected") || !strings.Contains(view, "1 new link") {
		t.Fatalf("view=%s", view)
	}

	// failed use: empty id
	m.knowledgeHits = append(m.knowledgeHits, knowledgeHit{ID: "", Content: "x", Title: "x", Source: "s"})
	idx := len(m.knowledgeHits) - 1
	if m.useKnowledgeAt(idx) {
		t.Fatal("empty id should fail")
	}
	if m.knowledgeLinkCount() != 1 {
		t.Fatal("failed use changed count")
	}

	got := m.promptWithConnectedKnowledge("continue please")
	if !strings.Contains(got, "Connected org knowledge") || !strings.Contains(got, "exponential backoff") || !strings.HasSuffix(strings.TrimSpace(got), "continue please") {
		t.Fatalf("prepend=%s", got)
	}
}

func TestKnowledgeLinksOverlaySessionOnly(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.threadID = "t"
	m.reduce(knowledgeSearchEvent("item/completed", "completed", knowledgeFixtureJSON))
	_ = m.useKnowledgeAt(0)
	_ = m.useKnowledgeAt(1)
	if m.knowledgeLinkCount() != 2 {
		t.Fatal(m.knowledgeLinkCount())
	}
	m.openKnowledgeLinksOverlay()
	if !m.knowledgeLinksOverlay {
		t.Fatal("overlay closed")
	}
	view := m.View()
	if !strings.Contains(view, "Connected this session") {
		t.Fatalf("%s", view)
	}
	if !strings.Contains(view, "2 new links") {
		t.Fatalf("%s", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.knowledgeLinksOverlay {
		t.Fatal("esc did not close")
	}
	if !m.draft.Focused() {
		t.Fatal("composer not restored")
	}
}

func TestKnowledgeStaleSearchDoesNotRestart(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: false})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.threadID = "t"
	m.reduce(knowledgeSearchEvent("item/started", "inProgress", ""))
	gen := m.knowledgeSearchGen
	m.reduce(knowledgeSearchEvent("item/completed", "completed", knowledgeFixtureJSON))
	if m.knowledgeSearching {
		t.Fatal("searching")
	}
	m.Update(frameMsg{})
	m.Update(frameMsg{})
	if m.knowledgeSearching || m.knowledgeSearchGen != gen {
		t.Fatal("frame restarted search")
	}
}

func TestKnowledgeCollapseToggleAndViewVsUse(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	m.threadID = "t"
	m.status = "Ready"
	m.reduce(knowledgeSearchEvent("item/completed", "completed", knowledgeFixtureJSON))
	if m.knowledgeExpanded {
		t.Fatal("expected collapsed")
	}
	m.focus = knowledgeFocusToggle
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.knowledgeExpanded {
		t.Fatal("enter did not expand")
	}
	view := m.View()
	if !strings.Contains(view, "Hide matches") || !strings.Contains(view, "View") {
		t.Fatalf("expanded view: %s", view)
	}

	m.focus = knowledgeFocusResults
	m.knowledgeSelected = 0
	before := m.knowledgeLinkCount()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if m.knowledgePreviewIdx != 0 {
		t.Fatal("view did not open")
	}
	if m.knowledgeLinkCount() != before {
		t.Fatal("view must not create a link")
	}
	view = m.View()
	if !strings.Contains(view, "exponential backoff") {
		t.Fatalf("preview missing body: %s", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.knowledgePreviewIdx >= 0 {
		t.Fatal("esc did not close preview")
	}
	if m.knowledgeLinkCount() != before {
		t.Fatal("close preview changed links")
	}

	m.focus = knowledgeFocusResults
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.knowledgeLinkCount() != before+1 || !m.knowledgeHits[0].Connected {
		t.Fatal("enter did not use")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.knowledgeLinkCount() != before || m.knowledgeHits[0].Connected {
		t.Fatal("enter on Connected did not unlink")
	}

	m.focus = knowledgeFocusToggle
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.knowledgeExpanded {
		t.Fatal("enter did not collapse")
	}
	if !strings.Contains(m.View(), "5 matches") {
		t.Fatalf("collapsed label: %s", m.View())
	}
}

func TestKnowledgeResizeKeepsLayout(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.threadID = "t"
	m.status = "Ready"
	m.reduce(knowledgeSearchEvent("item/completed", "completed", knowledgeFixtureJSON))
	_ = m.useKnowledgeAt(0)
	m.knowledgeExpanded = true
	m.openKnowledgePreview(0)
	m.openKnowledgeLinksOverlay()
	for _, size := range []tea.WindowSizeMsg{{Width: 40, Height: 12}, {Width: 80, Height: 24}, {Width: 24, Height: 8}} {
		m.Update(size)
		v := m.View()
		if strings.Count(v, "\n")+1 > m.height {
			t.Fatalf("too tall at %dx%d: %s", size.Width, size.Height, v)
		}
	}
}

func TestParseKnowledgeSearchPayloadIgnoresNoise(t *testing.T) {
	hits, ok := parseKnowledgeSearchPayload([]byte(`{"hello":"world"}`))
	if ok {
		t.Fatalf("should reject non-items payload, hits=%v", hits)
	}
}

func TestRepeatedSearchResultsReuseConnectedState(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.threadID = "t"
	m.reduce(knowledgeSearchEvent("item/completed", "completed", knowledgeFixtureJSON))
	_ = m.useKnowledgeAt(0)
	m.reduce(knowledgeSearchEvent("item/completed", "completed", knowledgeFixtureJSON))
	if !m.knowledgeHits[0].Connected {
		t.Fatal("connected state lost on repeat search")
	}
	if m.knowledgeLinkCount() != 1 {
		t.Fatal("duplicate link created")
	}
}
