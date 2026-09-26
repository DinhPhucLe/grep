package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCodexCommandWindowsNPM(t *testing.T) {
	for _, onPath := range []bool{true, false} {
		name := "stale PATH"
		if onPath {
			name = "npm shim on PATH"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			npm := filepath.Join(root, "npm")
			entry := filepath.Join(npm, "node_modules", "@openai", "codex", "bin", "codex.js")
			if err := os.MkdirAll(filepath.Dir(entry), 0700); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{entry, filepath.Join(npm, "codex.cmd"), filepath.Join(root, "node.exe")} {
				if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			search := root
			if onPath {
				search = npm + ";" + root
			}
			t.Setenv("PATH", search)
			t.Setenv("PATHEXT", ".EXE;.CMD")
			t.Setenv("APPDATA", root)
			cmd, err := codexCommand(context.Background(), "app-server", "--listen", "stdio://")
			if err != nil {
				t.Fatal(err)
			}
			want := []string{filepath.Join(root, "node.exe"), entry, "app-server", "--listen", "stdio://"}
			if !reflect.DeepEqual(cmd.Args, want) {
				t.Fatalf("got %q, want %q", cmd.Args, want)
			}
		})
	}
}
