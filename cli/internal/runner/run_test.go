package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"cortisol-cli/metrics"
	"github.com/charmbracelet/x/term"
	"github.com/charmbracelet/x/xpty"
)

func helperCommand(mode string, args ...string) *exec.Cmd {
	argv := append([]string{"-test.run=^TestRunnerHelper$", "--", mode}, args...)
	cmd := exec.Command(os.Args[0], argv...)
	cmd.Env = append(os.Environ(), "CORTISOL_RUNNER_HELPER=1")
	return cmd
}

func TestRunnerHelper(t *testing.T) {
	if os.Getenv("CORTISOL_RUNNER_HELPER") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) < 2 {
		os.Exit(99)
	}
	switch args[1] {
	case "args":
		_ = json.NewEncoder(os.Stdout).Encode(args[2:])
		fmt.Fprint(os.Stderr, "child stderr\n")
		os.Exit(7)
	case "copy":
		_, _ = io.Copy(os.Stdout, os.Stdin)
	case "sleep":
		time.Sleep(30 * time.Second)
	case "spawn":
		child := helperCommand("sleep")
		if err := child.Start(); err != nil {
			os.Exit(96)
		}
		fmt.Fprintf(os.Stdout, "%d\n", child.Process.Pid)
		time.Sleep(30 * time.Second)
	case "pty":
		for i := 0; i < 1000; i++ {
			fmt.Fprintf(os.Stdout, "output line %04d\r\n", i)
		}
		fmt.Fprint(os.Stdout, "final-output-marker\r\n")
		os.Exit(23)
	case "quiet":
		time.Sleep(600 * time.Millisecond)
		fmt.Fprint(os.Stdout, "quiet-child-final-marker\r\n")
		os.Exit(23)
	case "input":
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil || strings.TrimSpace(line) != "hello" {
			fmt.Fprintf(os.Stderr, "bad-input:%q %v", line, err)
			os.Exit(97)
		}
		fmt.Fprint(os.Stdout, "input-received-marker\r\n")
		os.Exit(23)
	case "interactive", "interactive-input", "interactive-cancel":
		beforeInput, _ := term.GetState(os.Stdin.Fd())
		beforeOutput, _ := term.GetState(os.Stdout.Fd())
		updates := make(chan metrics.Snapshot, 1)
		go func() {
			time.Sleep(100 * time.Millisecond)
			score := 31.0
			updates <- metrics.Snapshot{Score: &score, RiskLabel: "supplied-metric-label", Trend: "stable"}
			close(updates)
		}()
		ctx := context.Background()
		childMode := "quiet"
		if args[1] == "interactive-input" {
			childMode = "input"
		}
		if args[1] == "interactive-cancel" {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, 250*time.Millisecond)
			defer cancel()
			childMode = "sleep"
		}
		code, err := runCommand(ctx, helperCommand(childMode), (Options{Gauge: true, Updates: updates}).streams())
		if args[1] == "interactive-cancel" && errors.Is(err, context.DeadlineExceeded) && code == 130 {
			fmt.Fprint(os.Stdout, "cancelled-code=130\r\n")
			err, code = nil, 23
		}
		afterInput, _ := term.GetState(os.Stdin.Fd())
		afterOutput, _ := term.GetState(os.Stdout.Fd())
		if err != nil {
			fmt.Fprintf(os.Stderr, "interactive-error:%v\r\n", err)
			os.Exit(98)
		}
		fmt.Fprintf(os.Stdout, "restored-input=%t restored-output=%t\r\n", reflect.DeepEqual(beforeInput, afterInput), reflect.DeepEqual(beforeOutput, afterOutput))
		os.Exit(code)
	}
	os.Exit(0)
}

func testStreams(t *testing.T, input string) Options {
	t.Helper()
	create := func(name string) *os.File {
		file, err := os.CreateTemp(t.TempDir(), name)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { file.Close() })
		return file
	}
	opts := Options{Stdin: create("input"), Stdout: create("output"), Stderr: create("error")}
	if _, err := io.WriteString(opts.Stdin, input); err != nil {
		t.Fatal(err)
	}
	if _, err := opts.Stdin.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	return opts
}

func readStream(t *testing.T, file *os.File) []byte {
	t.Helper()
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDirectPreservesArgumentsStreamsAndExit(t *testing.T) {
	args := []string{"--", "multi word prompt", `"quotes"`, `a&b|c;$(no)`, "", "日本語"}
	opts := testStreams(t, "")
	code, err := runCommand(context.Background(), helperCommand("args", args...), opts)
	if err != nil || code != 7 {
		t.Fatalf("code = %d, error = %v", code, err)
	}
	var got []string
	if err := json.Unmarshal(readStream(t, opts.Stdout), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, args) {
		t.Fatalf("arguments = %#v", got)
	}
	if got := string(readStream(t, opts.Stderr)); got != "child stderr\n" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestRedirectedGaugeIsTransparent(t *testing.T) {
	input := strings.Repeat("raw\x1b[31m data 日本語\x00\r\n", 10000)
	opts := testStreams(t, input)
	opts.Gauge = true
	code, err := runCommand(context.Background(), helperCommand("copy"), opts)
	if err != nil || code != 0 {
		t.Fatalf("code = %d, error = %v", code, err)
	}
	if !bytes.Equal(readStream(t, opts.Stdout), []byte(input)) {
		t.Fatal("redirected output changed or was truncated")
	}
}

func TestCancellationReapsProcess(t *testing.T) {
	opts := testStreams(t, "")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	cmd := helperCommand("sleep")
	code, err := runCommand(ctx, cmd, opts)
	if code != 130 || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("code = %d, error = %v", code, err)
	}
	if cmd.ProcessState == nil {
		t.Fatal("child was not reaped")
	}
}

func TestPTYDrainsTrailingOutputAndPreservesExit(t *testing.T) {
	output := capturePTYHelper(t, "pty")
	if !bytes.Contains(output, []byte("final-output-marker")) {
		t.Fatalf("missing trailing output in %d bytes", len(output))
	}
}

func TestInteractiveMetricsAndTerminalRestoration(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "1")
	output := capturePTYHelper(t, "interactive")
	for _, want := range []string{"supplied-metric-label", "quiet-child-final-marker", "restored-input=true restored-output=true"} {
		if !bytes.Contains(output, []byte(want)) {
			t.Errorf("missing %q in interactive output: %q", want, output)
		}
	}
}

func TestInteractiveInputAndCancellation(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	for _, tc := range []struct{ mode, input, want string }{
		{"interactive-input", "hello\r", "input-received-marker"},
		{"interactive-cancel", "", "cancelled-code=130"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			output := capturePTYHelper(t, tc.mode, tc.input)
			for _, want := range []string{tc.want, "restored-input=true restored-output=true"} {
				if !bytes.Contains(output, []byte(want)) {
					t.Errorf("missing %q in output: %q", want, output)
				}
			}
		})
	}
}

func capturePTYHelper(t *testing.T, mode string, input ...string) []byte {
	t.Helper()
	p, err := newSessionPTY(80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	cmd := helperCommand(mode)
	preparePTYCommand(cmd)
	if err := p.Start(cmd); err != nil {
		t.Fatal(err)
	}
	p.afterStart()
	if len(input) > 0 && input[0] != "" {
		go func() { time.Sleep(250 * time.Millisecond); _, _ = p.Write([]byte(input[0])) }()
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = killProcess(cmd, true)
		}
	})
	type result struct {
		data []byte
		err  error
	}
	output := make(chan result, 1)
	go func() { data, err := io.ReadAll(p); output <- result{data, err} }()
	wait := make(chan error, 1)
	go func() { wait <- xpty.WaitProcess(context.Background(), cmd) }()
	select {
	case err := <-wait:
		code, resultErr := processResult(cmd, err)
		if resultErr != nil || code != 23 {
			t.Fatalf("code = %d, error = %v", code, resultErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("PTY child did not exit")
	}
	go p.finishOutput()
	select {
	case result := <-output:
		if result.err != nil && !normalPTYEOF(result.err) {
			t.Fatal(result.err)
		}
		return result.data
	case <-time.After(10 * time.Second):
		t.Fatal("PTY output did not drain")
	}
	return nil
}
