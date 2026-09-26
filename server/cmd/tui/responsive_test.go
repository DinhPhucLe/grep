package main

import (
	"encoding/json"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func assertFits(t *testing.T, m *model) {
	t.Helper()
	v := m.View()
	if strings.Count(v, "\n")+1 > m.height {
		t.Fatalf("too tall: %s", v)
	}
	for _, l := range strings.Split(v, "\n") {
		if ansi.StringWidth(l) > m.width {
			t.Fatalf("too wide: %q", l)
		}
	}
}

func TestShortTerminalKeepsConversationInputAndControls(t *testing.T) {
	for _, h := range []int{4, 6, 8, 10, 11, 12, 24} {
		for _, w := range []int{24, 40, 80} {
			m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
			m.status = "Ready"
			m.connected = true
			m.items = []*conversationItem{{kind: "agentMessage", raw: strings.Repeat("older answer\n", 20) + "LATEST", done: true}}
			m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			v := m.View()
			for _, s := range []string{"LATEST", "Ask Codex", "Enter"} {
				if !strings.Contains(v, s) {
					t.Fatalf("%dx%d missing %q: %s", w, h, s, v)
				}
			}
			assertFits(t, m)
		}
	}
}

func TestSmallTerminalCapsMultilineComposer(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true})
	m.status = "Ready"
	m.items = []*conversationItem{{kind: "userMessage", raw: "VISIBLE", done: true}}
	m.draft.SetValue(strings.Repeat("line\n", 10) + "draft")
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	v := m.View()
	if !strings.Contains(v, "VISIBLE") || !strings.Contains(v, "Enter") {
		t.Fatal(v)
	}
	assertFits(t, m)
}

func TestSmallApprovalKeepsChoiceAndDetailNavigation(t *testing.T) {
	for _, h := range []int{5, 6, 8, 10, 24} {
		m := newModel(nil, "w", uiOptions{NoColor: true})
		m.Update(tea.WindowSizeMsg{Width: 32, Height: h})
		p, _ := json.Marshal(map[string]string{"command": strings.Repeat("long ", 30), "reason": "Review this operation"})
		m.Update(wireMessage{ID: json.RawMessage(`1`), Method: "item/commandExecution/requestApproval", Params: p})
		v := m.View()
		if !strings.Contains(v, "Don't allow") || !strings.Contains(v, "Enter") {
			t.Fatalf("height %d: %s", h, v)
		}
		assertFits(t, m)
	}
}

func TestHelpAccessibleOnSmallScreenPreservesDraft(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true})
	m.Update(tea.WindowSizeMsg{Width: 28, Height: 6})
	m.draft.SetValue("my draft")
	m.Update(tea.KeyMsg{Type: tea.KeyF1})
	if !strings.Contains(m.View(), "Keyboard") {
		t.Fatal("help not visible")
	}
	found := false
	for n := 0; n < 30; n++ {
		if strings.Contains(m.View(), "Ctrl+C") {
			found = true
			break
		}
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
		assertFits(t, m)
	}
	if !found {
		t.Fatal("cannot scroll to all help")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.draft.Value() != "my draft" {
		t.Fatal("help changed draft")
	}
}

func TestResizeExtremesKeepLayoutWithinTerminal(t *testing.T) {
	m := newModel(nil, "workspace", uiOptions{NoColor: true})
	m.items = []*conversationItem{{kind: "userMessage", raw: "Earlier question", done: true}, {kind: "agentMessage", raw: "**Answer** with Unicode 👤 Việt", done: true}}
	for _, size := range []tea.WindowSizeMsg{{Width: 1, Height: 1}, {Width: 8, Height: 3}, {Width: 20, Height: 4}, {Width: 40, Height: 8}, {Width: 80, Height: 24}, {Width: 120, Height: 35}, {Width: 24, Height: 6}} {
		m.Update(size)
		assertFits(t, m)
	}
	if len(m.items) != 2 {
		t.Fatal("resize discarded history")
	}
	m.Update(wireMessage{ID: json.RawMessage(`3`), Method: "item/fileChange/requestApproval", Params: json.RawMessage(`{"reason":"Review this change"}`)})
	for _, size := range []tea.WindowSizeMsg{{Width: 1, Height: 1}, {Width: 8, Height: 3}, {Width: 24, Height: 6}, {Width: 80, Height: 24}} {
		m.Update(size)
		assertFits(t, m)
	}
}
