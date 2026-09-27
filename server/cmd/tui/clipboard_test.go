package main

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"
)

type failingClipboard struct{}

func (failingClipboard) ReadAll() (string, error) { return "", errors.New("clipboard busy") }
func (failingClipboard) WriteAll(string) error    { return errors.New("clipboard busy") }

func TestClipboardErrorKeepsDraftAndDoesNotClaimSuccess(t *testing.T) {
	m := newModel(nil, "w", uiOptions{})
	m.clipboard = failingClipboard{}
	m.draft.SetValue("keep this")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
	m.Update(cmd())
	if m.draft.Value() != "keep this" || !strings.Contains(m.clipboardNotice, "clipboard busy") {
		t.Fatal("paste error lost draft or was hidden")
	}
	m.selection = textSelection{lines: []string{"selected"}, start: cellPoint{0, 0}, end: cellPoint{7, 0}, selected: true}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m.Update(cmd())
	if strings.Contains(m.clipboardNotice, "Copied") {
		t.Fatal("false copy success")
	}
}

func TestReverseMultilineSelectionExcludesStyling(t *testing.T) {
	m := newModel(nil, "w", uiOptions{})
	m.selection = textSelection{lines: []string{"\x1b[1mHello world\x1b[0m   ", "second line   "}, start: cellPoint{5, 1}, end: cellPoint{6, 0}, selected: true}
	if got := m.selectedText(); got != "world\nsecond" {
		t.Fatalf("bad selection: %q", got)
	}
}

type memoryClipboard struct{ text string }

func (c *memoryClipboard) ReadAll() (string, error) { return c.text, nil }
func (c *memoryClipboard) WriteAll(s string) error  { c.text = s; return nil }

func TestCopySelectionDoesNotQuit(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.status = "Ready"
	cb := &memoryClipboard{}
	m.clipboard = cb
	m.items = []*conversationItem{{kind: "userMessage", raw: "Hello 👤 world", done: true}}
	m.refresh()
	m.View()
	y := m.headerRows() + 1 // padded user message body
	m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 0, Y: y})
	m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion, X: 7, Y: y})
	m.Update(tea.MouseMsg{Action: tea.MouseActionRelease, X: 7, Y: y})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("copy command missing")
	}
	msg := cmd()
	if _, quit := msg.(tea.QuitMsg); quit {
		t.Fatal("Ctrl+C quit")
	}
	m.Update(msg)
	if cb.text != "Hello 👤" {
		t.Fatalf("clipboard got %q", cb.text)
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("Ctrl+C without selection quit")
		}
	}
}

func TestClipboardPastePreservesMultilineDraft(t *testing.T) {
	m := newModel(nil, "w", uiOptions{})
	m.clipboard = &memoryClipboard{text: "one\r\ntwo\n/quit"}
	m.connected = true
	for _, key := range []tea.KeyType{tea.KeyCtrlV, tea.KeyInsert} {
		m.draft.Reset()
		_, cmd := m.Update(tea.KeyMsg{Type: key})
		if cmd == nil {
			t.Fatal("paste command missing")
		}
		m.Update(cmd())
		if m.draft.Value() != "one\ntwo\n/quit" || m.busy || len(m.items) > 0 {
			t.Fatal("paste lost text or submitted")
		}
	}
}

func TestQuitWhileApprovalOpen(t *testing.T) {
	m := newModel(nil, "w", uiOptions{})
	m.enqueueRequest(eventWithID())
	for _, r := range "/quit" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("no quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("/quit did not exit approval")
	}
}

func eventWithID() wireMessage {
	v := event("item/fileChange/requestApproval", `{}`)
	v.ID = []byte(`1`)
	return v
}

func TestSelectionStaysStableDuringStreaming(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true})
	m.status = "Ready"
	m.items = []*conversationItem{{kind: "userMessage", raw: "Original text", done: true}}
	m.refresh()
	y := m.headerRows() + 1
	m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 0, Y: y})
	m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion, X: 7, Y: y})
	m.items[0].raw = "Changed text"
	m.refresh()
	if !strings.Contains(m.View(), "Original") {
		t.Fatal("selection moved under streaming output")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !strings.Contains(m.View(), "Changed") {
		t.Fatal("Esc did not clear selection")
	}
}
