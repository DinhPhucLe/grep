package main

import (
	"context"
	"cortisol-server/internal/timing"
	"errors"
	"flag"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"os"
	"sync"
)

func main() {
	logFlag := flag.Bool("log", false, "record JSON-RPC under tui/log/")
	viewFlag := flag.Bool("view", false, "pretty-print a session JSONL (optional path)")
	icons := flag.Bool("no-icons", false, "use plain role labels")
	motion := flag.Bool("reduced-motion", false, "disable animated status and cursor")
	noColor := flag.Bool("no-color", os.Getenv("NO_COLOR") != "", "disable color and text styling")
	evaluationServer := flag.String("evaluation-server", "http://127.0.0.1:8080", "Go evaluation API base URL")
	approvalPolicy := flag.String("approval-policy", "on-request", "Codex execution approvals: on-request or never (workspace sandbox stays enabled)")
	timingLog := flag.String("timing-log", "", "append payload-free latency events to this JSONL file")
	flag.Parse()
	if *viewFlag {
		if err := viewSessionLog(flag.Arg(0)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *noColor {
		lipgloss.SetColorProfile(termenv.Ascii)
	} else {
		// The full-screen renderer already requires an ANSI-capable terminal.
		// Windows terminals may omit TERM even though styling is supported.
		lipgloss.SetColorProfile(termenv.ANSI256)
	}
	if err := run(*logFlag, uiOptions{NoIcons: *icons, ReducedMotion: *motion, NoColor: *noColor, EvaluationServer: *evaluationServer, ApprovalPolicy: *approvalPolicy, TimingLog: *timingLog}); err != nil {
		fmt.Fprintln(os.Stderr, "tui:", err)
		os.Exit(1)
	}
}

func run(logging bool, opts uiOptions) error {
	if _, err := threadStartParams(".", opts.ApprovalPolicy); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	var logger *sessionLogger
	if logging {
		logger = newSessionLogger()
		defer logger.Close()
	}
	client, err := startAppServer(ctx, nil, logger)
	if err != nil {
		return err
	}
	defer client.Close()
	m := newModel(client, cwd, opts)
	m.ctx = ctx
	if opts.TimingLog != "" {
		file, err := os.OpenFile(opts.TimingLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("open timing log: %w", err)
		}
		defer file.Close()
		m.timingSink = timing.JSONSink(file)
	}
	inputOptions, closeInput, err := terminalInputOptions()
	if err != nil {
		return err
	}
	defer closeInput()
	programOptions := append([]tea.ProgramOption{tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithFPS(30)}, inputOptions...)
	_, err = tea.NewProgram(m, programOptions...).Run()
	if errors.Is(err, tea.ErrInterrupted) {
		return nil
	}
	return err
}

// The reader only appends to this queue. Slow rendering and open approvals
// cannot block response delivery or lose events through a full channel.
type eventQueue struct {
	mu     sync.Mutex
	values []tea.Msg
	wake   chan struct{}
}

func newEventQueue() *eventQueue { return &eventQueue{wake: make(chan struct{}, 1)} }
func (q *eventQueue) push(v tea.Msg) {
	q.mu.Lock()
	q.values = append(q.values, v)
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}
func (q *eventQueue) next(ctx context.Context) tea.Msg {
	for {
		q.mu.Lock()
		n := min(256, len(q.values))
		v := q.values[:n:n]
		q.values = q.values[n:]
		if len(q.values) == 0 {
			q.values = nil
		}
		q.mu.Unlock()
		if len(v) > 0 {
			return rpcBatch(v)
		}
		select {
		case <-q.wake:
		case <-ctx.Done():
			return nil
		}
	}
}

type rpcBatch []tea.Msg
type disconnectedMsg struct{ err any }
type protocolErrorMsg struct{ err error }
