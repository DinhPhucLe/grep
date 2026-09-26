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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	_ "time/tzdata"
)

// --- session JSON-RPC log (--log) ---

type sessionLogger struct {
	mu     sync.Mutex
	buf    [][]byte // JSONL lines buffered until thread id is known
	f      *os.File
	path   string
	closed bool
}

type logLine struct {
	TS        string          `json:"ts"`
	Direction string          `json:"direction"`
	Message   json.RawMessage `json:"message"`
}

func newSessionLogger() *sessionLogger {
	return &sessionLogger{}
}

func resolveLogDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if filepath.Base(cwd) == "tui" {
		return filepath.Join(cwd, "log"), nil
	}
	return filepath.Join(cwd, "tui", "log"), nil
}

func (l *sessionLogger) Path() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path
}

// BindThread creates tui/log/{EasternTime}_{threadID}.jsonl and flushes
// any RPC lines captured during handshake.
func (l *sessionLogger) BindThread(threadID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return errors.New("session log already closed")
	}
	if l.f != nil {
		return nil
	}

	dir, err := resolveLogDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.FixedZone("EST", -5*60*60)
	}
	stamp := time.Now().In(loc).Format("2006-01-02T150405MST")
	safeID := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, threadID)
	path := filepath.Join(dir, stamp+"_"+safeID+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	l.f = f
	l.path = path
	for _, line := range l.buf {
		if _, err := l.f.Write(line); err != nil {
			return err
		}
	}
	l.buf = nil
	return l.f.Sync()
}

func (l *sessionLogger) Record(direction string, payload any) {
	if l == nil {
		return
	}
	msg, err := json.Marshal(payload)
	if err != nil {
		return
	}
	entry, err := json.Marshal(logLine{
		TS:        time.Now().UTC().Format(time.RFC3339Nano),
		Direction: direction,
		Message:   msg,
	})
	if err != nil {
		return
	}
	entry = append(entry, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}

	if l.f == nil {
		l.buf = append(l.buf, entry)
		return
	}
	_, _ = l.f.Write(entry)
	_ = l.f.Sync()
}

func (l *sessionLogger) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	l.closed = true

	// Unexpected exit before thread/start: still persist buffered handshake RPC.
	if l.f == nil && len(l.buf) > 0 {
		dir, err := resolveLogDir()
		if err == nil {
			_ = os.MkdirAll(dir, 0o755)
			loc, locErr := time.LoadLocation("America/New_York")
			if locErr != nil {
				loc = time.FixedZone("EST", -5*60*60)
			}
			stamp := time.Now().In(loc).Format("2006-01-02T150405MST")
			path := filepath.Join(dir, stamp+"_no-thread.jsonl")
			if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
				l.f = f
				l.path = path
				for _, line := range l.buf {
					_, _ = l.f.Write(line)
				}
				l.buf = nil
			}
		}
	}

	if l.f != nil {
		_ = l.f.Sync()
		_ = l.f.Close()
		l.f = nil
	}
}

// --- app-server JSON-RPC client ---

type appServer struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	events *eventQueue
	log    *sessionLogger

	mu           sync.Mutex
	nextID       atomic.Int64
	pending      map[int64]chan rpcResponse
	writeMu      sync.Mutex
	closed       atomic.Bool
	disconnected atomic.Bool
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

func startAppServer(ctx context.Context, events *eventQueue, slog *sessionLogger) (*appServer, error) {
	cmd, err := codexCommand(ctx, "app-server", "--listen", "stdio://")
	if err != nil {
		return nil, err
	}

	cmd.Stderr = io.Discard

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
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReaderSize(stdoutPipe, 1024*1024),
		events:  newEventQueue(),
		log:     slog,
		pending: make(map[int64]chan rpcResponse),
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
			c.disconnected.Store(true)
			c.failAll(err)
			c.events.push(disconnectedMsg{err: err})
			return
		}
		line = bytesTrimSpace(line)
		if len(line) == 0 {
			continue
		}

		var msg wireMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			c.events.push(protocolErrorMsg{err: err})
			continue
		}
		c.log.Record("in", json.RawMessage(append([]byte(nil), line...)))

		if len(msg.ID) > 0 && msg.Method == "" {
			c.deliverResponse(msg)
			continue
		}
		if msg.Method != "" && len(msg.ID) > 0 {
			c.events.push(msg)
			continue
		}
		c.events.push(msg)
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
	c.log.Record("out", json.RawMessage(append([]byte(nil), data...)))
	data = append(data, '\n')
	_, err = c.stdin.Write(data)
	return err
}

func (c *appServer) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if c.disconnected.Load() {
		return nil, errors.New("app-server disconnected")
	}
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
