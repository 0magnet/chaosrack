package racktui

import "strings"

// hoverAt takes the pointer at cell x, y with nothing held: the control under
// it becomes the one outlined and named, as a page names what it is over. It
// reports whether that changed, so a pointer moving across one control, or
// across the scene, draws nothing.
func (p *panel) hoverAt(x, y, w, h int) bool {
	id := p.controlAt(x, y, w, h)
	if id == p.hover {
		return false
	}
	p.hover = id
	return true
}

// controlAt is the control drawn at cell x, y of the terminal, or "".
func (p *panel) controlAt(x, y, w, h int) string {
	if p.list {
		return ""
	}
	if p.paged {
		if p.winMode == winHidden {
			return ""
		}
		lx, ly := x-p.winX, y-p.winY
		if lx < 0 || ly < 0 || lx >= p.view.W || ly >= p.view.H {
			return ""
		}
		px, py := pageOfCell(p.view.X+lx, p.view.Y+ly)
		id, _ := ctlAtPage(p.picture(), px, py)
		return id
	}
	vw, vh := maxi(w-1, 1), maxi(h-2, 1)
	if x >= vw || y >= vh {
		return ""
	}
	if hits := p.hitsAt(x+p.view.X, y+p.view.Y); len(hits) > 0 {
		return p.pick(hits)
	}
	return ""
}

// hoverLine is the status line for the control under the pointer: what it is
// called, where it stands, and how far it goes.
func (p *panel) hoverLine() string {
	for _, c := range p.all {
		if c.ID != p.hover {
			continue
		}
		parts := []string{c.Label}
		if c.Label == "" {
			parts[0] = c.ID
		}
		if c.Value != "" {
			parts = append(parts, c.Value)
		}
		parts = append(parts, "("+rangeOf(c)+")", c.ID)
		return strings.Join(parts, "   ")
	}
	return p.hover
}
