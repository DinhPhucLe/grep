//go:build windows

package runner

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestDirectCancellationTerminatesLauncherDescendants(t *testing.T) {
	opts := testStreams(t, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() { code, err := runCommand(ctx, helperCommand("spawn"), opts); done <- result{code, err} }()
	var pid int
	deadline := time.After(5 * time.Second)
	for pid == 0 {
		select {
		case result := <-done:
			t.Fatalf("launcher exited before spawning: %+v", result)
		case <-deadline:
			t.Fatal("launcher did not create child")
		case <-time.After(10 * time.Millisecond):
			data, err := os.ReadFile(opts.Stdout.Name())
			if err != nil {
				t.Fatal(err)
			}
			pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		}
	}
	child, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = windows.TerminateProcess(child, 1); _ = windows.CloseHandle(child) }()
	cancel()
	select {
	case result := <-done:
		if result.code != 130 || !errors.Is(result.err, context.Canceled) {
			t.Fatalf("result = %+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not stop launcher")
	}
	event, err := windows.WaitForSingleObject(child, 5000)
	if err != nil || event != windows.WAIT_OBJECT_0 {
		t.Fatalf("launcher descendant survived cancellation: event=%d error=%v", event, err)
	}
}
