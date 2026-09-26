//go:build !windows

package runner

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/charmbracelet/x/xpty"
	"golang.org/x/sys/unix"
)

type sessionPTY struct{ *xpty.UnixPty }

type directProcess struct{ *os.Process }

func controlDirectProcess(cmd *exec.Cmd) (*directProcess, error) {
	return &directProcess{cmd.Process}, nil
}

func (p *directProcess) Close() error { return nil }

func newSessionPTY(cols, rows int) (*sessionPTY, error) {
	p, err := xpty.NewUnixPty(cols, rows)
	if err != nil {
		return nil, err
	}
	return &sessionPTY{p}, nil
}

func (p *sessionPTY) afterStart()   { _ = p.Slave().Close() }
func (p *sessionPTY) finishOutput() {}

func preparePTYCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
}

func terminationSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
}

func signalProcess(cmd *exec.Cmd, sig os.Signal, group bool) error {
	if group {
		if signal, ok := sig.(syscall.Signal); ok {
			return syscall.Kill(-cmd.Process.Pid, signal)
		}
	}
	return cmd.Process.Signal(sig)
}

func killProcess(cmd *exec.Cmd, group bool) error { return signalProcess(cmd, syscall.SIGKILL, group) }

func signalExitCode(state *os.ProcessState) int {
	if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	return 1
}

func normalPTYEOF(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO) || errors.Is(err, os.ErrClosed)
}

// Polling a duplicate terminal descriptor makes reads cancellable without
// closing stdin or changing its file status flags for the caller.
type terminalInput struct {
	fd     int
	mu     sync.Mutex
	closed chan struct{}
	once   sync.Once
}

func newInputReader(input *os.File) (io.ReadCloser, error) {
	fd, err := unix.Dup(int(input.Fd()))
	if err != nil {
		return nil, err
	}
	unix.CloseOnExec(fd)
	return &terminalInput{fd: fd, closed: make(chan struct{})}, nil
}

func (r *terminalInput) Read(data []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for {
		select {
		case <-r.closed:
			return 0, io.EOF
		default:
		}
		fds := []unix.PollFd{{Fd: int32(r.fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, 100)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if n == 0 {
			continue
		}
		select {
		case <-r.closed:
			return 0, io.EOF
		default:
		}
		n, err = unix.Read(r.fd, data)
		if n == 0 && err == nil {
			err = io.EOF
		}
		return n, err
	}
}

func (r *terminalInput) Close() (err error) {
	r.once.Do(func() {
		close(r.closed)
		r.mu.Lock()
		defer r.mu.Unlock()
		err = unix.Close(r.fd)
	})
	return err
}
