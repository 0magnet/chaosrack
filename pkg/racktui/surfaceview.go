package racktui

import (
	"strings"

	"github.com/0magnet/chaosrack/pkg/panelart"
	"github.com/0magnet/chaosrack/pkg/racksurface"
)

// Drawing the rack onto a surface, and a window onto the surface.
//
// The rack is drawn ONCE at its own size and the terminal shows part of it.
// Nothing here consults the terminal's width: a module two slots wide is two
// slots wide, bay three begins where bay three begins, and making the window
// bigger — or the cell smaller — reveals more of the same picture rather than
// rearranging it.
//
// That is the inversion. Before this, the panel wrapped modules to the
// terminal's width, which meant the rack had one shape in the browser and a
// different one in a shell, and a module's neighbors in the terminal were
// whatever the wrap happened to put beside it. A physical rack does not get
// wider when you step back from it.

// Layout constants for one module's interior, in cells.
const (
	ctlLabelRows = 1 // the silkscreen above a control
	ctlValueRows = 1 // the reading below it
	ctlGap       = 1 // between one control and the next
	panelPad     = 1 // inside the module's border
	// knobCols is how wide a dial is drawn. Eight columns is the floor at
	// which a half-block knob still reads as a knob rather than as a smudge,
	// and it stays coarse until about twelve. The slot width in
	// racksurface.DefaultMetrics is set FROM this, not the other way round: the
	// dial decides how wide a slot has to be, as it does on a real panel.
	knobCols = 12
)

// ctlBlockRows is how tall one control's block is, knob included.
func ctlBlockRows() int {
	_, kr := panelart.KnobCells(knobCols, 0, 0, panelart.Dark)
	return ctlLabelRows + kr + ctlValueRows + ctlGap
}

// ModuleRows is how tall a module with n controls needs to be, given how wide
// it is in slots. It is what a front end tells the packer so that a bay comes
// out as tall as the tallest module in it.
//
// Controls are laid out in columns inside the module, as many across as the
// module is wide, so a 2-slot module with six controls is three rows of two
// rather than six rows of one — which is both what the panel does and the only
// way a wide module does not end up a tall thin strip.
func ModuleRows(ctls, slots, slotCols int) int {
	if ctls < 1 {
		ctls = 1
	}
	across := ctlsAcross(slots, slotCols)
	down := (ctls + across - 1) / across
	return panelPad*2 + 1 + down*ctlBlockRows() // +1 for the module's name
}

// ctlsAcross is how many controls fit side by side in a module.
func ctlsAcross(slots, slotCols int) int {
	if slots < 1 {
		slots = 1
	}
	inner := slots*slotCols - panelPad*2 - 2 // less the border
	n := max(inner/(knobCols+1), 1)
	return n
}

// cellAt is one painted cell of the surface: either a rune in a style, or a
// half block carrying two colors.
type cellAt struct {
	Ch     rune
	Art    bool // this cell came from panelart; Top/Bottom carry its colors
	Top    [3]uint8
	Bottom [3]uint8
	Style  int // an index into the renderer's styles, for a text cell
}

// Style indices. Small and explicit because a tcell style is not comparable
// cheaply and the buffer is written per cell.
const (
	styPanel = iota
	styLabel
	styValue
	styFrame
	styBayLabel
	styCursor
	styDim
)

// Painter is what the renderer writes to: the screen, or a test's buffer.
type Painter interface {
	Set(x, y int, c cellAt)
}

// DrawSurface paints the part of the surface the view can see.
//
// Only the panels the view actually touches are drawn, so a rack far larger
// than the terminal costs what is on screen and not what exists.
func DrawSurface(p Painter, s racksurface.Surface, v racksurface.View, mods []moduleCtls, cur ctlAt) {
	for bi := range s.Bays {
		b := s.Bays[bi]
		if !v.Sees(0, b.Y, s.Cols, b.H) {
			continue
		}
		drawBayHead(p, v, b)
		for _, pi := range b.Panels {
			pan := s.Panels[pi]
			if pan.W == 0 || !v.Sees(pan.X, pan.Y, pan.W, pan.H) {
				continue
			}
			var mc moduleCtls
			if pan.Item < len(mods) {
				mc = mods[pan.Item]
			}
			drawModule(p, v, pan, mc, cur)
		}
	}
}

// drawBayHead writes the bay number and the section labels, each over exactly
// the modules it names — a bay carrying three sections needs three labels, and
// one label over the whole bay would be two-thirds wrong.
func drawBayHead(p Painter, v racksurface.View, b racksurface.Bay) {
	y := b.Y
	put(p, v, 0, y, "bay "+itoa(b.Index+1), styBayLabel)
	for i, r := range b.Runs {
		if i >= len(b.SpanX) {
			break
		}
		x, w := b.SpanX[i], b.SpanW[i]
		if w < 3 {
			continue
		}
		label := " " + strings.ToUpper(r.Section) + " "
		if len(label) > w {
			label = label[:w]
		}
		// A rule across the run with the name let into it, which is how a
		// group is marked on a panel that has no room for a box round it.
		line := label + strings.Repeat("─", w-len(label))
		put(p, v, x, y+1, line, styFrame)
	}
}

// drawModule paints one module: its border, its name, and its controls.
func drawModule(p Painter, v racksurface.View, pan racksurface.Panel, mc moduleCtls, cur ctlAt) {
	x0, y0, w, h := pan.X, pan.Y, pan.W, pan.H
	// The border. A module is a plate in a frame, and the frame is what makes
	// a bay read as a bay rather than as a row of floating widgets.
	put(p, v, x0, y0, "┌"+strings.Repeat("─", maxi(w-2, 0))+"┐", styFrame)
	for y := y0 + 1; y < y0+h-1; y++ {
		put(p, v, x0, y, "│", styFrame)
		put(p, v, x0+w-1, y, "│", styFrame)
	}
	put(p, v, x0, y0+h-1, "└"+strings.Repeat("─", maxi(w-2, 0))+"┘", styFrame)

	name := strings.ToUpper(mc.Name)
	if name != "" {
		put(p, v, x0+2, y0, " "+clipStr(name, maxi(w-6, 1))+" ", styBayLabel)
	}

	across := maxi((w-panelPad*2-2)/(knobCols+1), 1)
	block := ctlBlockRows()
	for i, c := range mc.Ctls {
		col, row := i%across, i/across
		cx := x0 + 1 + panelPad + col*(knobCols+1)
		cy := y0 + 1 + panelPad + row*block
		if cy+block > y0+h {
			// Out of panel. Say so rather than drawing over the border: a
			// module whose controls do not fit is a metric to fix, not a
			// thing to hide.
			put(p, v, x0+2, y0+h-2, clipStr("+"+itoa(len(mc.Ctls)-i)+" more", maxi(w-4, 1)), styDim)
			break
		}
		sel := cur.Module == pan.Item && cur.Index == i
		drawOneControl(p, v, cx, cy, c, sel)
	}
}

// drawOneControl is a label, a dial, and a reading.
func drawOneControl(p Painter, v racksurface.View, x, y int, c Control, sel bool) {
	st := styLabel
	if sel {
		st = styCursor
	}
	put(p, v, x, y, clipStr(shortLabel(c), knobCols), st)

	// The dial. Clipped here rather than left to the Painter: a knob sitting
	// half off the edge of the window is the ordinary case once the rack is
	// bigger than the terminal, and a renderer that writes past the edge
	// corrupts whatever the host had drawn there — in a shell, the scrollback.
	//
	// A switch gets a lamp instead: it has a state, not a position, so there
	// is nothing for a pointer to read and a dial would put a knob where the
	// instrument has a toggle.
	cells, rows := panelart.KnobCells(knobCols, fracOf(c), detentsOf(c), panelart.Dark)
	if c.IsSwitch {
		cells, rows = panelart.LampCells(knobCols, switchOn(c), panelart.Dark)
	}
	for ry := range rows {
		sy := y + ctlLabelRows + ry - v.Y
		if sy < 0 || sy >= v.H {
			continue
		}
		for rx := range knobCols {
			sx := x + rx - v.X
			if sx < 0 || sx >= v.W {
				continue
			}
			cl := cells[ry*knobCols+rx]
			p.Set(sx, sy, cellAt{
				Ch:     panelart.HalfBlock,
				Art:    true,
				Top:    [3]uint8{cl.Top.R, cl.Top.G, cl.Top.B},
				Bottom: [3]uint8{cl.Bottom.R, cl.Bottom.G, cl.Bottom.B},
			})
		}
	}
	vst := styValue
	if sel {
		vst = styCursor
	}
	put(p, v, x, y+ctlLabelRows+rows, clipStr(readingOf(c), knobCols), vst)
}

// put writes a string at a surface position, clipped to the view.
func put(p Painter, v racksurface.View, x, y int, s string, style int) {
	sy := y - v.Y
	if sy < 0 || sy >= v.H {
		return
	}
	for i, r := range []rune(s) {
		sx := x + i - v.X
		if sx < 0 || sx >= v.W {
			continue
		}
		p.Set(sx, sy, cellAt{Ch: r, Style: style})
	}
}

func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

func clipStr(s string, w int) string {
	r := []rune(s)
	if w < 0 {
		w = 0
	}
	if len(r) <= w {
		return s
	}
	return string(r[:w])
}
