package db

import (
	"os"
	"path/filepath"
	"testing"
)

// CHECK THE COMMITTED MIGRATIONS WITHOUT CONNECTING TO ATLAS
func TestCheckProjectMigrations(t *testing.T) {
	if err := CheckMigrations("migrations"); err != nil {
		t.Fatal(err)
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
