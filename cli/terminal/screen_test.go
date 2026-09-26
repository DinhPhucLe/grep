package terminal

import (
	"bytes"
	"strings"
	"testing"
)

func screenText(s *terminalState) []string {
	var rows []string
	for _, row := range s.screen.cells {
		var b strings.Builder
		for _, cell := range row {
			if cell.continuation {
				continue
			}
			if cell.text == "" {
				b.WriteByte(' ')
			} else {
				b.WriteString(cell.text)
			}
		}
		rows = append(rows, strings.TrimRight(b.String(), " "))
	}
	return rows
}

func feedScreen(t *testing.T, s *terminalState, value string) {
	t.Helper()
	var stream ansiStream
	for _, token := range stream.push(value) {
		if _, ok := s.accept(token); !ok {
			t.Fatalf("unsupported test sequence %q", token)
		}
	}
}

func TestChildScreenKeepsScrolledAndEditedOutput(t *testing.T) {
	s := newTerminalState(8, 3)
	feedScreen(t, s, "one\r\ntwo\r\nthree\r\nfour")
	if got := strings.Join(screenText(s), "|"); got != "two|three|four" {
		t.Fatal(got)
	}
	feedScreen(t, s, CSI+"2;2H"+CSI+"2P")
	if got := screenText(s)[1]; got != "tee" {
		t.Fatal(got)
	}
	feedScreen(t, s, CSI+"2@"+"hr")
	if got := screenText(s)[1]; got != "three" {
		t.Fatal(got)
	}
	feedScreen(t, s, CSI+"2;1H"+CSI+"L")
	if got := strings.Join(screenText(s), "|"); got != "two||three" {
		t.Fatal(got)
	}
	feedScreen(t, s, CSI+"M")
	if got := strings.Join(screenText(s), "|"); got != "two|three|" {
		t.Fatal(got)
	}
	feedScreen(t, s, CSI+"2;3H"+CSI+"K")
	if got := screenText(s)[1]; got != "th" {
		t.Fatal(got)
	}
	feedScreen(t, s, CSI+"2J")
	if got := strings.Join(screenText(s), ""); got != "" {
		t.Fatal(got)
	}
}

func TestNestedChildScreenRestoresTextStyleAndWideCharacters(t *testing.T) {
	s := newTerminalState(12, 3)
	s.ownedScreen = true
	feedScreen(t, s, CSI+"31m"+"A界e\u0301")
	normal := strings.Join(screenText(s), "|")
	feedScreen(t, s, CSI+"?1049h"+CSI+"H"+"alternate")
	s.resize(10, 4)
	out, ok := s.accept(CSI + "?1049l")
	if !ok || strings.Contains(out, "?1049l") {
		t.Fatal("nested screen escaped owned host buffer")
	}
	if !strings.Contains(out, CSI+"31m") || !strings.Contains(out, "A界e\u0301") {
		t.Fatalf("restore lost text/style: %q", out)
	}
	if got := strings.Join(screenText(s), "|"); got != normal+"|" {
		t.Fatal(got)
	}
	s.resize(2, 3)
	if got := screenText(s)[0]; got != "A" {
		t.Fatalf("cropped half-wide glyph survived: %q", got)
	}
}

func TestMetricPaintProbesSizeBeforeRunnerPoll(t *testing.T) {
	var output bytes.Buffer
	r := NewRenderer(&output, 80, 24, true)
	r.Start()
	r.Write([]byte("child text"))
	r.Update(snapshot(58))
	r.readSize = func() (int, int, error) { return 40, 24, nil }
	output.Reset()
	r.Update(snapshot(59))
	if c, h := r.Dimensions(); c != 40 || h != 24 {
		t.Fatalf("stale dimensions %dx%d", c, h)
	}
	if strings.Contains(output.String(), "VIBECODE") {
		t.Fatal("painted old-width status between resize polls")
	}
	if !strings.Contains(output.String(), "child text") {
		t.Fatal("hidden header lost child viewport")
	}
	if !strings.Contains(output.String(), CSI+"2J") {
		t.Fatal("resize did not invalidate complete viewport")
	}
}
