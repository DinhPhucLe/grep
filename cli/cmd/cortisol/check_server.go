package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const checkServerUsage = `Usage: cortisol check-server [options]

Send a test input to the Go server's POST /jobs endpoint and print its response.
This command does not launch Codex or calculate metrics.

Options:
  --server URL       Server base URL (default http://localhost:8080).
  --input TEXT       Test input (default connection-test-001).
  --timeout DURATION Request timeout, including the response body (default 5s).
  --help             Show this help.

Example:
  cortisol check-server --server http://localhost:8080 --input connection-test-001
`

const maxCheckResponseBytes = 1 << 20

func runCheckServer(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("check-server", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, checkServerUsage) }
	server := flags.String("server", "http://localhost:8080", "server base URL")
	input := flags.String("input", "connection-test-001", "test input")
	timeout := flags.Duration("timeout", 5*time.Second, "request timeout")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	endpoint, err := jobsEndpoint(*server)
	switch {
	case err != nil:
	case flags.NArg() != 0:
		err = errors.New("check-server accepts only --server, --input, and --timeout options")
	case strings.TrimSpace(*input) == "":
		err = errors.New("--input must not be empty")
	case *timeout <= 0:
		err = errors.New("--timeout must be greater than zero")
	}
	if err != nil {
		fmt.Fprintln(stderr, "cortisol:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	client := &http.Client{
		Timeout: *timeout,
		// A connection check should report the supplied endpoint's response.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	if err := checkServer(ctx, client, endpoint, *input, stdout); err != nil {
		fmt.Fprintln(stderr, "cortisol:", err)
		return 1
	}
	return 0
}

func jobsEndpoint(server string) (string, error) {
	u, err := url.Parse(server)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("--server must be an absolute http:// or https:// base URL")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", errors.New("--server must be a base URL without credentials, query parameters, or a fragment")
	}
	return u.JoinPath("jobs").String(), nil
}

// checkServer verifies the existing job request/response contract. It is
// independent of the Codex process and the metric display's input channel.
func checkServer(ctx context.Context, client *http.Client, endpoint, input string, output io.Writer) error {
	body, err := json.Marshal(struct {
		Input string `json:"input"`
	}{Input: input})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create test request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if _, err := fmt.Fprintln(output, "POST", endpoint); err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("request timed out: %w", err)
		}
		if errors.Is(err, context.Canceled) {
			return fmt.Errorf("request canceled: %w", err)
		}
		return fmt.Errorf("could not reach the server; check that it is running and --server is correct: %w", err)
	}
	defer response.Body.Close()
	if _, err := fmt.Fprintf(output, "HTTP %d %s\n", response.StatusCode, http.StatusText(response.StatusCode)); err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxCheckResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read server response: %w", err)
	}
	if len(data) > maxCheckResponseBytes {
		return errors.New("server response exceeds 1 MiB")
	}
	if response.StatusCode != http.StatusOK {
		message := strings.TrimSpace(string(data))
		if len(message) > 1024 {
			message = message[:1024] + "..."
		}
		return fmt.Errorf("server returned HTTP %d: %q", response.StatusCode, message)
	}
	var result struct {
		Output *string `json:"output"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("expected a JSON job response with an output string: %w", err)
	}
	if result.Output == nil {
		return errors.New("server response is missing an output string")
	}
	// Quoting keeps server-supplied control characters from affecting the terminal.
	_, err = fmt.Fprintf(output, "Output: %q\nConnection OK\n", *result.Output)
	return err
}
