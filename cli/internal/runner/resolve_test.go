package runner

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestResolveWindowsNativePreservesArguments(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "codex.exe"))
	touch(t, filepath.Join(dir, "codex.cmd"))
	args := []string{"--", "a prompt with spaces", `"quotes"`, `a&b|c;$(no)`, "", "日本語"}
	cmd, err := resolve(args, "windows", dir)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path != filepath.Join(dir, "codex.exe") {
		t.Fatalf("path = %q", cmd.Path)
	}
	if !reflect.DeepEqual(cmd.Args[1:], args) {
		t.Fatalf("args = %#v", cmd.Args)
	}
}

func TestResolveWindowsNPMLayouts(t *testing.T) {
	for _, layout := range []string{"global", "local"} {
		t.Run(layout, func(t *testing.T) {
			base := t.TempDir()
			dir := base
			entry := filepath.Join(base, "node_modules", "@openai", "codex", "bin", "codex.js")
			if layout == "local" {
				dir = filepath.Join(base, "node_modules", ".bin")
				entry = filepath.Join(base, "node_modules", "@openai", "codex", "bin", "codex.js")
			}
			nodeDir := filepath.Join(base, "node runtime")
			touch(t, filepath.Join(dir, "codex.cmd"))
			touch(t, entry)
			touch(t, filepath.Join(nodeDir, "node.exe"))
			args := []string{"--", "hello & goodbye", `$(touch unexpected)`, ""}
			cmd, err := resolve(args, "windows", dir+";"+nodeDir)
			if err != nil {
				t.Fatal(err)
			}
			if cmd.Path != filepath.Join(nodeDir, "node.exe") {
				t.Fatalf("path = %q", cmd.Path)
			}
			if !reflect.DeepEqual(cmd.Args[1:], append([]string{entry}, args...)) {
				t.Fatalf("args = %#v", cmd.Args)
			}
		})
	}
}

func TestResolveRejectsUnsupportedShim(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "codex.cmd"))
	if _, err := resolve(nil, "windows", dir); err == nil {
		t.Fatal("accepted arbitrary shell shim")
	}
	touch(t, filepath.Join(dir, "node_modules", "@openai", "codex", "bin", "codex.js"))
	if _, err := resolve(nil, "windows", dir); err == nil || !strings.Contains(err.Error(), "node.exe") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveSkipsDirectoriesAndEmptyPATHEntries(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "codex.exe"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := resolve(nil, "windows", ";"+dir+";"); err == nil {
		t.Fatal("directory accepted as executable")
	}
}

func TestResolveUnixExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not preserve Unix executable permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "codex")
	if err := os.WriteFile(path, []byte("fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolve(nil, "linux", dir); err == nil {
		t.Fatal("accepted non-executable file")
	}
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
	cmd, err := resolve([]string{"hello"}, "linux", dir)
	if err != nil || cmd.Path != path {
		t.Fatalf("command = %v, error = %v", cmd, err)
	}
}
