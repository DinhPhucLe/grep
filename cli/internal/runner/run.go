// Package runner hosts Codex and renders externally supplied metric snapshots.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"cortisol-cli/metrics"
	"cortisol-cli/terminal"
	"github.com/charmbracelet/x/term"
	"github.com/charmbracelet/x/xpty"
)

// Options contains only presentation and process configuration. Updates must
// contain completed snapshots; the runner does not calculate metrics.
type Options struct {
	Args                  []string
	Gauge                 bool
	Updates               <-chan metrics.Snapshot
	Stdin, Stdout, Stderr *os.File
}

func (o Options) streams() Options {
	if o.Stdin == nil {
		o.Stdin = os.Stdin
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	return o
}

// OpenTerminalInput returns a cancellable terminal reader. Closing it unblocks
// pending reads without closing the caller's file or consuming later input.
// The caller owns terminal raw-mode setup and restoration.
func OpenTerminalInput(input *os.File) (io.ReadCloser, error) {
	return newInputReader(input)
}

// Run starts Codex. A child command's nonzero exit is returned as a code, not an
// error; errors describe failures in the wrapper itself.
func Run(ctx context.Context, opts Options) (int, error) {
	if err := ctx.Err(); err != nil {
		return 1, err
	}
	cmd, err := Resolve(opts.Args)
	if err != nil {
		return 1, err
	}
	return runCommand(ctx, cmd, opts.streams())
}

func runCommand(ctx context.Context, cmd *exec.Cmd, opts Options) (int, error) {
	if !opts.Gauge || os.Getenv("TERM") == "dumb" || !term.IsTerminal(opts.Stdin.Fd()) || !term.IsTerminal(opts.Stdout.Fd()) {
		return runDirect(ctx, cmd, opts)
	}
	return runTerminal(ctx, cmd, opts)
}

func runDirect(ctx context.Context, cmd *exec.Cmd, opts Options) (int, error) {
	cmd.Stdin, cmd.Stdout, cmd.Stderr = opts.Stdin, opts.Stdout, opts.Stderr
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, terminationSignals()...)
	defer signal.Stop(signals)
	if err := cmd.Start(); err != nil {
		return 1, err
	}
	control, err := controlDirectProcess(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return 1, fmt.Errorf("track Codex process: %w", err)
	}
	defer control.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for {
		select {
		case err := <-done:
			return processResult(cmd, err)
		case sig := <-signals:
			_ = control.Signal(sig)
		case <-ctx.Done():
			_ = control.Kill()
			<-done
			return 130, ctx.Err()
		}
	}
}

type outputChunk struct {
	data []byte
	err  error
}

func runTerminal(ctx context.Context, cmd *exec.Cmd, opts Options) (code int, resultErr error) {
	cols, rows, err := term.GetSize(opts.Stdout.Fd())
	if err != nil {
		return 1, fmt.Errorf("read terminal size: %w", err)
	}
	restoreOutput, err := terminal.EnableOutput(opts.Stdout)
	if err != nil {
		return 1, fmt.Errorf("enable terminal output: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, restoreOutput()) }()
	state, err := term.MakeRaw(opts.Stdin.Fd())
	if err != nil {
		return 1, fmt.Errorf("enable raw input: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, term.Restore(opts.Stdin.Fd(), state)) }()
	input, err := newInputReader(opts.Stdin)
	if err != nil {
		return 1, err
	}
	defer input.Close()
	renderer := terminal.NewRenderer(opts.Stdout, cols, rows, true)
	if err := renderer.Start(); err != nil {
		return 1, err
	}
	defer func() { resultErr = errors.Join(resultErr, renderer.Stop()) }()
	width, height := renderer.Dimensions()
	pty, err := newSessionPTY(width, height)
	if err != nil {
		return 1, fmt.Errorf("create pseudoterminal: %w", err)
	}
	defer pty.Close()
	preparePTYCommand(cmd)
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	if os.Getenv("TERM") == "" {
		cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	}
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, terminationSignals()...)
	defer signal.Stop(signals)
	if err := pty.Start(cmd); err != nil {
		return 1, fmt.Errorf("start Codex: %w", err)
	}
	pty.afterStart()
	output := make(chan outputChunk, 16)
	go func() {
		buffer := make([]byte, 32*1024)
		for {
			n, err := pty.Read(buffer)
			if n > 0 {
				output <- outputChunk{data: append([]byte(nil), buffer[:n]...)}
			}
			if err != nil {
				output <- outputChunk{err: err}
				close(output)
				return
			}
		}
	}()
	inputDone := make(chan error, 1)
	go func() { _, err := io.Copy(pty, input); inputDone <- err }()
	wait := make(chan error, 1)
	go func() { wait <- xpty.WaitProcess(context.Background(), cmd) }()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	updates, cancellation := opts.Updates, ctx.Done()
	var latest metrics.Snapshot
	var waitErr, failure error
	exited, drained := false, false
	outputFailed := false
	fail := func(err error) {
		if failure == nil {
			failure = err
			_ = killProcess(cmd, true)
		}
	}
	syncSize := func() error {
		nextWidth, nextHeight := renderer.Dimensions()
		if nextWidth == width && nextHeight == height {
			return nil
		}
		width, height = nextWidth, nextHeight
		return pty.Resize(width, height)
	}
	// All rendering happens on this goroutine. Metric updates can repaint the
	// footer even while Codex is silent, and output is drained before cleanup.
	for !exited || !drained {
		select {
		case chunk, ok := <-output:
			if !ok {
				output = nil
				drained = true
				continue
			}
			if len(chunk.data) > 0 && !outputFailed {
				if _, err := renderer.Write(chunk.data); err != nil {
					outputFailed = true
					fail(fmt.Errorf("write terminal output: %w", err))
				}
				if !exited && failure == nil {
					if err := syncSize(); err != nil {
						fail(err)
					}
				}
			}
			if chunk.err != nil && !normalPTYEOF(chunk.err) {
				fail(fmt.Errorf("read Codex output: %w", chunk.err))
			}
		case err := <-wait:
			waitErr, exited, wait = err, true, nil
			_ = input.Close()
			// ConPTY must be closed while its output is still being consumed.
			// On Unix closing the parent's slave at startup provides natural EOF.
			go pty.finishOutput()
		case err := <-inputDone:
			inputDone = nil
			if !exited {
				if err != nil {
					fail(fmt.Errorf("forward terminal input: %w", err))
				} else {
					_ = killProcess(cmd, true)
				}
			}
		case snapshot, ok := <-updates:
			if !ok {
				updates = nil
				continue
			}
			if err := snapshot.Validate(); err != nil {
				snapshot = metrics.Snapshot{RiskLabel: "Invalid metrics"}
			}
			latest = snapshot.Clone()
			if failure == nil {
				if err := renderer.Update(latest); err != nil {
					fail(err)
				}
			}
		case <-ticker.C:
			if exited || failure != nil {
				continue
			}
			nextCols, nextRows, err := term.GetSize(opts.Stdout.Fd())
			if err != nil {
				fail(err)
				continue
			}
			if nextCols != cols || nextRows != rows {
				cols, rows = nextCols, nextRows
				if err := renderer.Resize(cols, rows); err != nil {
					fail(err)
					continue
				}
				if err := syncSize(); err != nil {
					fail(err)
				}
			}
			if err := renderer.Update(latest); err != nil {
				fail(err)
			}
		case sig := <-signals:
			if !exited {
				_ = signalProcess(cmd, sig, true)
			}
		case <-cancellation:
			cancellation = nil
			fail(ctx.Err())
		}
	}
	code, err = processResult(cmd, waitErr)
	if errors.Is(failure, context.Canceled) || errors.Is(failure, context.DeadlineExceeded) {
		code = 130
	}
	return code, errors.Join(err, failure)
}

func processResult(cmd *exec.Cmd, err error) (int, error) {
	if cmd.ProcessState != nil {
		if code := cmd.ProcessState.ExitCode(); code >= 0 {
			return code, nil
		}
		return signalExitCode(cmd.ProcessState), nil
	}
	if err != nil {
		return 1, err
	}
	return 0, nil
}
