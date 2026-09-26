//go:build windows

package terminal

import (
	"os"

	"golang.org/x/sys/windows"
)

// EnableOutput enables VT processing for a Windows console and returns a
// restoration function. Call only for an interactive terminal file.
func EnableOutput(output *os.File) (func() error, error) {
	handle := windows.Handle(output.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return nil, err
	}
	if err := windows.SetConsoleMode(handle, mode|windows.ENABLE_PROCESSED_OUTPUT|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		return nil, err
	}
	return func() error { return windows.SetConsoleMode(handle, mode) }, nil
}
