package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUserSelectionDoesNotCreateIdentity(t *testing.T) {
	dir := t.TempDir()
	selection, err := loadUserSelection(dir, "/workspace", "", "")
	if err != nil || selection != (userSelection{}) {
		t.Fatalf("unexpected selection: %+v %v", selection, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("no selection should create no files or identity")
	}
}

func TestUserSelectionRemembersExistingIDsPerWorkspace(t *testing.T) {
	dir := t.TempDir()
	want := userSelection{"66f600000000000000000001", "66f600000000000000000002"}
	got, err := loadUserSelection(dir, "/workspace", want.UserID, want.ProjectID)
	if err != nil || got != want {
		t.Fatalf("save: %+v %v", got, err)
	}
	got, err = loadUserSelection(dir, "/workspace", "", "")
	if err != nil || got != want {
		t.Fatalf("load: %+v %v", got, err)
	}
	got, err = loadUserSelection(dir, "/another", "", "")
	if err != nil || got != (userSelection{}) {
		t.Fatalf("workspace selection leaked: %+v %v", got, err)
	}
	info, err := os.Stat(filepath.Join(dir, "cortisol", "users.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("file permissions: %v", err)
	}
}

func TestUserSelectionRejectsPartialOrInvalidIDsWithoutSaving(t *testing.T) {
	for _, selection := range []userSelection{{"66f600000000000000000001", ""}, {"bad", "66f600000000000000000002"}, {"000000000000000000000000", "66f600000000000000000002"}} {
		dir := t.TempDir()
		if _, err := loadUserSelection(dir, "/workspace", selection.UserID, selection.ProjectID); err == nil {
			t.Fatal("invalid selection accepted")
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 0 {
			t.Fatal("invalid selection was saved")
		}
	}
}
