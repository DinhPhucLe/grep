package main

import (
	"encoding/json"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestHideCommandsKeepFileChanges(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true})
	m.threadID = "t"
	m.reduce(event("item/started", `{"threadId":"t","turnId":"1","item":{"id":"cmd","type":"commandExecution","command":"powershell.exe -Command Get-Content secret-path","status":"inProgress"}}`))
	m.reduce(event("item/completed", `{"threadId":"t","turnId":"1","item":{"id":"file","type":"fileChange","status":"completed","changes":[{"path":"main.go","kind":{"type":"update"},"diff":"patch contents"}]}}`))
	m.refresh()
	v := m.View()
	if strings.Contains(v, "powershell") || strings.Contains(v, "secret-path") {
		t.Fatal("raw command still visible")
	}
	if !strings.Contains(v, "main.go") || !strings.Contains(v, "Updated") {
		t.Fatalf("file change missing: %s", v)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != 1 {
		t.Fatal("hidden commands still take focus")
	}
}

func TestStreamingMarkdownAtNarrowWidths(t *testing.T) {
	for _, width := range []int{26, 80} {
		for _, done := range []bool{false, true} {
			m := newModel(nil, "w", uiOptions{NoColor: true})
			m.width = width
			i := &conversationItem{raw: "# Heading\n\nThis is **bold** and *italic*.", done: done}
			got := ansi.Strip(m.markdown(i))
			if strings.Contains(got, "**") || strings.Contains(got, "*italic*") || strings.Contains(got, "# Heading") {
				t.Fatalf("literal Markdown at width %d done %v: %s", width, done, got)
			}
			if !strings.Contains(got, "bold") {
				t.Fatal("lost content")
			}
		}
	}
}

func TestApprovalScrollbarMouse(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true})
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 24})
	p, _ := json.Marshal(map[string]string{"command": strings.Repeat("long", 50) + "THE-END"})
	m.Update(wireMessage{ID: json.RawMessage(`3`), Method: "item/commandExecution/requestApproval", Params: p})
	m.View()
	r := m.requests[0]
	m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: r.barWidth - 1, Y: r.barY})
	if r.horizontal == 0 || !strings.Contains(m.requestView(), "THE-END") {
		t.Fatal("scrollbar click did not reach right side")
	}
}

func TestFileApprovalIncludesProposedPaths(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true})
	m.threadID = "t"
	m.reduce(event("item/started", `{"threadId":"t","turnId":"1","item":{"id":"f","type":"fileChange","status":"inProgress","changes":[{"path":"hello.py","kind":{"type":"add"}}]}}`))
	m.Update(wireMessage{ID: json.RawMessage(`5`), Method: "item/fileChange/requestApproval", Params: json.RawMessage(`{"threadId":"t","turnId":"1","itemId":"f"}`)})
	if !strings.Contains(m.requestView(), "Create: hello.py") {
		t.Fatal("approval omitted affected file")
	}
	m.refresh()
	if strings.Contains(m.viewport.View(), "Created:") {
		t.Fatal("proposal falsely shown as applied")
	}
}

func TestApprovalHorizontalScrollAndBar(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true})
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 24})
	path := "C:/" + strings.Repeat("long-folder/", 12) + "target.txt"
	p, _ := json.Marshal(map[string]any{"command": "read " + path, "commandActions": []map[string]string{{"type": "read", "path": path}}})
	m.Update(wireMessage{ID: json.RawMessage(`1`), Method: "item/commandExecution/requestApproval", Params: p})
	before := m.requestView()
	if !strings.Contains(before, "◀") {
		t.Fatal("no horizontal scrollbar")
	}
	for n := 0; n < 30; n++ {
		m.Update(tea.KeyMsg{Type: tea.KeyRight})
	}
	after := m.requestView()
	if !strings.Contains(after, "target.txt") {
		t.Fatalf("cannot reach end of path: %s", after)
	}
	if !strings.Contains(after, "> ") {
		t.Fatal("selection marker scrolled away")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.requestView() == after {
		t.Fatal("left does not scroll")
	}
}
