package main

import (
	"strings"
	"time"

	"cortisol-server/mascot"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// mascotMouse: only a double-click on an opaque cat cell hides it.
// Single clicks and transparent cells pass through to other handlers.
func (m *model) mascotMouse(v tea.MouseMsg) bool {
	if !m.mascotOn || m.mascot == nil || m.mascotBounds.W == 0 {
		return false
	}
	if v.Button != tea.MouseButtonLeft || v.Action != tea.MouseActionPress {
		return false
	}
	if !m.mascotBounds.Contains(v.X, v.Y) {
		return false
	}
	relX := v.X - m.mascotBounds.X
	relY := v.Y - m.mascotBounds.Y
	sprite := m.mascotSprite()
	if sprite == nil || !sprite.OpaqueCell(relX, relY) {
		return false
	}
	now := time.Now()
	same := v.X == m.mascotClickX && v.Y == m.mascotClickY
	if same && !m.mascotClickAt.IsZero() && now.Sub(m.mascotClickAt) <= time.Duration(mascot.DoubleClickWindowMs)*time.Millisecond {
		m.mascotOn = false
		m.mascotClickAt = time.Time{}
		m.mascotBounds = mascot.Bounds{}
		m.resize()
		return true
	}
	m.mascotClickAt = now
	m.mascotClickX = v.X
	m.mascotClickY = v.Y
	return false
}

func (m *model) mascotSprite() mascot.Sprite {
	if m.mascot == nil {
		return nil
	}
	if m.opts.ReducedMotion {
		return m.mascot.IdleSprite()
	}
	return m.mascot.CurrentSprite()
}

func (m *model) mascotFits() bool {
	if !m.mascotOn || m.mascot == nil {
		return false
	}
	cellW, cellH := mascot.CellSize()
	if m.width < cellW+4 {
		return false
	}
	// header + min conversation + lane + optional status + border + input + footer
	need := m.headerRows() + 2 + cellH + 1 + m.borderRows() + m.footerRows()
	return m.height >= need
}

// Blank rows between status and the composer border; feet sit on the last row.
func (m *model) mascotLaneRows() int {
	if !m.mascotFits() {
		return 0
	}
	_, cellH := mascot.CellSize()
	return cellH
}

// Screen Y of the composer's top border (first border line).
func (m *model) composerBorderY() int {
	y := m.headerRows() + m.viewport.Height
	if m.statusText() != "" {
		y++
	}
	y += m.mascotLaneRows()
	return y
}

func (m *model) syncMascotRange() {
	if m.mascot == nil || !m.mascotFits() {
		return
	}
	cellW, _ := mascot.CellSize()
	margin := 1
	minX := max(0, m.width/2) // stay on the right half
	maxX := m.width - cellW - margin
	if maxX < minX {
		minX = max(0, maxX)
	}
	m.mascot.SetRange(minX, maxX)
}

// Paint into the reserved lane only — never onto the input border or text.
func (m *model) overlayMascot(screen string) string {
	m.mascotBounds = mascot.Bounds{}
	if !m.mascotFits() || m.showHelp {
		return screen
	}
	sprite := m.mascotSprite()
	if sprite == nil {
		return screen
	}
	cellW, cellH := mascot.CellSize()
	m.syncMascotRange()
	x := m.mascot.X()
	originY := m.composerBorderY() - cellH
	if originY < m.headerRows() {
		return screen
	}
	art := mascot.RenderLines(sprite, m.mascot.Color, m.opts.NoColor)
	lines := strings.Split(screen, "\n")
	borderY := m.composerBorderY()
	for i := 0; i < cellH && i < len(art); i++ {
		y := originY + i
		if y < 0 || y >= len(lines) || y >= borderY {
			continue
		}
		lines[y] = placeFrag(lines[y], x, art[i], m.width)
	}
	m.mascotBounds = mascot.Bounds{X: x, Y: originY, W: cellW, H: cellH}
	return strings.Join(lines, "\n")
}

func placeFrag(line string, x int, frag string, screenW int) string {
	if x < 0 {
		x = 0
	}
	fragW := ansi.StringWidth(frag)
	if fragW <= 0 {
		return ansi.Truncate(line, screenW, "")
	}
	base := line
	if ansi.StringWidth(base) < screenW {
		base += strings.Repeat(" ", screenW-ansi.StringWidth(base))
	}
	base = ansi.Truncate(base, screenW, "")
	left := ansi.Cut(base, 0, x)
	right := ""
	if x+fragW < ansi.StringWidth(base) {
		right = ansi.Cut(base, x+fragW, ansi.StringWidth(base))
	}
	return ansi.Truncate(left+frag+right, screenW, "")
}
