package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGradedAnswerMigrationRequiresConciseScoredRecord(t *testing.T) {
	raw, err := os.ReadFile("migrations/000009_graded_quiz_answers.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []struct {
		CollMod         string `json:"collMod"`
		ValidationLevel string `json:"validationLevel"`
		Validator       struct {
			Schema struct {
				Required   []string                   `json:"required"`
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"$jsonSchema"`
		} `json:"validator"`
	}
	if err := json.Unmarshal(raw, &commands); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || commands[0].CollMod != "quiz_answers" || commands[0].ValidationLevel != "moderate" {
		t.Fatalf("unexpected migration: %+v", commands)
	}
	required := map[string]bool{}
	for _, name := range commands[0].Validator.Schema.Required {
		required[name] = true
	}
	for _, name := range []string{"_id", "user_id", "project_id", "quiz_id", "question_id", "answer", "quiz_question", "graded", "reasoning", "prompt", "created_at"} {
		if !required[name] {
			t.Fatalf("%s is not required", name)
		}
	}
	for _, name := range []string{"status", "request", "question", "model", "thread_id"} {
		if _, found := commands[0].Validator.Schema.Properties[name]; found {
			t.Fatalf("legacy field %s retained", name)
		}
	}
}

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
