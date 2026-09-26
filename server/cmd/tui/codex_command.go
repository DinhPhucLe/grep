package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func codexCommand(ctx context.Context, args ...string) (*exec.Cmd, error) {
	bin, err := exec.LookPath("codex")
	if runtime.GOOS != "windows" {
		if err != nil {
			return nil, fmt.Errorf("codex not on PATH: %w", err)
		}
		return exec.CommandContext(ctx, bin, args...), nil
	}
	if err == nil && (strings.EqualFold(filepath.Ext(bin), ".exe") || strings.EqualFold(filepath.Ext(bin), ".com")) {
		return exec.CommandContext(ctx, bin, args...), nil
	}

	// Go does not execute npm's PowerShell/batch launchers directly. Use
	// their Node entrypoint, preserving arguments without a command shell.
	var directories []string
	if err == nil {
		directories = append(directories, filepath.Dir(bin))
	}
	// Existing terminals can retain PATH from before npm was installed.
	if appData := os.Getenv("APPDATA"); filepath.IsAbs(appData) {
		directories = append(directories, filepath.Join(appData, "npm"))
	}
	for _, directory := range directories {
		for _, entry := range []string{
			filepath.Join(directory, "node_modules", "@openai", "codex", "bin", "codex.js"),
			filepath.Join(directory, "..", "@openai", "codex", "bin", "codex.js"),
		} {
			info, statErr := os.Stat(entry)
			if statErr != nil || !info.Mode().IsRegular() {
				continue
			}
			node := filepath.Join(directory, "node.exe")
			if info, statErr := os.Stat(node); statErr != nil || !info.Mode().IsRegular() {
				node, err = exec.LookPath("node.exe")
				if err != nil {
					return nil, fmt.Errorf("Codex npm installation found, but node.exe is not on PATH: %w", err)
				}
			}
			return exec.CommandContext(ctx, node, append([]string{entry}, args...)...), nil
		}
	}
	return nil, fmt.Errorf("Codex executable or npm installation not found; install with npm.cmd install -g @openai/codex and restart your terminal")
}
