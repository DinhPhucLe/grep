package main

import (
	"strings"
	"testing"
	"time"

	"cortisol-server/mascot"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestMascotVisibleByDefault(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	if !m.mascotOn || m.mascot == nil {
		t.Fatal("mascot should appear by default")
	}
	m.width, m.height = 80, 24
	m.resize()
	view := m.View()
	if m.mascotBounds.W == 0 {
		t.Fatal("expected mascot bounds after View")
	}
	if !strings.Contains(view, "▀") && !strings.Contains(view, "▄") && !strings.Contains(view, "█") {
		t.Fatalf("expected half-block cat in view, bounds=%+v", m.mascotBounds)
	}
}

func TestMascotHiddenOnNarrowTerminal(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.Update(tea.WindowSizeMsg{Width: 20, Height: 8})
	m.View()
	if m.mascotLaneRows() != 0 || m.mascotBounds.W != 0 {
		t.Fatal("mascot should hide when terminal is too small")
	}
}

func TestMascotFeetAboveInputBorder(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.width, m.height = 80, 24
	m.resize()
	m.View()
	feetY := m.mascotBounds.Y + m.mascotBounds.H - 1
	if feetY+1 != m.composerBorderY() {
		t.Fatalf("feet Y %d should sit immediately above border %d", feetY, m.composerBorderY())
	}
}

func TestMascotTracksComposerWhenDraftGrows(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.width, m.height = 80, 24
	m.resize()
	m.View()
	y1 := m.composerBorderY()
	m.draft.SetValue("line1\nline2\nline3\nline4")
	m.resize()
	m.View()
	y2 := m.composerBorderY()
	feet := m.mascotBounds.Y + m.mascotBounds.H - 1
	if feet+1 != y2 {
		t.Fatalf("feet not glued to border after resize")
	}
	if y2 >= y1 {
		t.Fatalf("border should move up when draft grows: %d -> %d", y1, y2)
	}
}

func TestMascotDoubleClickOpaqueHides(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.width, m.height = 80, 24
	m.resize()
	m.View()
	sprite := m.mascotSprite()
	var ox, oy int
	found := false
	for y := 0; y < m.mascotBounds.H; y++ {
		for x := 0; x < m.mascotBounds.W; x++ {
			if sprite.OpaqueCell(x, y) {
				ox, oy = m.mascotBounds.X+x, m.mascotBounds.Y+y
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatal("no opaque cell")
	}
	m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: ox, Y: oy})
	if !m.mascotOn {
		t.Fatal("single opaque click must not hide")
	}
	m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: ox, Y: oy})
	if m.mascotOn {
		t.Fatal("double-click on opaque cell should hide")
	}
}

func TestMascotTransparentClickPassesThrough(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.width, m.height = 80, 24
	m.resize()
	m.View()
	sprite := m.mascotSprite()
	var tx, ty int
	found := false
	for y := 0; y < m.mascotBounds.H; y++ {
		for x := 0; x < m.mascotBounds.W; x++ {
			if !sprite.OpaqueCell(x, y) {
				tx, ty = m.mascotBounds.X+x, m.mascotBounds.Y+y
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatal("no transparent cell")
	}
	handled := m.mascotMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: tx, Y: ty})
	if handled {
		t.Fatal("transparent click must not be consumed")
	}
	if !m.mascotOn {
		t.Fatal("transparent click must not hide")
	}
}

func TestMascotReducedMotionStatic(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.width, m.height = 80, 24
	m.resize()
	a := mascot.Render(m.mascotSprite(), "", true)
	for i := 0; i < 40; i++ {
		m.Update(frameMsg(time.Now()))
	}
	b := mascot.Render(m.mascotSprite(), "", true)
	if a != b {
		t.Fatal("reduced motion should keep idle frame")
	}
}

func TestMascotDoesNotPaintOnInputBorder(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true, ReducedMotion: true})
	m.width, m.height = 80, 24
	m.resize()
	view := m.View()
	lines := strings.Split(view, "\n")
	borderY := m.composerBorderY()
	if borderY < 0 || borderY >= len(lines) {
		t.Fatalf("bad borderY %d", borderY)
	}
	line := ansi.Strip(lines[borderY])
	if !strings.Contains(line, "─") && !strings.Contains(line, "┌") {
		t.Fatalf("border row missing border glyphs: %q", line)
	}
}
