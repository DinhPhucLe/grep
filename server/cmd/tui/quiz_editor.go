package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type quizSourceOpenedMsg struct {
	session            *quizSession
	question, sequence int
	err                error
}

// Launch without a shell or --wait: the TUI keeps accepting input while VS Code
// opens the existing file in its editor, at the evidence's starting line.
func openVSCodeSource(ctx context.Context, path string, line int) error {
	code, err := exec.LookPath("code")
	if err != nil {
		return fmt.Errorf("install VS Code's 'code' command in PATH, or open the reference manually")
	}
	cmd := exec.CommandContext(ctx, code, "--reuse-window", "--goto", fmt.Sprintf("%s:%d", path, line))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("VS Code could not open the reference; open it manually (%v)", err)
	}
	return nil
}

func quizSourcePath(workspace, path string) (string, error) {
	if !filepath.IsLocal(path) {
		return "", fmt.Errorf("source reference is outside the workspace")
	}
	root, err := filepath.Abs(workspace)
	if err == nil {
		root, err = filepath.EvalSymlinks(root)
	}
	if err != nil {
		return "", fmt.Errorf("workspace is unavailable")
	}
	target, err := filepath.EvalSymlinks(filepath.Join(root, path))
	if err != nil {
		return "", fmt.Errorf("referenced file is unavailable: %s", path)
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || !filepath.IsLocal(relative) {
		return "", fmt.Errorf("source reference is outside the workspace")
	}
	info, err := os.Stat(target)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("source reference is not a regular file: %s", path)
	}
	return target, nil
}

func (m *model) openQuizSource() tea.Cmd {
	q := m.quiz
	if !m.quizActive() || (q.phase != "question" && q.phase != "reveal") || q.index >= len(q.result.Questions) {
		return nil
	}
	refs := q.result.Questions[q.index].Evidence
	if len(refs) == 0 {
		return nil
	}
	ref := refs[q.sourceIndex]
	q.editorSequence++
	index, sequence := q.index, q.editorSequence
	if q.editorCancel != nil {
		q.editorCancel()
	}
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
	q.editorCancel = cancel
	workspace, openSource := m.workspace, m.openSource
	return func() tea.Msg {
		defer cancel()
		err := ctx.Err()
		var path string
		if err == nil {
			path, err = quizSourcePath(workspace, ref.FilePath)
		}
		if err == nil {
			err = ctx.Err()
		}
		if err == nil {
			err = openSource(ctx, path, ref.StartLine)
		}
		return quizSourceOpenedMsg{session: q, question: index, sequence: sequence, err: err}
	}
}

func (m *model) nextQuizSource() tea.Cmd {
	if !m.quizActive() || (m.quiz.phase != "question" && m.quiz.phase != "reveal") {
		return nil
	}
	q := m.quiz
	refs := q.result.Questions[q.index].Evidence
	if len(refs) == 0 {
		return nil
	}
	q.sourceIndex = (q.sourceIndex + 1) % len(refs)
	q.editorNotice = ""
	m.showQuizQuestion(q.feedback)
	return m.openQuizSource()
}

func (m *model) handleQuizSourceOpened(msg quizSourceOpenedMsg) tea.Cmd {
	q := m.quiz
	if !m.quizActive() || q != msg.session || q.index != msg.question || q.editorSequence != msg.sequence {
		return nil
	}
	if msg.err != nil {
		q.editorNotice = "Could not open source: " + msg.err.Error()
		m.showQuizQuestion(q.feedback)
	}
	return nil
}
