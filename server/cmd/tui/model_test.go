package main

import (
	"encoding/json"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func event(method, params string) wireMessage {
	return wireMessage{Method: method, Params: json.RawMessage(params)}
}

func TestStreamReconciliationAndTurnLifetime(t *testing.T) {
	m := newModel(nil, "workspace", uiOptions{NoIcons: true, ReducedMotion: true})
	m.threadID = "t"
	m.busy = true
	for _, e := range []wireMessage{
		event("item/agentMessage/delta", `{"threadId":"t","turnId":"1","itemId":"a","delta":"**Xin "}`),
		event("item/agentMessage/delta", `{"threadId":"t","turnId":"1","itemId":"b","delta":"Other"}`),
		event("item/agentMessage/delta", `{"threadId":"t","turnId":"1","itemId":"a","delta":"chào** 👤"}`),
		event("item/completed", `{"threadId":"t","turnId":"1","item":{"id":"a","type":"agentMessage","text":"**Xin chào** 👤"}}`),
	} {
		m.reduce(e)
	}
	if len(m.items) != 2 || m.items[0].raw != "**Xin chào** 👤" || m.items[1].raw != "Other" {
		t.Fatalf("mixed items: %+v", m.items)
	}
	if !m.busy {
		t.Fatal("item completion ended turn")
	}
	m.reduce(event("turn/completed", `{"threadId":"t","turn":{"id":"1","status":"failed","error":{"message":"oops"}}}`))
	if m.busy || !strings.Contains(m.status, "oops") {
		t.Fatal(m.status)
	}
}

func TestLayoutPasteAndDisconnect(t *testing.T) {
	m := newModel(nil, "workspace", uiOptions{NoIcons: true, ReducedMotion: true})
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 120, Height: 35}, {Width: 30, Height: 12}} {
		m.Update(size)
		if lines := strings.Count(m.View(), "\n") + 1; lines > size.Height {
			t.Fatalf("layout %v has %d lines", size, lines)
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("one\ntwo"), Paste: true})
	if m.draft.Value() != "one\ntwo" || m.busy {
		t.Fatal("paste submitted or lost")
	}
	m.Update(disconnectedMsg{err: strings.NewReader("")})
	if m.connected {
		t.Fatal("disconnect still connected")
	}
}

func TestLogKeepsIdenticalWireEvents(t *testing.T) {
	l := newSessionLogger()
	l.Record("in", map[string]string{"delta": "a"})
	l.Record("in", map[string]string{"delta": "a"})
	if len(l.buf) != 2 {
		t.Fatalf("lost repeated RPC: %d", len(l.buf))
	}
}

func TestSanitizeExternalContent(t *testing.T) {
	got := safeText("hello\x1b[2J\x1b]52;c;payload\a chào 👤\rbye")
	if strings.ContainsAny(got, "\x1b\a\r") || strings.Contains(got, "payload") {
		t.Fatalf("unsafe: %q", got)
	}
}

func TestStableFenceAndIncompleteTail(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true})
	i := &conversationItem{kind: "agentMessage", raw: "**Done**\n\n```go\nfmt.Println(1)\n```\n\n**unfinished"}
	got := m.markdown(i)
	if strings.Contains(got, "```go") || !strings.Contains(got, "**unfinished") {
		t.Fatalf("stable fence not rendered or tail changed: %q", got)
	}
	i.raw = "**Done**\n\n```go\nfmt.Println(1)\n```\n\n**finished**"
	i.done = true
	got = m.markdown(i)
	if strings.Contains(got, "**finished**") || strings.Count(got, "finished") != 1 {
		t.Fatal(got)
	}
}

func TestTinyApprovalKeepsActionVisible(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true})
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	m.Update(wireMessage{ID: json.RawMessage(`1`), Method: "item/fileChange/requestApproval", Params: json.RawMessage(`{"reason":"review a change"}`)})
	view := m.View()
	if !strings.Contains(view, "Don't allow") || !strings.Contains(view, "Enter") {
		t.Fatalf("controls clipped: %s", view)
	}
}

func TestCursorAtActiveAnswerAndUnicodeWidth(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true})
	m.width = 30
	m.busy = true
	m.frame = 1
	m.items = append(m.items, &conversationItem{kind: "agentMessage", raw: "Hello 👤"})
	m.refresh()
	if !strings.Contains(m.viewport.View(), "Hello 👤▌") {
		t.Fatal("cursor not at active answer")
	}
	m.opts.ReducedMotion = true
	m.refresh()
	if strings.Contains(m.viewport.View(), "▌") {
		t.Fatal("animated cursor in reduced motion")
	}
	m.items[0].raw = strings.Repeat("👤 e\u0301 Việt ", 12)
	m.refresh()
	for _, line := range strings.Split(m.View(), "\n") {
		if ansi.StringWidth(line) > 30 {
			t.Fatalf("wide line: %q", line)
		}
	}
}
