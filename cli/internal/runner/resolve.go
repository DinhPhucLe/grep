package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Resolve locates Codex without a shell, preserving every argument verbatim.
func Resolve(args []string) (*exec.Cmd, error) {
	return resolve(args, runtime.GOOS, os.Getenv("PATH"))
}

func resolve(args []string, goos, searchPath string) (*exec.Cmd, error) {
	separator := ":"
	if goos == "windows" {
		separator = ";"
	}
	directories := strings.Split(searchPath, separator)
	for _, directory := range directories {
		directory = strings.Trim(directory, "\"")
		if directory == "" {
			continue
		}
		if goos != "windows" {
			candidate := filepath.Join(directory, "codex")
			if executable(candidate, goos) {
				return command(candidate, args)
			}
			continue
		}
		for _, name := range []string{"codex.exe", "codex.com"} {
			candidate := filepath.Join(directory, name)
			if executable(candidate, goos) {
				return command(candidate, args)
			}
		}
		if !executable(filepath.Join(directory, "codex.cmd"), goos) && !executable(filepath.Join(directory, "codex.ps1"), goos) {
			continue
		}
		// npm's shims require a shell. Invoke their known JS entrypoint with
		// node.exe instead, so quotes and shell metacharacters stay arguments.
		for _, entrypoint := range []string{
			filepath.Join(directory, "node_modules", "@openai", "codex", "bin", "codex.js"),
			filepath.Join(directory, "..", "@openai", "codex", "bin", "codex.js"),
		} {
			if !executable(entrypoint, goos) {
				continue
			}
			node := findNode(directory, directories)
			if node == "" {
				return nil, fmt.Errorf("Codex's npm installation was found, but node.exe was not found on PATH")
			}
			return command(node, append([]string{entrypoint}, args...))
		}
	}
	return nil, fmt.Errorf("Codex was not found on PATH; install the Codex CLI (npm install -g @openai/codex), then try again")
}

func findNode(shimDirectory string, directories []string) string {
	for _, directory := range append([]string{shimDirectory}, directories...) {
		directory = strings.Trim(directory, "\"")
		if directory == "" {
			continue
		}
		candidate := filepath.Join(directory, "node.exe")
		if executable(candidate, "windows") {
			return candidate
		}
	}
	return ""
}

func executable(path, goos string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && (goos == "windows" || info.Mode()&0111 != 0)
}

func command(path string, args []string) (*exec.Cmd, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return exec.Command(absolute, args...), nil
}
