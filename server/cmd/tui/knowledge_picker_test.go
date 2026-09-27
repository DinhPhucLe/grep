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

func TestKnowledgePickerOpensFromSearchJSON(t *testing.T) {
	m := newModel(nil, "workspace", uiOptions{NoIcons: true, NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.threadID = "t"
	m.status = "Ready"
	m.connected = true

	if !m.TryOpenKnowledgePicker([]byte(knowledgeFixtureJSON)) {
		t.Fatal("expected picker to open")
	}
	if !m.knowledgePickerOpen || len(m.knowledgeHits) != 5 {
		t.Fatalf("open=%v hits=%d", m.knowledgePickerOpen, len(m.knowledgeHits))
	}
	view := m.View()
	if !strings.Contains(view, "Alex Rivera") {
		t.Fatalf("missing author in view: %s", view)
	}
	if !strings.Contains(view, "exponential backoff") && !strings.Contains(view, "backoff") {
		t.Fatalf("missing hit content: %s", view)
	}
}

func TestKnowledgePickerSelectExpectedKeywordLeavesReady(t *testing.T) {
	m := newModel(nil, "workspace", uiOptions{NoIcons: true, NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.threadID = "t"
	m.connected = true
	m.busy = true
	m.status = "Working"
	if !m.TryOpenKnowledgePicker([]byte(knowledgeFixtureJSON)) {
		t.Fatal("open")
	}

	wantKeyword := "HMAC-SHA256"
	idx := -1
	for i, hit := range m.knowledgeHits {
		if strings.Contains(hit.Content, wantKeyword) {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("fixture missing keyword")
	}
	m.knowledgeSelected = idx
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.knowledgePickerOpen {
		t.Fatal("picker still open")
	}
	if m.selectedKnowledge == nil || !strings.Contains(m.selectedKnowledge.Content, wantKeyword) {
		t.Fatalf("selection %+v", m.selectedKnowledge)
	}

	m.reduce(event("turn/completed", `{"threadId":"t","turn":{"id":"1","status":"completed"}}`))
	if m.busy {
		t.Fatal("still busy")
	}
	if m.draft.Focused() == false && len(m.requests) == 0 {
		m.draft.Focus()
	}
	if !m.draft.Focused() {
		t.Fatal("composer not focused for continued use")
	}
}

func TestKnowledgePickerEscDismisses(t *testing.T) {
	m := newModel(nil, "workspace", uiOptions{NoIcons: true, NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.TryOpenKnowledgePicker([]byte(knowledgeFixtureJSON))
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.knowledgePickerOpen {
		t.Fatal("still open")
	}
	if m.selectedKnowledge != nil {
		t.Fatal("unexpected selection")
	}
}

func TestParseKnowledgeSearchPayloadIgnoresNoise(t *testing.T) {
	hits, ok := parseKnowledgeSearchPayload([]byte(`{"hello":"world"}`))
	if ok || hits != nil {
		t.Fatal("should reject")
	}
	_, err := json.Marshal(struct{}{})
	if err != nil {
		t.Fatal(err)
	}
}
