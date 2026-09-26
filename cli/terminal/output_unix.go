//go:build !windows

package terminal

import "os"

// EnableOutput needs no console-mode changes on Unix terminals.
func EnableOutput(_ *os.File) (func() error, error) {
	return func() error { return nil }, nil
}
