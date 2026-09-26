package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func historyModel() *model {
	m := newModel(nil, "workspace", uiOptions{NoColor: true, ReducedMotion: true})
	m.threadID = "t"
	m.items = []*conversationItem{
		{kind: "userMessage", raw: "First question", done: true},
		{kind: "agentMessage", raw: strings.Repeat("An earlier answer is still here.\n", 50), done: true},
		{kind: "userMessage", raw: "Second question", done: true},
		{kind: "agentMessage", raw: strings.Repeat("The latest answer keeps growing.\n", 50)},
	}
	m.refresh()
	return m
}

func TestWheelScrollPausesFollowingAndReturnsToBottom(t *testing.T) {
	m := historyModel()
	m.draft.SetValue("keep my draft")
	bottom := m.viewport.YOffset
	wheelUp := tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress, X: 10, Y: 5}
	wheelDown := tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress, X: 10, Y: 5}
	m.Update(wheelUp)
	if m.viewport.YOffset >= bottom || m.follow {
		t.Fatal("wheel did not scroll up and pause following")
	}
	reading := m.viewport.YOffset
	m.items[3].raw += "New streamed text\n"
	m.dirty = true
	m.newOutput = true
	m.Update(frameMsg{})
	if m.viewport.YOffset != reading {
		t.Fatal("streaming pulled history back down")
	}
	for n := 0; n < 100; n++ {
		m.Update(wheelUp)
	}
	if !strings.Contains(m.viewport.View(), "First question") {
		t.Fatal("earlier conversation is inaccessible")
	}
	for n := 0; n < 100; n++ {
		m.Update(wheelDown)
	}
	if !m.follow || m.newOutput || !m.viewport.AtBottom() {
		t.Fatal("scrolling back down did not resume following")
	}
	if m.draft.Value() != "keep my draft" {
		t.Fatal("scrolling changed draft")
	}
}

func TestHistoryArrowKeysAndGlobalLatest(t *testing.T) {
	m := historyModel()
	m.draft.SetValue("draft")
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	bottom := m.viewport.YOffset
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.viewport.YOffset != bottom-1 || m.follow {
		t.Fatal("history up arrow did not scroll")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if !m.follow {
		t.Fatal("history down arrow did not reach latest")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlHome})
	if !m.viewport.AtTop() {
		t.Fatal("Ctrl+Home did not reach first message")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab}) // back to composer
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlEnd})
	if !m.viewport.AtBottom() || !m.follow || m.draft.Value() != "draft" {
		t.Fatal("Ctrl+End did not return to latest safely")
	}
}

func TestResizeWrapsWholeWordsInStreamingAndCompletedAnswers(t *testing.T) {
	text := "And then, one evening, you make dinner while listening to a song."
	for _, done := range []bool{false, true} {
		m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
		i := &conversationItem{kind: "agentMessage", raw: text, done: done}
		m.items = []*conversationItem{i}
		for _, width := range []int{80, 26, 40, 42, 60, 32, 80} {
			m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
			rendered := ansi.Strip(m.markdown(i))
			if strings.Join(strings.Fields(rendered), " ") != text {
				t.Fatalf("width %d done=%v splits words: %q", width, done, rendered)
			}
			for _, line := range strings.Split(rendered, "\n") {
				if ansi.StringWidth(line) > width-2 {
					t.Fatalf("line too wide: %q", line)
				}
			}
			if i.raw != text {
				t.Fatal("resize changed source")
			}
		}
	}
}

func TestPromptWrapsWordsAndLongTokensStillFit(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 26, Height: 24})
	text := "And then, one evening, you make dinner"
	m.items = []*conversationItem{{kind: "userMessage", raw: text, done: true}}
	m.refresh()
	if !strings.Contains(m.viewport.View(), "evening,") {
		t.Fatal("user prompt splits evening")
	}
	i := &conversationItem{kind: "agentMessage", raw: strings.Repeat("x", 120) + " 👤 Việt e\u0301"}
	for _, line := range strings.Split(m.markdown(i), "\n") {
		if ansi.StringWidth(line) > 24 {
			t.Fatalf("long token overflow: %q", line)
		}
	}
}
