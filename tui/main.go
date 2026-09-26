// Minimal Codex app-server chat client.
// Spawns `codex app-server`, renders line-oriented chat history, and
// forwards user text / agent messages with interactive approvals.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const approvalPrompt = "approve? [y/N] "

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cwd, err := os.Getwd()
	if err != nil {
		fatal(err)
	}

	stdin := newStdinReader()
	client, err := startAppServer(ctx, stdin)
	if err != nil {
		fatal(err)
	}
	defer client.Close()

	// Ctrl+C must unblock stdin reads and not hang on a pending approval.
	go func() {
		<-ctx.Done()
		fmt.Fprintln(os.Stdout, "\ntui: interrupt — exiting")
		_ = os.Stdin.Close()
		client.Close()
		os.Exit(130)
	}()

	if err := client.Initialize(ctx); err != nil {
		fatal(err)
	}

	threadID, err := client.StartThread(ctx, cwd)
	if err != nil {
		fatal(err)
	}

	ui := newChatUI()
	ui.println("cortisol test tui — connected to codex app-server")
	ui.println("cwd: " + cwd)
	ui.println("thread: " + threadID)
	ui.println("approvals: untrusted (type y or n when prompted; q declines)")
	ui.println("type a message and press enter; /quit or Ctrl+C to exit")
	ui.println("")

	for {
		if ctx.Err() != nil {
			break
		}
		ui.printPrompt()
		line, err := readUserMessage(stdin)
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
				fatal(err)
			}
			break
		}
		if line == "/quit" || line == "/exit" {
			break
		}

		ui.add("you", line)
		ui.render()

		if err := client.RunTurn(ctx, threadID, line, ui); err != nil {
			if errors.Is(err, context.Canceled) {
				break
			}
			ui.println("error: " + err.Error())
		}
		ui.render()
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "tui: %v\n", err)
	os.Exit(1)
}

// --- shared stdin (main loop + approval prompts) ---

type stdinReader struct {
	mu sync.Mutex
	r  *bufio.Reader
}

func newStdinReader() *stdinReader {
	return &stdinReader{r: bufio.NewReader(os.Stdin)}
}

func (s *stdinReader) ReadLine() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	line, err := s.r.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), err
}

func (s *stdinReader) Buffered() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.r.Buffered()
}

// readUserMessage reads one prompt. Enter sends immediately. If more bytes are
// already buffered (multiline paste), remaining non-empty lines are joined.
func readUserMessage(stdin *stdinReader) (string, error) {
	for {
		line, err := stdin.ReadLine()
		if err != nil {
			return "", err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if trimmed == "/quit" || trimmed == "/exit" {
			return trimmed, nil
		}

		parts := []string{line}
		for stdin.Buffered() > 0 {
			next, err := stdin.ReadLine()
			if err != nil {
				break
			}
			if strings.TrimSpace(next) == "" {
				if stdin.Buffered() == 0 {
					break
				}
				continue
			}
			parts = append(parts, next)
		}
		return strings.Join(parts, "\n"), nil
	}
}

func readApprovalDecision(stdin *stdinReader) string {
	if stdin == nil {
		return "decline"
	}
	for {
		fmt.Fprint(os.Stdout, approvalPrompt)
		line, err := stdin.ReadLine()
		if err != nil {
			return "decline"
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "":
			continue
		case "y", "yes":
			return "accept"
		case "n", "no", "q", "quit":
			return "decline"
		default:
			fmt.Fprintln(os.Stdout, "tui: type y or n (q to decline)")
		}
	}
}

// --- chat UI ---

type chatUI struct {
	mu      sync.Mutex
	history []chatLine
}

type chatLine struct {
	role string
	text string
}

func newChatUI() *chatUI {
	return &chatUI{}
}

func (u *chatUI) add(role, text string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.history = append(u.history, chatLine{role: role, text: text})
}

func (u *chatUI) appendAgentDelta(delta string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	n := len(u.history)
	if n > 0 && u.history[n-1].role == "agent" {
		u.history[n-1].text += delta
		return
	}
	u.history = append(u.history, chatLine{role: "agent", text: delta})
}

func (u *chatUI) setLastAgent(text string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	n := len(u.history)
	if n > 0 && u.history[n-1].role == "agent" {
		u.history[n-1].text = text
		return
	}
	u.history = append(u.history, chatLine{role: "agent", text: text})
}

func (u *chatUI) render() {
	u.mu.Lock()
	defer u.mu.Unlock()
	fmt.Print("\n---------- chat ----------\n")
	for _, line := range u.history {
		fmt.Printf("[%s]\n%s\n\n", line.role, line.text)
	}
	fmt.Print("--------------------------\n")
}

func (u *chatUI) println(s string) {
	fmt.Fprintln(os.Stdout, s)
}

func (u *chatUI) printPrompt() {
	fmt.Fprint(os.Stdout, "> ")
}

func (u *chatUI) printDelta(delta string) {
	fmt.Fprint(os.Stdout, delta)
}

// --- app-server JSON-RPC client ---

type appServer struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	userStdin *stdinReader

	mu       sync.Mutex
	nextID   atomic.Int64
	pending  map[int64]chan rpcResponse
	writeMu  sync.Mutex
	closed   atomic.Bool
}

type rpcResponse struct {
	Result json.RawMessage
	Error  *rpcError
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type wireMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

func startAppServer(ctx context.Context, userStdin *stdinReader) (*appServer, error) {
	bin, err := exec.LookPath("codex")
	if err != nil {
		return nil, fmt.Errorf("codex not on PATH: %w", err)
	}

	cmd := exec.CommandContext(ctx, bin, "app-server", "--listen", "stdio://")
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, err
	}

	c := &appServer{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    bufio.NewReaderSize(stdoutPipe, 1024*1024),
		userStdin: userStdin,
		pending:   make(map[int64]chan rpcResponse),
	}
	go c.readLoop()
	return c, nil
}

func (c *appServer) Close() {
	if c.closed.Swap(true) {
		return
	}
	_ = c.stdin.Close()
	if c.cmd.Process == nil {
		return
	}
	_ = c.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		_, _ = c.cmd.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = c.cmd.Process.Kill()
		_, _ = c.cmd.Process.Wait()
	}
}

func (c *appServer) readLoop() {
	for {
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			c.failAll(err)
			return
		}
		line = bytesTrimSpace(line)
		if len(line) == 0 {
			continue
		}

		var msg wireMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			fmt.Fprintf(os.Stderr, "tui: bad json from app-server: %v\n", err)
			continue
		}

		if len(msg.ID) > 0 && msg.Method == "" {
			c.deliverResponse(msg)
			continue
		}
		if msg.Method != "" && len(msg.ID) > 0 {
			c.replyServerRequest(msg)
			continue
		}
		c.dispatchNotification(msg)
	}
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func (c *appServer) failAll(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		ch <- rpcResponse{Error: &rpcError{Message: err.Error()}}
		delete(c.pending, id)
	}
}

func (c *appServer) deliverResponse(msg wireMessage) {
	id, ok := parseID(msg.ID)
	if !ok {
		return
	}
	c.mu.Lock()
	ch := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if ch == nil {
		return
	}
	ch <- rpcResponse{Result: msg.Result, Error: msg.Error}
}

func parseID(raw json.RawMessage) (int64, bool) {
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		n, err := strconv.ParseInt(s, 10, 64)
		return n, err == nil
	}
	return 0, false
}

func (c *appServer) writeJSON(v any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = c.stdin.Write(data)
	return err
}

func (c *appServer) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan rpcResponse, 1)

	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	req := map[string]any{
		"id":     id,
		"method": method,
		"params": params,
	}
	if err := c.writeJSON(req); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

func (c *appServer) notify(method string, params any) error {
	return c.writeJSON(map[string]any{
		"method": method,
		"params": params,
	})
}

func (c *appServer) Initialize(ctx context.Context) error {
	_, err := c.call(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "cortisol_test_tui",
			"title":   "Cortisol Test TUI",
			"version": "0.0.1",
		},
	})
	if err != nil {
		return err
	}
	return c.notify("initialized", map[string]any{})
}

func (c *appServer) StartThread(ctx context.Context, cwd string) (string, error) {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	result, err := c.call(ctx, "thread/start", map[string]any{
		"cwd":            abs,
		"approvalPolicy": "untrusted",
		"sandbox":        "workspace-write",
	})
	if err != nil {
		return "", err
	}
	var out struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(result, &out); err != nil {
		return "", err
	}
	if out.Thread.ID == "" {
		return "", errors.New("thread/start returned empty thread id")
	}
	return out.Thread.ID, nil
}

type turnSink struct {
	ui       *chatUI
	done     chan error
	threadID string
}

var (
	sinkMu sync.Mutex
	sink   *turnSink
)

func (c *appServer) dispatchNotification(msg wireMessage) {
	sinkMu.Lock()
	s := sink
	sinkMu.Unlock()
	if s == nil {
		return
	}

	switch msg.Method {
	case "item/agentMessage/delta":
		var p struct {
			Delta    string `json:"delta"`
			ThreadID string `json:"threadId"`
		}
		if json.Unmarshal(msg.Params, &p) != nil || p.ThreadID != s.threadID {
			return
		}
		s.ui.appendAgentDelta(p.Delta)
		s.ui.printDelta(p.Delta)

	case "item/completed":
		var p struct {
			ThreadID string `json:"threadId"`
			Item     struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if json.Unmarshal(msg.Params, &p) != nil || p.ThreadID != s.threadID {
			return
		}
		if p.Item.Type == "agentMessage" && p.Item.Text != "" {
			s.ui.setLastAgent(p.Item.Text)
			fmt.Fprintln(os.Stdout)
		}

	case "turn/completed":
		var p struct {
			ThreadID string `json:"threadId"`
			Turn     struct {
				Status string `json:"status"`
				Error  *struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"turn"`
		}
		if json.Unmarshal(msg.Params, &p) != nil || p.ThreadID != s.threadID {
			return
		}
		var err error
		if p.Turn.Status == "failed" {
			msg := "turn failed"
			if p.Turn.Error != nil && p.Turn.Error.Message != "" {
				msg = p.Turn.Error.Message
			}
			err = errors.New(msg)
		}
		select {
		case s.done <- err:
		default:
		}

	case "turn/failed":
		select {
		case s.done <- errors.New("turn/failed"):
		default:
		}
	}
}

func (c *appServer) RunTurn(ctx context.Context, threadID, text string, ui *chatUI) error {
	done := make(chan error, 1)
	sinkMu.Lock()
	sink = &turnSink{ui: ui, done: done, threadID: threadID}
	sinkMu.Unlock()
	defer func() {
		sinkMu.Lock()
		sink = nil
		sinkMu.Unlock()
	}()

	_, err := c.call(ctx, "turn/start", map[string]any{
		"threadId": threadID,
		"input": []map[string]any{
			{"type": "text", "text": text},
		},
	})
	if err != nil {
		return err
	}

	fmt.Fprint(os.Stdout, "[agent]\n")
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func isApprovalRequest(method string) bool {
	return strings.Contains(method, "requestApproval") || strings.HasSuffix(method, "/requestApproval")
}

func summarizeApproval(method string, params json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(params, &m) != nil {
		return method
	}
	parts := []string{method}
	if cmd, ok := m["command"]; ok {
		parts = append(parts, fmt.Sprintf("command=%v", cmd))
	}
	if reason, ok := m["reason"].(string); ok && reason != "" {
		parts = append(parts, "reason="+reason)
	}
	if changes, ok := m["changes"]; ok {
		parts = append(parts, fmt.Sprintf("changes=%v", changes))
	}
	if grantRoot, ok := m["grantRoot"]; ok {
		parts = append(parts, fmt.Sprintf("grantRoot=%v", grantRoot))
	}
	return strings.Join(parts, " | ")
}

func (c *appServer) replyServerRequest(msg wireMessage) {
	var id any
	_ = json.Unmarshal(msg.ID, &id)

	var result map[string]any
	switch {
	case isApprovalRequest(msg.Method):
		fmt.Fprintf(os.Stdout, "\n[approval] %s\n", summarizeApproval(msg.Method, msg.Params))
		decision := readApprovalDecision(c.userStdin)
		result = map[string]any{"decision": decision}
		fmt.Fprintf(os.Stdout, "tui: replied %s to %s\n", decision, msg.Method)

	case strings.Contains(msg.Method, "requestUserInput"):
		result = map[string]any{"answers": map[string]any{}}

	default:
		fmt.Fprintf(os.Stderr, "tui: unsupported server request %s — declining\n", msg.Method)
		result = map[string]any{"decision": "decline"}
	}

	if err := c.writeJSON(map[string]any{"id": id, "result": result}); err != nil {
		fmt.Fprintf(os.Stderr, "tui: reply to %s: %v\n", msg.Method, err)
	}
}
