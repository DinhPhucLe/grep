package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"cortisol-server/internal/quiz"
)

type fileSnapshot map[string][32]byte

// Git discovers tracked and untracked, nonignored files; hashing the baseline
// avoids attributing preexisting dirty changes to this Codex turn.
func snapshotWorkspace(ctx context.Context, workspace string) (fileSnapshot, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	cmd.Dir = workspace
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, nil
	} // Non-Git workspaces use explicit fileChange events.
	paths := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	if len(paths) > 20000 {
		return nil, fmt.Errorf("workspace has too many files to snapshot")
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	snapshot := fileSnapshot{}
	for _, path := range paths {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if path == "" {
			continue
		}
		content, err := readQuizFile(root, path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			continue
		} // Explicit file-change reads below report unsupported files.
		snapshot[path] = sha256.Sum256([]byte(content))
	}
	return snapshot, nil
}

func readQuizFile(root *os.Root, path string) (string, error) {
	info, err := root.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file: %s", path)
	}
	file, err := root.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file: %s", path)
	}
	data, err := io.ReadAll(io.LimitReader(file, 128001))
	if err != nil {
		return "", err
	}
	if len(data) > 128000 || !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
		return "", fmt.Errorf("file is too large or not text: %s", path)
	}
	return string(data), nil
}

func collectQuizFiles(ctx context.Context, workspace string, before fileSnapshot, changes []string) ([]quiz.File, error) {
	paths := map[string]bool{}
	after, err := snapshotWorkspace(ctx, workspace)
	if err != nil {
		return nil, err
	}
	if before != nil {
		for path, hash := range after {
			if old, ok := before[path]; !ok || old != hash {
				paths[path] = true
			}
		}
	}
	for _, raw := range changes {
		var items []struct {
			Path string `json:"path"`
			Kind struct {
				Type     string `json:"type"`
				MovePath string `json:"move_path"`
			} `json:"kind"`
		}
		if json.Unmarshal([]byte(raw), &items) != nil {
			return nil, fmt.Errorf("could not decode generated file changes")
		}
		for _, item := range items {
			if item.Kind.Type == "delete" {
				continue
			}
			path := item.Path
			if item.Kind.MovePath != "" {
				path = item.Kind.MovePath
			}
			if filepath.IsAbs(path) {
				path, err = filepath.Rel(workspace, path)
				if err != nil {
					return nil, err
				}
			}
			if !filepath.IsLocal(path) {
				return nil, fmt.Errorf("generated path is outside workspace")
			}
			paths[filepath.ToSlash(filepath.Clean(path))] = true
		}
	}
	if len(paths) > 30 {
		return nil, fmt.Errorf("more than 30 files changed; quiz cannot cover this turn")
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	var files []quiz.File
	for _, path := range ordered {
		content, err := readQuizFile(root, path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("cannot collect %s: %w", path, err)
		}
		if strings.TrimSpace(content) != "" {
			files = append(files, quiz.File{Path: path, Content: content})
		}
	}
	return files, nil
}
