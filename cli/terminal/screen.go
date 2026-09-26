package terminal

import (
	"fmt"
	"strings"
)

// childScreen retains only the visible child viewport, not metric history or
// raw event logs. It lets a resize repaint without replaying child commands.
type screenCell struct {
	text, style  string
	continuation bool
}

type childScreen struct {
	cells  [][]screenCell
	normal [][]screenCell
}

func blankScreen(columns, rows int) [][]screenCell {
	cells := make([][]screenCell, rows)
	for y := range cells {
		cells[y] = make([]screenCell, columns)
	}
	return cells
}

func resizeCells(old [][]screenCell, columns, rows int) [][]screenCell {
	grid := blankScreen(columns, rows)
	for y := 0; y < min(rows, len(old)); y++ {
		copy(grid[y], old[y])
		// Do not retain half a double-width character at the cropped edge.
		if columns < len(old[y]) && old[y][columns].continuation {
			grid[y][columns-1] = screenCell{}
		}
	}
	return grid
}

func (g *childScreen) resize(columns, rows int) {
	g.cells = resizeCells(g.cells, columns, rows)
	if g.normal != nil {
		g.normal = resizeCells(g.normal, columns, rows)
	}
}

func (g *childScreen) scroll(top, bottom, n int) {
	top, bottom = max(0, top-1), min(len(g.cells), bottom)
	if top >= bottom {
		return
	}
	n = max(-(bottom - top), min(bottom-top, n))
	if n > 0 {
		copy(g.cells[top:bottom-n], g.cells[top+n:bottom])
		for y := bottom - n; y < bottom; y++ {
			g.cells[y] = make([]screenCell, len(g.cells[0]))
		}
	} else if n < 0 {
		copy(g.cells[top-n:bottom], g.cells[top:bottom+n])
		for y := top; y < top-n; y++ {
			g.cells[y] = make([]screenCell, len(g.cells[0]))
		}
	}
}

func (g *childScreen) clear(y, first, last int) {
	if y < 1 || y > len(g.cells) {
		return
	}
	row := g.cells[y-1]
	first, last = max(1, first), min(len(row), last)
	if first > last {
		return
	}
	if row[first-1].continuation && first > 1 {
		row[first-2] = screenCell{}
	}
	if last < len(row) && row[last].continuation {
		row[last] = screenCell{}
	}
	clear(row[first-1 : last])
}

func (g *childScreen) accept(token string, before cursor, wasAlternate bool, s *terminalState) {
	if wasAlternate != s.alternate {
		if s.alternate {
			g.normal = g.cells
			g.cells = blankScreen(s.columns, s.rows)
		} else {
			g.cells = g.normal
			g.normal = nil
			if g.cells == nil {
				g.cells = blankScreen(s.columns, s.rows)
			}
		}
		return
	}
	switch token {
	case "\n", "\v", "\f", "\x1bD", "\x1bE":
		if before.y == s.bottom {
			g.scroll(s.top, s.bottom, 1)
		}
		return
	case "\x1bM":
		if before.y == s.top {
			g.scroll(s.top, s.bottom, -1)
		}
		return
	}
	if strings.HasPrefix(token, CSI) {
		m := csiPattern.FindStringSubmatch(token)
		if m == nil || m[1] != "" || m[3] != "" {
			return
		}
		n := 0
		fmt.Sscanf(m[2], "%d", &n)
		count := max(1, n)
		x, y := before.x, before.y
		switch m[4] {
		case "J":
			for row := 1; row <= s.rows; row++ {
				switch {
				case n == 2:
					g.clear(row, 1, s.columns)
				case n == 0 && row > y:
					g.clear(row, 1, s.columns)
				case n == 0 && row == y:
					g.clear(row, x, s.columns)
				case n == 1 && row < y:
					g.clear(row, 1, s.columns)
				case n == 1 && row == y:
					g.clear(row, 1, x)
				}
			}
		case "K":
			switch n {
			case 0:
				g.clear(y, x, s.columns)
			case 1:
				g.clear(y, 1, x)
			case 2:
				g.clear(y, 1, s.columns)
			}
		case "X":
			g.clear(y, x, x+min(count, s.columns)-1)
		case "P", "@":
			row := g.cells[y-1]
			count = min(count, s.columns-x+1)
			if m[4] == "P" {
				copy(row[x-1:], row[x-1+count:])
				clear(row[s.columns-count:])
			} else {
				copy(row[x-1+count:], row[x-1:s.columns-count])
				clear(row[x-1 : x-1+count])
			}
		case "L":
			if y >= s.top && y <= s.bottom {
				g.scroll(y, s.bottom, -count)
			}
		case "M":
			if y >= s.top && y <= s.bottom {
				g.scroll(y, s.bottom, count)
			}
		case "S":
			g.scroll(s.top, s.bottom, count)
		case "T":
			g.scroll(s.top, s.bottom, -count)
		}
		return
	}
	if token == "" || token[0] < 32 || token[0] == 127 {
		return
	}
	x, y, wrap := before.x, before.y, before.wrap
	for _, r := range token {
		width, _ := runeWidth(r)
		if width == 0 {
			col := x - 2
			if wrap {
				col = x - 1
			}
			if col >= 0 && y <= s.rows {
				g.cells[y-1][col].text += string(r)
			}
			continue
		}
		if (wrap || x+width-1 > s.columns) && s.autoWrap {
			x = 1
			wrap = false
			if y == s.bottom {
				g.scroll(s.top, s.bottom, 1)
			} else {
				y = min(s.rows, y+1)
			}
		}
		if x+width-1 <= s.columns {
			g.clear(y, x, x+width-1)
			g.cells[y-1][x-1] = screenCell{text: string(r), style: before.style}
			if width == 2 {
				g.cells[y-1][x] = screenCell{continuation: true}
			}
		}
		x += width
		wrap = x > s.columns && s.autoWrap
		x = min(x, s.columns)
	}
}

// paint runs with host autowrap disabled. Exact cursor placement and full row
// clears remove old/reflowed glyphs without consuming the child's save slot.
func (g *childScreen) paint(offset int) string {
	var out strings.Builder
	style := ""
	out.WriteString(CSI + "0m")
	for y, row := range g.cells {
		out.WriteString(CSI + "0m")
		style = ""
		fmt.Fprintf(&out, "%s%d;1H%s2K", CSI, y+offset+1, CSI)
		last := len(row)
		for last > 0 && row[last-1].text == "" && !row[last-1].continuation {
			last--
		}
		for _, cell := range row[:last] {
			if cell.continuation {
				continue
			}
			if cell.style != style {
				out.WriteString(CSI + "0m" + cell.style)
				style = cell.style
			}
			if cell.text == "" {
				out.WriteByte(' ')
			} else {
				out.WriteString(cell.text)
			}
		}
	}
	out.WriteString(CSI + "0m")
	return out.String()
}
