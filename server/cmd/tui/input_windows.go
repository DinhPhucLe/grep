//go:build windows

package main

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
)

// Bubble Tea v1's special reader for os.Stdin emits Windows console records
// as individual keys and does not decode bracketed-paste boundaries. A separate
// console handle keeps its raw-mode/restoration logic but selects its ANSI
// reader, which emits the whole paste as a single KeyMsg with Paste=true.
func terminalInputOptions() ([]tea.ProgramOption, func(), error) {
	input, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return nil, func() {}, err
	}
	return []tea.ProgramOption{tea.WithInput(input)}, func() { _ = input.Close() }, nil
}

// The ANSI reader does not emit native WINDOW_BUFFER_SIZE_EVENT records.
// Poll the visible terminal size so resizing still works with this input path.
func pollTerminalSize() tea.Msg {
	w, h, err := term.GetSize(os.Stdout.Fd())
	if err != nil {
		return nil
	}
	return terminalSizeMsg{width: w, height: h}
}
