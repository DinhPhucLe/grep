package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"cortisol-server/internal/participant"
)

func TestLoadParticipantIDPersistsPrivateIdentity(t *testing.T) {
	dir := t.TempDir()
	id, err := loadParticipantID(dir)
	if err != nil {
		t.Fatal(err)
	}
	again, err := loadParticipantID(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !participant.ValidID(id) || again != id {
		t.Fatalf("identity did not persist: %q %q", id, again)
	}
	path := filepath.Join(dir, "cortisol", "participant.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 1 || fields["id"] != id {
		t.Fatalf("unexpected identity data: %s", data)
	}
	for _, tc := range []struct {
		path string
		mode os.FileMode
	}{{filepath.Dir(path), 0700}, {path, 0600}} {
		info, err := os.Stat(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != tc.mode {
			t.Errorf("%s permissions %o", tc.path, info.Mode().Perm())
		}
	}
}

func TestLoadParticipantIDConcurrentCreation(t *testing.T) {
	dir := t.TempDir()
	start := make(chan struct{})
	ids := make(chan string, 24)
	errs := make(chan error, 24)
	var wg sync.WaitGroup
	for range 24 {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; id, err := loadParticipantID(dir); ids <- id; errs <- err }()
	}
	close(start)
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first string
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatalf("identity race: %s vs %s", first, id)
		}
	}
	entries, err := os.ReadDir(filepath.Join(dir, "cortisol"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary files left: %v", entries)
	}
}

func TestLoadParticipantIDRejectsCorruptionWithoutOverwrite(t *testing.T) {
	for _, bad := range []string{`{`, `{"id":"invalid"}`, `null`, `{"id":"12345678-1234-4123-8123-123456789abc"} {}`} {
		dir := t.TempDir()
		path := filepath.Join(dir, "cortisol", "participant.json")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadParticipantID(dir); err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "restore") {
			t.Fatalf("expected actionable corruption error, got %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != bad {
			t.Fatal("corrupt identity overwritten")
		}
	}
}
