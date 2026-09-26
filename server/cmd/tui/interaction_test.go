package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	tea "github.com/charmbracelet/bubbletea"
	"io"
	"strings"
	"testing"
	"time"
)

type bufferCloser struct{ bytes.Buffer }

func (b *bufferCloser) Close() error { return nil }

func TestApprovalReplyOncePreservesDraftAndReader(t *testing.T) {
	out := &bufferCloser{}
	c := &appServer{stdin: out, events: newEventQueue(), pending: map[int64]chan rpcResponse{}}
	m := newModel(c, "workspace", uiOptions{ReducedMotion: true})
	m.threadID = "t"
	m.draft.SetValue("unfinished prompt")
	req := wireMessage{ID: json.RawMessage(`"opaque-123"`), Method: "item/commandExecution/requestApproval", Params: json.RawMessage(`{"threadId":"t","command":"go test ./..."}`)}
	m.Update(req)
	m.Update(req)
	m.Update(event("item/agentMessage/delta", `{"threadId":"t","turnId":"1","itemId":"a","delta":"still receiving"}`))
	if len(m.requests) != 1 || m.items[0].raw != "still receiving" {
		t.Fatal("request blocked events or duplicated")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	cmd()
	m.Update(req)
	var reply struct {
		ID     string
		Result struct{ Decision string }
	}
	if err := json.Unmarshal(out.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	if reply.ID != "opaque-123" || reply.Result.Decision != "decline" || len(m.requests) != 0 || m.draft.Value() != "unfinished prompt" {
		t.Fatalf("bad reply/draft: %s", out.String())
	}
}

func TestInputQuestionAnswerShape(t *testing.T) {
	out := &bufferCloser{}
	c := &appServer{stdin: out}
	m := newModel(c, "w", uiOptions{})
	m.Update(wireMessage{ID: json.RawMessage(`9007199254740993`), Method: "item/tool/requestUserInput", Params: json.RawMessage(`{"questions":[{"id":"q1","question":"Choose","options":[{"label":"One","description":"first"}]},{"id":"q2","question":"Explain","options":null}]}`)})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("free answer"), Paste: true})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	cmd()
	if !strings.Contains(out.String(), `"id":9007199254740993`) || !strings.Contains(out.String(), `"q2":{"answers":["free answer"]}`) {
		t.Fatal(out.String())
	}
}

func TestHistoryFollowAndLargeStream(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.threadID = "t"
	raw := strings.Repeat("Xin chào 👤\n", 8000)
	p, _ := json.Marshal(map[string]string{"threadId": "t", "turnId": "1", "itemId": "a", "delta": raw})
	m.reduce(wireMessage{Method: "item/agentMessage/delta", Params: p})
	m.refresh()
	m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	offset := m.viewport.YOffset
	m.reduce(event("item/agentMessage/delta", `{"threadId":"t","turnId":"1","itemId":"a","delta":"new"}`))
	m.refresh()
	if m.viewport.YOffset != offset || !m.newOutput {
		t.Fatal("scroll position lost")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if !m.follow || m.newOutput {
		t.Fatal("cannot return to latest without a tool card")
	}
	if m.items[0].raw != raw+"new" {
		t.Fatal("large stream lost bytes")
	}
}

func TestResolvedRequestDismissesPanel(t *testing.T) {
	m := newModel(nil, "w", uiOptions{})
	m.threadID = "t"
	m.Update(wireMessage{ID: json.RawMessage(`7`), Method: "item/fileChange/requestApproval", Params: json.RawMessage(`{}`)})
	m.Update(event("serverRequest/resolved", `{"threadId":"t","requestId":7}`))
	if len(m.requests) != 0 {
		t.Fatal("resolved request still blocks composer")
	}
}

func TestReaderDoesNotWaitForApproval(t *testing.T) {
	inR, inW := io.Pipe()
	defer inR.Close()
	c := &appServer{stdout: bufio.NewReader(inR), events: newEventQueue(), pending: map[int64]chan rpcResponse{}}
	go c.readLoop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		io.WriteString(inW, "{\"id\":\"approval\",\"method\":\"item/fileChange/requestApproval\",\"params\":{}}\n{\"method\":\"item/agentMessage/delta\",\"params\":{}}\n")
		inW.Close()
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reader blocked on approval")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	count := 0
	for count < 2 {
		batch, ok := c.events.next(ctx).(rpcBatch)
		if !ok {
			t.Fatal("lost events")
		}
		for _, v := range batch {
			if _, ok := v.(wireMessage); ok {
				count++
			}
		}
	}
}

func TestReadyAfterDisconnectStaysOffline(t *testing.T) {
	m := newModel(nil, "w", uiOptions{})
	m.Update(disconnectedMsg{err: io.EOF})
	m.Update(readyMsg{thread: "t"})
	if m.connected {
		t.Fatal("late handshake resurrected connection")
	}
}

func TestComposerResizeKeepsFollowing(t *testing.T) {
	m := newModel(nil, "w", uiOptions{})
	m.items = append(m.items, &conversationItem{kind: "userMessage", raw: strings.Repeat("line\n", 100), done: true})
	m.refresh()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("one\ntwo\nthree\nfour"), Paste: true})
	if !m.viewport.AtBottom() {
		t.Fatal("composer growth hid latest output")
	}
}

func TestLongQuestionCanBeRead(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true})
	m.width = 40
	p, _ := json.Marshal(map[string]any{"questions": []map[string]any{{"id": "q", "question": strings.Repeat("details ", 30) + "ENDQUESTION", "options": nil}}})
	m.Update(wireMessage{ID: json.RawMessage(`1`), Method: "item/tool/requestUserInput", Params: p})
	found := false
	for n := 0; n < 20; n++ {
		if strings.Contains(m.requestView(), "ENDQUESTION") {
			found = true
			break
		}
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	}
	if !found {
		t.Fatal("question tail inaccessible")
	}
}

func BenchmarkSmallDeltas100KB(b *testing.B) {
	delta := event("item/agentMessage/delta", `{"threadId":"t","turnId":"1","itemId":"a","delta":"abcdefghij"}`)
	for n := 0; n < b.N; n++ {
		m := newModel(nil, "w", uiOptions{ReducedMotion: true})
		m.threadID = "t"
		for j := 0; j < 10000; j++ {
			m.reduce(delta)
		}
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("draft")})
		if m.draft.Value() != "draft" {
			b.Fatal("input lost")
		}
	}
}

func TestQueueYieldsBoundedBatches(t *testing.T) {
	q := newEventQueue()
	for n := 0; n < 1000; n++ {
		q.push(n)
	}
	batch := q.next(context.Background()).(rpcBatch)
	if len(batch) > 256 {
		t.Fatalf("%d events monopolize UI update", len(batch))
	}
	count := len(batch)
	for count < 1000 {
		count += len(q.next(context.Background()).(rpcBatch))
	}
	if count != 1000 {
		t.Fatal("lost events")
	}
}

func TestTabRevealsOffscreenActivity(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true})
	m.items = []*conversationItem{{kind: "fileChange", raw: `[{"path":"main.go","kind":{"type":"update"}}]`, status: "completed", done: true}, {kind: "agentMessage", raw: strings.Repeat("long answer\n", 100), done: true}}
	m.refresh()
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !strings.Contains(m.viewport.View(), "main.go") {
		t.Fatal("focused card is offscreen")
	}
}
