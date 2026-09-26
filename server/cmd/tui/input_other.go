//go:build !windows

package main

import tea "github.com/charmbracelet/bubbletea"

func terminalInputOptions() ([]tea.ProgramOption, func(), error) { return nil, func() {}, nil }
func pollTerminalSize() tea.Msg                                  { return nil }
