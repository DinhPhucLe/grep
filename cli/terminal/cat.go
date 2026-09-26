package terminal

// Original terminal pixel art adapted from the supplied motion blueprint.
// Every frame is 16x10 pixels; movement never changes the layout bounds.
type catPixels [10][postureWidth]byte

func (p *catPixels) dot(x, y int, ch byte) {
	if x >= 0 && x < postureWidth && y >= 0 && y < len(p) {
		p[y][x] = ch
	}
}
func (p *catPixels) rect(x, y, w, h int, fill byte) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			p.dot(xx, yy, fill)
		}
	}
}
func (p *catPixels) box(x, y, w, h int) {
	p.rect(x, y, w, h, '#')
	if w > 2 && h > 2 {
		p.rect(x+1, y+1, w-2, h-2, 'o')
	}
}

func catInk(state postureState, ch byte) rgb {
	switch ch {
	case 'o':
		return rgb{177, 196, 175}
	case 'w':
		return rgb{238, 242, 231}
	case 'd':
		return rgb{31, 43, 42}
	case '!':
		return rgb{230, 212, 136}
	}
	switch state {
	case postureRelaxed:
		return rgb{102, 215, 129}
	case postureUpright:
		return rgb{237, 207, 80}
	case postureTense:
		return rgb{246, 82, 75}
	default:
		return rgb{126, 146, 147}
	}
}

func catFrame(view postureView) catPixels {
	var p catPixels
	ticks := 16
	switch view.state {
	case postureUpright:
		ticks = 12
	case postureTense:
		ticks = 8
	}
	action := int(view.motion) / ticks % 5
	step := int(view.motion) % ticks
	half := step >= ticks/2
	dx, dy := 0, 0
	// Tremor/shake are one-pixel accents within the fixed frame, with pauses.
	if action == 2 && view.state != postureRelaxed && view.state != postureUnknown && step%4 == 1 {
		dx = 1
	}
	if action == 4 && view.state == postureTense && half {
		dx = 1
		dy = 1
	}
	hx, hy := 4+dx, dy
	if action == 1 && view.state == postureRelaxed && half {
		hx--
	}
	// Tail first, behind the body. Low: slow wag. Medium: sharp flick.
	// High: a larger swish and a short horizontal lunge in the last action.
	tailX := 3 + dx
	if action == 3 && half {
		tailX = 2 + dx
	}
	if view.state == postureTense && action == 3 && half {
		tailX = 1
	}
	p.rect(tailX, 6, 1, 3, '#')
	p.rect(tailX, 8, 6+dx-tailX, 1, '#')
	if action == 3 && half {
		p.dot(tailX-1, 5, '#')
	}
	p.box(6+dx, 5, 5, 4)
	p.dot(6+dx, 9, '#')
	p.dot(10+dx, 9, '#')
	if view.state == postureTense && action == 4 && half {
		p.rect(4, 7, 8, 2, '#')
		p.dot(4, 9, '#')
		p.dot(12, 9, '#')
	}
	// Broad square head and two distinct cat ears.
	p.box(hx, hy+1, 9, 5)
	p.rect(hx, hy, 2, 2, '#')
	p.rect(hx+7, hy, 2, 2, '#')
	if action == 1 && half && view.state == postureUpright {
		p.dot(hx+7, hy, 0)
		p.dot(hx+8, hy, 0)
		p.dot(hx+8, hy+1, '#')
	}
	if action == 1 && view.state == postureTense {
		p.rect(hx, hy, 9, 1, 0)
		p.dot(hx-1, hy+2, '#')
		p.dot(hx+9, hy+2, '#')
	}
	lx, rx, ey := hx+2, hx+6, hy+3
	if view.state == postureUpright {
		p.rect(lx-1, ey-1, 2, 2, 'w')
		p.rect(rx-1, ey-1, 2, 2, 'w')
		if action == 4 && half {
			lx--
			rx--
		}
	}
	p.dot(lx, ey, 'd')
	p.dot(rx, ey, 'd')
	p.dot(hx+4, hy+4, 'd')
	if view.state == postureRelaxed && action == 1 && half {
		p.dot(lx, ey, 'o')
		p.dot(lx, ey+1, 'd')
	}
	if view.state == postureTense {
		p.dot(lx-1, ey-1, 'd')
		p.dot(rx+1, ey-1, 'd')
		p.dot(hx+3, hy+5, 'd')
		p.dot(hx+5, hy+5, 'd')
	} else if view.blink || view.state == postureRelaxed && action == 0 && half {
		p.dot(lx+1, ey, 'd')
		p.dot(rx-1, ey, 'd')
	}
	// Slow paw wave, drawn beside the torso rather than moving the whole cat.
	if view.state == postureRelaxed && action == 2 {
		pawY := 7
		if half {
			pawY = 6
		}
		p.rect(11, pawY, 3, 1, '#')
		p.dot(13, pawY-1, 'o')
	}
	if view.state == postureUnknown {
		// A still neutral cat when no actual metric is known.
		p.dot(hx+4, hy+4, 'o')
	}
	if view.state == postureTense && action == 2 && step%4 == 1 {
		p.dot(1, 3, '!')
		p.dot(15, 6, '!')
	}
	return p
}
