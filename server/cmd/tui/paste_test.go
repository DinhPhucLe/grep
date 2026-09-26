package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"testing"
)

func TestEnterWhileClipboardPendingDoesNotSendPartialDraft(t *testing.T) {
	m := newModel(nil, "w", uiOptions{})
	m.connected = true
	m.draft.SetValue("First ")
	m.clipboard = &memoryClipboard{text: "word\r\nsecond line\r\n"}
	_, paste := m.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
	_, send := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if send != nil || m.busy || len(m.items) > 0 {
		t.Fatal("sent partial draft before clipboard arrived")
	}
	m.Update(paste())
	if m.draft.Value() != "First word\nsecond line\n" {
		t.Fatalf("lost pasted prefix: %q", m.draft.Value())
	}
	if m.busy || len(m.items) > 0 {
		t.Fatal("clipboard result submitted automatically")
	}
	_, send = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if send == nil || !m.busy || !m.evaluating || m.draft.Value() != "First word\nsecond line\n" {
		t.Fatal("explicit Enter did not evaluate full paste")
	}
	m.stopEvaluation()
}

func TestTerminalPasteTargetsComposerAndKeepsFirstWord(t *testing.T) {
	m := newModel(nil, "w", uiOptions{})
	m.connected = true
	m.focus = -2
	m.draft.Blur()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("First word\r\nSecond line\r\n"), Paste: true})
	if m.draft.Value() != "First word\nSecond line\n" || m.busy || len(m.items) > 0 {
		t.Fatalf("paste discarded or submitted: %q", m.draft.Value())
	}
}
