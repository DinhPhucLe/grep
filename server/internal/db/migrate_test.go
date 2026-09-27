package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CHECK THE COMMITTED MIGRATIONS WITHOUT CONNECTING TO ATLAS
func TestCheckProjectMigrations(t *testing.T) {
	if err := CheckMigrations("migrations"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckMigrationsDuplicateVersions(t *testing.T) {
	for _, suffix := range []string{"up", "down"} {
		t.Run(suffix, func(t *testing.T) {
			dir := t.TempDir()
			names := []string{"000001_a.up.json", "000001_a.down.json", "000001_b." + suffix + ".json"}
			if suffix == "up" {
				names = append(names, "000001_b.down.json")
			}
			for _, name := range names {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(`[{"create":"test"}]`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := CheckMigrations(dir); err == nil || !strings.Contains(err.Error(), "duplicate migration file") {
				t.Fatalf("expected duplicate migration error, got %v", err)
			}
		})
	}
}

// REJECT BROKEN FILES BEFORE ANY DATABASE CHANGES
func TestCheckMigrationsInvalidFiles(t *testing.T) {
	for _, test := range []struct {
		name string
		up   string
		down string
	}{
		{"empty_directory", "", ""},
		{"missing_down", `[{"create":"test"}]`, ""},
		{"invalid_json", `[`, `[{"drop":"test"}]`},
		{"not_array", `{}`, `[{"drop":"test"}]`},
		{"empty_array", `[]`, `[{"drop":"test"}]`},
		{"empty_command", `[{}]`, `[{"drop":"test"}]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			for suffix, data := range map[string]string{"up": test.up, "down": test.down} {
				if data != "" {
					if err := os.WriteFile(filepath.Join(dir, "000001_test."+suffix+".json"), []byte(data), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := CheckMigrations(dir); err == nil {
				t.Fatal("expected invalid migration files to fail")
			}
		})
	}
}
