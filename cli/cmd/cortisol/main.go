package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	cli "cortisol-cli"
	"cortisol-cli/internal/runner"
	"cortisol-cli/metrics"
	"cortisol-cli/terminal"
	"github.com/charmbracelet/x/term"
)

const usage = `Usage: cortisol [options] [-- Codex arguments...]
       cortisol check-server [--server URL] [--input TEXT] [--timeout 5s]

Launch Codex with a pixel Vibe meter supplied by your metric producer.

Options:
  --codex           Launch Codex (the default).
  --metrics PATH    Watch a JSON display snapshot; never consumes Codex stdin.
  --no-gauge        Run Codex with its original terminal and no status meter.
  --demo            Preview explicitly marked sample display values.
  --render PATH     Validate and print a snapshot once (use - for stdin).
  --help            Show this help. Use -- --help for Codex's help.

Examples:
  cortisol
  cortisol --metrics status.json -- resume
  cortisol --no-gauge -- --help
  cortisol --demo
  cortisol --render examples/snapshot.json
  cortisol check-server --server http://localhost:8080 --input connection-test-001
`

type options struct {
	codex, noGauge, demo    bool
	metricsPath, renderPath string
	args                    []string
}

func parse(args []string, output io.Writer) (options, error) {
	var opts options
	for i, arg := range args {
		if arg == "--" {
			opts.args = append([]string(nil), args[i+1:]...)
			args = args[:i]
			break
		}
	}
	flags := flag.NewFlagSet("cortisol", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.Usage = func() { fmt.Fprint(output, usage) }
	flags.BoolVar(&opts.codex, "codex", false, "launch Codex")
	flags.BoolVar(&opts.noGauge, "no-gauge", false, "disable status meter")
	flags.BoolVar(&opts.demo, "demo", false, "preview sample values")
	flags.StringVar(&opts.metricsPath, "metrics", "", "JSON snapshot path")
	flags.StringVar(&opts.renderPath, "render", "", "print one JSON snapshot")
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if flags.NArg() != 0 {
		return opts, errors.New("Codex arguments must follow --")
	}
	if opts.demo && (opts.codex || opts.metricsPath != "" || opts.renderPath != "" || len(opts.args) != 0) {
		return opts, errors.New("--demo runs separately from Codex and metric input")
	}
	if opts.renderPath != "" && (opts.codex || opts.noGauge || opts.metricsPath != "" || len(opts.args) != 0) {
		return opts, errors.New("--render runs separately from Codex")
	}
	if opts.metricsPath == "-" {
		return opts, errors.New("--metrics requires a file path; stdin belongs to Codex (use --render - for a one-shot pipe)")
	}
	return opts, nil
}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) > 0 && args[0] == "check-server" {
		return runCheckServer(args[1:], os.Stdout, os.Stderr)
	}
	opts, err := parse(args, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cortisol:", err)
		return 2
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var code int
	switch {
	case opts.renderPath != "":
		err = renderFile(opts.renderPath, os.Stdin, os.Stdout)
	case opts.demo:
		var stop context.CancelFunc
		ctx, stop = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
		defer stop()
		err = demo(ctx, !opts.noGauge)
	default:
		var updates <-chan metrics.Snapshot
		if opts.metricsPath != "" && !opts.noGauge {
			updates = metrics.Watch(ctx, opts.metricsPath, 250*time.Millisecond)
		}
		code, err = cli.Run(ctx, cli.Options{Args: opts.args, Gauge: !opts.noGauge, Updates: updates})
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cortisol:", err)
		return 1
	}
	return code
}

func renderFile(path string, stdin io.Reader, stdout io.Writer) error {
	r := stdin
	if path != "-" {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("snapshot path must be a regular file")
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		info, err = f.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("snapshot path must be a regular file")
		}
		r = f
	}
	snapshot, err := metrics.Decode(r)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, terminal.RenderStatusLine(snapshot, 100, false))
	return err
}

func demo(ctx context.Context, gauge bool) (resultErr error) {
	// These are complete producer fixtures. No label/trend is derived here.
	states := []metrics.Snapshot{
		{Score: ptr(30), RiskLabel: "LOW", Trend: "stable", Delta: ptr(0)},
		{Score: ptr(58), RiskLabel: "MODERATE", Trend: "rising", Delta: ptr(28)},
		{Score: ptr(86), RiskLabel: "HIGH", Trend: "rising", Delta: ptr(28)},
		{Score: ptr(30), RiskLabel: "LOW", Trend: "falling", Delta: ptr(-56)},
	}
	if !gauge || !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) || os.Getenv("TERM") == "dumb" {
		for _, s := range states {
			if _, err := fmt.Fprintln(os.Stdout, terminal.RenderStatusLine(s, 80, false)); err != nil {
				return err
			}
		}
		return nil
	}
	width, height, err := term.GetSize(os.Stdout.Fd())
	if err != nil {
		return err
	}
	restoreOutput, err := terminal.EnableOutput(os.Stdout)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, restoreOutput()) }()
	state, err := term.MakeRaw(os.Stdin.Fd())
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, term.Restore(os.Stdin.Fd(), state)) }()
	input, err := runner.OpenTerminalInput(os.Stdin)
	if err != nil {
		return err
	}
	defer input.Close()
	renderer := terminal.NewRenderer(os.Stdout, width, height, true)
	if err := renderer.Start(); err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, renderer.Stop()) }()
	if _, err := renderer.Write([]byte("Cortisol demo: sample data only. Press q or Ctrl+C to exit.\r\n")); err != nil {
		return err
	}
	if err := renderer.Update(states[0]); err != nil {
		return err
	}
	quit := make(chan struct{})
	go func() {
		defer close(quit)
		buffer := make([]byte, 128)
		for {
			n, err := input.Read(buffer)
			if err != nil || strings.ContainsAny(string(buffer[:n]), "q\x03") {
				return
			}
		}
	}()
	defer func() {
		_ = input.Close()
		<-quit
	}()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-quit:
			return nil
		case <-ticker.C:
			w, h, err := term.GetSize(os.Stdout.Fd())
			if err == nil && (w != width || h != height) {
				width, height = w, h
				if err := renderer.Resize(w, h); err != nil {
					return err
				}
			}
			index := int(time.Since(start)/(10*time.Second)) % len(states)
			if err := renderer.Update(states[index]); err != nil {
				return err
			}
		}
	}
}

func ptr(value float64) *float64 { return &value }
