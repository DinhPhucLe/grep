package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"cortisol-server/internal/quiz"
	tea "github.com/charmbracelet/bubbletea"
)

func TestQuizOpensSourceOncePerQuestionAndCyclesReferences(t *testing.T) {
	m := generatedFilesQuizModel(t, "")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	other := "file with spaces.go"
	if err := os.WriteFile(filepath.Join(m.workspace, other), []byte("other source\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var opened []string
	m.openSource = func(ctx context.Context, path string, line int) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("editor launch lacks a deadline")
		}
		opened = append(opened, filepath.Base(path))
		wantLine := []int{2, 1, 2, 4}[len(opened)-1]
		if line != wantLine {
			t.Errorf("opened line %d, want %d", line, wantLine)
		}
		return nil
	}
	q := m.quiz
	q.phase = "generating"
	q.panel = &conversationItem{kind: "quizPanel"}
	result := twoQuestions()
	result.Questions[0].Evidence = append(result.Questions[0].Evidence, quiz.Evidence{FilePath: other, StartLine: 1, EndLine: 1})
	response := quizGeneratedMsg{session: q, result: result}
	runEditor := func(cmd tea.Cmd) {
		t.Helper()
		if cmd == nil {
			t.Fatal("no editor command")
		}
		msg := cmd()
		if openedMsg, ok := msg.(quizSourceOpenedMsg); !ok || openedMsg.err != nil {
			t.Fatalf("editor failed: %+v", msg)
		}
		m.Update(msg)
	}
	_, cmd := m.Update(response)
	runEditor(cmd)
	for _, msg := range []tea.Msg{response, tea.WindowSizeMsg{Width: 120, Height: 40}} {
		if _, cmd := m.Update(msg); cmd != nil {
			t.Fatal("replayed result or resize launched editor again")
		}
		m.View()
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	runEditor(cmd)
	if !strings.Contains(q.panel.raw, "> "+other+":1–1") {
		t.Fatal("active source reference not shown")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	runEditor(cmd)
	if cmd := m.quizEnter("my answer"); cmd != nil {
		t.Fatal("answer launched external work")
	}
	runEditor(m.quizEnter(""))
	if strings.Join(opened, ",") != "main.go,file with spaces.go,main.go,main.go" {
		t.Fatalf("wrong editor targets: %v", opened)
	}
	for _, snippet := range []string{"one\ntwo", "other source", "[hidden for quiz]"} {
		if strings.Contains(q.panel.raw, snippet) {
			t.Fatalf("quiz rendered source/masking: %s", snippet)
		}
	}
}

func TestEditorFailureKeepsQuizUsableAndIgnoresStaleReplies(t *testing.T) {
	m := generatedFilesQuizModel(t, "")
	m.quiz.result = twoQuestions()
	m.quiz.phase = "question"
	m.quiz.panel = &conversationItem{kind: "quizPanel"}
	m.openSource = func(context.Context, string, int) error { return errors.New("editor unavailable") }
	m.showQuizQuestion("")
	old := m.openQuizSource()()
	m.Update(old)
	if !m.quizActive() || !strings.Contains(m.quiz.panel.raw, "editor unavailable") {
		t.Fatal("launch error blocked quiz or was hidden")
	}
	m.quizEnter("record this answer")
	if !strings.Contains(m.quiz.panel.raw, "record this answer") {
		t.Fatal("could not answer after editor failure")
	}
	m.quizEnter("")
	m.Update(old)
	if m.quiz.index != 1 || strings.Contains(m.quiz.panel.raw, "editor unavailable") {
		t.Fatal("old editor reply changed the next question")
	}
	late := m.openQuizSource()()
	m.quizEnter("/reveal")
	m.Update(late)
	if m.quizActive() || strings.Contains(m.quiz.panel.raw, "editor unavailable") {
		t.Fatal("late editor reply changed finished review")
	}
}

func TestQuizEditorRejectsMissingAndExternalPaths(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("code"), 0600); err != nil {
		t.Fatal(err)
	}
	paths := []string{"../outside.go", outside, "missing.go", "."}
	if err := os.Symlink(outside, filepath.Join(root, "escape.go")); err == nil {
		paths = append(paths, "escape.go")
	} else if runtime.GOOS != "windows" {
		t.Fatal(err)
	}
	for _, path := range paths {
		if _, err := quizSourcePath(root, path); err == nil {
			t.Errorf("accepted invalid editor path: %s", path)
		}
	}
}

func TestQuizCancelsObsoleteEditorLaunches(t *testing.T) {
	m := generatedFilesQuizModel(t, "")
	q := m.quiz
	q.result = twoQuestions()
	q.phase = "question"
	q.panel = &conversationItem{kind: "quizPanel"}
	started := make(chan struct{})
	m.openSource = func(ctx context.Context, _ string, _ int) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	first := m.openQuizSource()
	done := make(chan tea.Msg, 1)
	go func() { done <- first() }()
	<-started
	m.quizEnter("answer")
	next := m.quizEnter("")
	select {
	case msg := <-done:
		if !errors.Is(msg.(quizSourceOpenedMsg).err, context.Canceled) {
			t.Fatal("old editor launch did not receive cancellation")
		}
		m.Update(msg)
	case <-time.After(time.Second):
		t.Fatal("advancing left previous editor running")
	}
	m.quizEnter("/reveal")
	// Even if Bubble Tea only executes this command after the quiz ends, it
	// must not launch VS Code. Calling the fake opener again would also panic.
	msg := next().(quizSourceOpenedMsg)
	if !errors.Is(msg.err, context.Canceled) {
		t.Fatal("queued editor command ran after quiz ended")
	}
}

func TestVSCodeCLIUsesExistingWindowAndExactLocation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix CLI fixture")
	}
	dir := t.TempDir()
	output := filepath.Join(dir, "args")
	// A fake executable checks argument boundaries without opening the user's GUI.
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$QUIZ_EDITOR_TEST_ARGS\"\n"
	if err := os.WriteFile(filepath.Join(dir, "code"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("QUIZ_EDITOR_TEST_ARGS", output)
	path := filepath.Join(dir, "space $(touch NEVER).go")
	if err := openVSCodeSource(context.Background(), path, 17); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(output)
	if err != nil || string(got) != "--reuse-window\n--goto\n"+path+":17\n" {
		t.Fatalf("wrong VS Code arguments: %q (%v)", got, err)
	}
	t.Setenv("PATH", t.TempDir())
	if err := openVSCodeSource(context.Background(), path, 17); err == nil || !strings.Contains(err.Error(), "PATH") {
		t.Fatal("missing CLI has no actionable error")
	}
}
