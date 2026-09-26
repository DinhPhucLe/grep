//go:build windows

package runner

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/charmbracelet/x/xpty"
	"golang.org/x/sys/windows"
)

type sessionPTY struct {
	*xpty.ConPty
	output *os.File
	finish sync.Once
}

// The npm launcher starts a native child. A job lets explicit cancellation
// reach that child as well. It deliberately has no KILL_ON_JOB_CLOSE limit:
// normal exits must not terminate intentionally detached background work.
type directProcess struct {
	process *os.Process
	job     windows.Handle
}

func controlDirectProcess(cmd *exec.Cmd) (*directProcess, error) {
	fallback := &directProcess{process: cmd.Process}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fallback, nil
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		windows.CloseHandle(job)
		return fallback, nil
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		windows.CloseHandle(job)
		return fallback, nil
	}
	return &directProcess{process: cmd.Process, job: job}, nil
}

func (p *directProcess) Kill() error {
	if p.job == 0 {
		return p.process.Kill()
	}
	return windows.TerminateJobObject(p.job, 1)
}
func (p *directProcess) Signal(sig os.Signal) error {
	if p.job == 0 {
		return p.process.Kill()
	}
	code := uint32(1)
	if sig == os.Interrupt {
		code = 130
	} else if sig == syscall.SIGTERM {
		code = 143
	}
	return windows.TerminateJobObject(p.job, code)
}
func (p *directProcess) Close() error {
	if p.job == 0 {
		return nil
	}
	return windows.CloseHandle(p.job)
}

func newSessionPTY(cols, rows int) (*sessionPTY, error) {
	p, err := xpty.NewConPty(cols, rows)
	if err != nil {
		return nil, err
	}
	// Keep an independent read handle: ConPty.Close closes its own pipe after
	// flushing the console, but buffered trailing output still needs draining.
	var duplicate windows.Handle
	process := windows.CurrentProcess()
	if err := windows.DuplicateHandle(process, windows.Handle(p.OutPipeReadFd()), process, &duplicate, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		_ = p.Close()
		return nil, err
	}
	return &sessionPTY{ConPty: p, output: os.NewFile(uintptr(duplicate), "codex-pty-output")}, nil
}

func (p *sessionPTY) Read(data []byte) (int, error) { return p.output.Read(data) }
func (p *sessionPTY) afterStart()                   {}
func (p *sessionPTY) finishOutput()                 { p.finish.Do(func() { _ = p.ConPty.Close() }) }
func (p *sessionPTY) Close() error                  { p.finishOutput(); return p.output.Close() }
func preparePTYCommand(_ *exec.Cmd)                 {}

func terminationSignals() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM} }

func signalProcess(cmd *exec.Cmd, _ os.Signal, _ bool) error { return cmd.Process.Kill() }
func killProcess(cmd *exec.Cmd, _ bool) error                { return cmd.Process.Kill() }
func signalExitCode(_ *os.ProcessState) int                  { return 1 }
func normalPTYEOF(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || errors.Is(err, windows.ERROR_BROKEN_PIPE)
}

// An overlapped CONIN$ handle lets cancellation unblock an input read without
// consuming a later keystroke or closing the caller's standard input handle.
type terminalInput struct {
	handle, ready, canceled windows.Handle
	mu                      sync.Mutex
	once                    sync.Once
	closed                  chan struct{}
}

func newInputReader(_ *os.File) (io.ReadCloser, error) {
	name, err := windows.UTF16PtrFromString("CONIN$")
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, err
	}
	ready, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}
	canceled, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(handle)
		windows.CloseHandle(ready)
		return nil, err
	}
	return &terminalInput{handle: handle, ready: ready, canceled: canceled, closed: make(chan struct{})}, nil
}

func (r *terminalInput) Read(data []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case <-r.closed:
		return 0, io.EOF
	default:
	}
	if err := windows.ResetEvent(r.ready); err != nil {
		return 0, err
	}
	overlapped := windows.Overlapped{HEvent: r.ready}
	var n uint32
	err := windows.ReadFile(r.handle, data, &n, &overlapped)
	if err != windows.ERROR_IO_PENDING {
		return int(n), err
	}
	event, err := windows.WaitForMultipleObjects([]windows.Handle{r.ready, r.canceled}, false, windows.INFINITE)
	if err != nil || event == windows.WAIT_OBJECT_0+1 {
		_ = windows.CancelIoEx(r.handle, &overlapped)
		_ = windows.GetOverlappedResult(r.handle, &overlapped, &n, true)
		if err != nil {
			return 0, err
		}
		return 0, io.EOF
	}
	err = windows.GetOverlappedResult(r.handle, &overlapped, &n, true)
	return int(n), err
}

func (r *terminalInput) Close() (err error) {
	r.once.Do(func() {
		close(r.closed)
		_ = windows.SetEvent(r.canceled)
		r.mu.Lock()
		defer r.mu.Unlock()
		err = errors.Join(windows.CloseHandle(r.handle), windows.CloseHandle(r.ready), windows.CloseHandle(r.canceled))
	})
	return err
}
