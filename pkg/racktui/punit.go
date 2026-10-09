package racktui

import (
	"image/color"
	"sort"
	"strconv"
	"strings"

	"github.com/0magnet/chaosrack/pkg/panelart"
	"github.com/0magnet/chaosrack/pkg/rackspec"
	"github.com/0magnet/chaosrack/pkg/racksurface"
)

// The P-unit, in cells.
//
// The page builds most of its positions from one part (manual, "The P-unit"):
// a legend display down the left edge, a step readout with its trim and a
// reset, a setting display with a reset, an endless encoder in an LED ring,
// and + 0 − down the right edge. This draws that part, piece for piece, in
// the same places:
//
//	 ◉▐  0.25▌ ↺
//	l▐  36.0▌ ↺
//	o ⢀⠔⠒⠢⡀  +
//	n ⡇⣿⣿⣿⢸  0
//	  ⠈⠢⠤⠔⠁  −
//
// Matching the PART rather than the picture is what lets the two surfaces
// resemble each other. A dial rasterized into half blocks looks like the
// page's knob only from twelve columns up, which made a slot sixteen columns
// wide and the rack too big to see; nearly everything in a P-unit is a
// display, a lamp or a button, and those are what a terminal draws natively.

// Look is how a control is drawn.
type Look int

const (
	// LookDial is a rasterized knob over a reading: the original panel.
	LookDial Look = iota
	// LookPUnit is the page's P-unit.
	LookPUnit
	// LookPage is the panel as the page draws it, at a fixed scale
	// (pagelook.go). It needs a Source that is Pictured.
	LookPage
)

// ParseLook names a look for a flag.
func ParseLook(s string) (Look, bool) {
	switch strings.ToLower(s) {
	case "page", "":
		return LookPage, true
	case "dial":
		return LookDial, true
	case "punit", "p-unit", "p":
		return LookPUnit, true
	}
	return LookPage, false
}

var look = LookPage

// SetLook chooses how controls are drawn. It changes the size of a control,
// so it changes the size of the rack: set it before the panel lays out.
func SetLook(l Look) { look = l }

// The P-unit's columns, left to right.
const (
	puLegend  = 0 // the legend display, down the edge
	puDisplay = 2 // where both displays start
	puDispW   = 7 // their width
	puRingW   = 6 // the encoder
	puButtons = 10
	puCols    = 11
	// puSlotCols is a slot at this look: one P-unit and its share of a
	// module's border, so a 1-slot module holds exactly one, as on the page,
	// and a 12-slot bank holds twelve across.
	puSlotCols = 13
)

// puRows is how tall a P-unit is: two displays over the encoder, whose height
// is whatever keeps it round in this terminal's cell.
func puRows() int { return 2 + panelart.RingRows(puRingW) }

// block is one control's footprint, gap included, and the padding inside a
// module's border.
func block() (cols, rows, pad int) {
	if look == LookPUnit {
		return puCols + 1, puRows() + 1, 0
	}
	return knobCols + 1, ctlBlockRows(), panelPad
}

// metrics is the surface's scale at the current look. A bay is never shorter
// than one row of controls.
func metrics() racksurface.Metrics {
	m := racksurface.DefaultMetrics
	if look == LookPUnit {
		_, rows, _ := block()
		m.SlotCols = puSlotCols
		m.PanelRows = 1 + rows
	}
	return m
}

// The P-unit's colors, from the page's: a plate a shade off the frame, red
// LEDs on dark red glass, grey hardware.
var (
	puPlate  = color.RGBA{24, 27, 34, 255}
	puGlass  = color.RGBA{38, 8, 8, 255}
	puLED    = panelart.Dark.LEDOn
	puGrey   = color.RGBA{140, 144, 152, 255}
	puButton = color.RGBA{44, 48, 56, 255}
)

func puPalette() panelart.Palette {
	p := panelart.Dark
	p.Panel = puPlate
	p.Shadow = color.RGBA{96, 98, 106, 255}
	return p
}

// drawPUnit paints one control as a P-unit with its top-left at x, y.
func drawPUnit(p Painter, v racksurface.View, x, y int, c Control, sel bool, mark rune) {
	rows := puRows()
	set := func(cx, cy int, ch rune, fg, bg color.RGBA) {
		sx, sy := x+cx-v.X, y+cy-v.Y
		if sx < 0 || sy < 0 || sx >= v.W || sy >= v.H {
			return
		}
		p.Set(sx, sy, rgbCell(ch, fg, bg))
	}
	text := func(cx, cy int, s string, fg, bg color.RGBA) {
		for i, r := range []rune(s) {
			set(cx+i, cy, r, fg, bg)
		}
	}
	// The plate.
	for cy := range rows {
		for cx := range puCols {
			set(cx, cy, ' ', puGrey, puPlate)
		}
	}

	// The legend, one letter a row down the edge, as the page stacks it.
	fg, bg := puLED, puPlate
	if sel {
		fg, bg = puPlate, puLED
	}
	leg := []rune(strings.ToLower(shortLabel(c)))
	for i := 1; i < rows; i++ {
		ch := ' '
		if i-1 < len(leg) {
			ch = leg[i-1]
		}
		set(puLegend, i, ch, fg, bg)
	}

	// The step readout under its trim, and the setting display. The
	// selected unit's setting display is lit, so the cursor is where the
	// eye already goes.
	set(1, 0, mark, puGrey, puPlate)
	text(puDisplay, 0, display(stepOf(c), puDispW), puLED, puGlass)
	dfg, dbg := puLED, puGlass
	if sel {
		dfg, dbg = puGlass, puLED
	}
	text(puDisplay, 1, display(readingOfPU(c), puDispW), dfg, dbg)
	set(puButtons, 0, '↺', puGrey, puPlate)
	set(puButtons, 1, '↺', puGrey, puPlate)

	// The encoder.
	rr := rows - 2
	det := 0
	switch {
	case c.IsSwitch:
		det = 2
	case c.IsSelect:
		det = len(c.Options)
	}
	frac := fracOf(c)
	if c.IsSwitch && switchOn(c) {
		frac = 1
	}
	g := panelart.Ring(puRingW, rr, frac, det, puPalette())
	for ry := range rr {
		for rx := range puRingW {
			cl := g[ry*puRingW+rx]
			set(puDisplay+rx, 2+ry, cl.Ch, cl.Fg, cl.Bg)
		}
	}

	// + 0 −, from the encoder's top down.
	for i, ch := range []rune{'+', '0', '−'} {
		if 2+i < rows {
			set(puButtons, 2+i, ch, puGrey, puButton)
		}
	}
}

// display fits s to an LED display: numbers to the right, as the page sets
// them, and words to the left.
func display(s string, w int) string {
	s = clipStr(s, w)
	n := len([]rune(s))
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return strings.Repeat(" ", w-n) + s
	}
	return s + strings.Repeat(" ", w-n)
}

// stepOf is what the step readout says: the control's step, or nothing for a
// part that moves in detents.
func stepOf(c Control) string {
	if c.IsSwitch || c.IsSelect || c.Step == 0 {
		return ""
	}
	return shortNum(trimFloat(c.Step), puDispW)
}

// readingOfPU is the setting display's text, fitted to it.
func readingOfPU(c Control) string {
	if c.IsSwitch || c.IsSelect {
		return readingOf(c)
	}
	return shortNum(c.Value, puDispW)
}

// rgbCell is a cell in two colors of its own rather than a named style.
func rgbCell(ch rune, fg, bg color.RGBA) cellAt {
	return cellAt{
		Ch:     ch,
		Art:    true,
		Top:    [3]uint8{fg.R, fg.G, fg.B},
		Bottom: [3]uint8{bg.R, bg.G, bg.B},
	}
}

// ── placing by address ───────────────────────────────────────────────────

// addr is a control's address on the page, bay.column.row[.letter].
type addr struct {
	col, row int
	letter   string
}

// parseLoc reads an address. The bay is not kept: the module the control is
// mounted in already says which bay it is in, and where in the bay the module
// starts, so only the column and the row are news.
func parseLoc(s string) (addr, bool) {
	f := strings.Split(s, ".")
	if len(f) < 3 {
		return addr{}, false
	}
	col, err1 := strconv.Atoi(f[1])
	row, err2 := strconv.Atoi(f[2])
	if err1 != nil || err2 != nil {
		return addr{}, false
	}
	a := addr{col: col, row: row}
	if len(f) > 3 {
		a.letter = f[3]
	}
	return a, true
}

// rackRows is how many rows of controls a bay holds, the page's own number.
var rackRows = rackspec.RowsPerPanel()

// placedRows is how tall a module laid out by address is: every bay is
// rackRows of P-units, as every bay on the page is.
func placedRows() int {
	_, bh, _ := block()
	return 1 + rackRows*bh
}

// hasLocs reports whether the page has placed any of a module's controls.
func hasLocs(ctls []Control) bool {
	for _, c := range ctls {
		if _, ok := parseLoc(c.Loc); ok {
			return true
		}
	}
	return false
}

// puCell is one position of a module: where its block is, and the controls
// that share it. Several controls in one cell are one programmable P-unit,
// which shows one of them at a time, as the page's does.
type puCell struct {
	x, y    int
	members []int // indices into the module's controls
}

// placeModule lays a module's controls out by their addresses. at[i] is the
// cell control i is in, or -1 for one there was no room for.
//
// A control the page has not placed takes the first free cell, down each
// column: it is still a control, and dropping it would hide it.
func placeModule(pan racksurface.Panel, ctls []Control) (cells []puCell, at []int) {
	_, bh, _ := block()
	slots := max(pan.W/puSlotCols, 1)
	at = make([]int, len(ctls))
	byPos := map[[2]int]int{}
	cellAt := func(off, row int) int {
		k := [2]int{off, row}
		if i, ok := byPos[k]; ok {
			return i
		}
		byPos[k] = len(cells)
		cells = append(cells, puCell{x: pan.X + 1 + off*puSlotCols, y: pan.Y + 1 + (row-1)*bh})
		return byPos[k]
	}
	var loose []int
	for i, c := range ctls {
		a, ok := parseLoc(c.Loc)
		off := a.col - 1 - pan.Slot
		if !ok || off < 0 || off >= slots || a.row < 1 || a.row > rackRows {
			loose = append(loose, i)
			continue
		}
		ci := cellAt(off, a.row)
		cells[ci].members = append(cells[ci].members, i)
		at[i] = ci
	}
	for _, i := range loose {
		at[i] = -1
	free:
		for off := range slots {
			for row := 1; row <= rackRows; row++ {
				if _, taken := byPos[[2]int{off, row}]; !taken {
					ci := cellAt(off, row)
					cells[ci].members = []int{i}
					at[i] = ci
					break free
				}
			}
		}
	}
	return cells, at
}

// drawPlaced paints a module's cells.
func drawPlaced(p Painter, v racksurface.View, pan racksurface.Panel, mc moduleCtls, cur ctlAt) {
	cells, _ := placeModule(pan, mc.Ctls)
	for _, cl := range cells {
		show, sel := cl.members[0], false
		for _, m := range cl.members {
			if cur.Module == pan.Item && cur.Index == m {
				show, sel = m, true
			}
		}
		allSwitches := len(cl.members) > 1
		for _, m := range cl.members {
			allSwitches = allSwitches && mc.Ctls[m].IsSwitch
		}
		if allSwitches {
			drawLamps(p, v, cl.x, cl.y, mc.Ctls, cl.members, show, sel)
			continue
		}
		mark := '◉'
		if len(cl.members) > 1 {
			// Which of the cell's controls is showing, as the page's
			// setting display says which parameter a P-unit is on.
			for k, m := range cl.members {
				if m == show {
					mark = rune('a' + k)
				}
			}
		}
		if c := mc.Ctls[show]; !c.PUnit && !c.IsSwitch && len(cl.members) == 1 {
			drawDialCell(p, v, cl.x, cl.y, c, sel)
			continue
		}
		drawPUnit(p, v, cl.x, cl.y, mc.Ctls[show], sel, mark)
	}
}

// drawLamps is a cell of switches: the page's column of lit buttons, one
// lamp and legend a row.
func drawLamps(p Painter, v racksurface.View, x, y int, ctls []Control, members []int, show int, sel bool) {
	rows := puRows()
	set := func(cx, cy int, ch rune, fg, bg color.RGBA) {
		sx, sy := x+cx-v.X, y+cy-v.Y
		if sx < 0 || sy < 0 || sx >= v.W || sy >= v.H {
			return
		}
		p.Set(sx, sy, rgbCell(ch, fg, bg))
	}
	for cy := range rows {
		for cx := range puCols {
			set(cx, cy, ' ', puGrey, puPlate)
		}
	}
	// Scrolled so the selected one is always in sight.
	first := 0
	for k, m := range members {
		if m == show && k >= rows {
			first = k - rows + 1
		}
	}
	for r := range rows {
		k := first + r
		if k >= len(members) {
			break
		}
		c := ctls[members[k]]
		lamp := scaleRGB(puLED, 0.3)
		if switchOn(c) {
			lamp = puLED
		}
		set(1, r, '●', lamp, puPlate)
		fg, bg := puGrey, puPlate
		if sel && members[k] == show {
			fg, bg = puPlate, puLED
		}
		for i, ch := range []rune(clipStr(strings.ToLower(shortLabel(c)), puCols-3)) {
			set(3+i, r, ch, fg, bg)
		}
	}
}

func scaleRGB(c color.RGBA, k float64) color.RGBA {
	return color.RGBA{uint8(float64(c.R) * k), uint8(float64(c.G) * k), uint8(float64(c.B) * k), 255}
}

// placedRect is where control i of a module is drawn at this look.
func placedRect(pan racksurface.Panel, ctls []Control, i int) (x, y, w, h int, ok bool) {
	cells, at := placeModule(pan, ctls)
	if i < 0 || i >= len(at) || at[i] < 0 {
		return 0, 0, 0, 0, false
	}
	return cells[at[i]].x, cells[at[i]].y, puCols, puRows(), true
}

// byAddress orders a module's controls the way the page letters them: down
// each column, then across, and the controls the page has not placed last.
func byAddress(ctls []Control) {
	key := func(c Control) (int, int, string, bool) {
		a, ok := parseLoc(c.Loc)
		return a.col, a.row, a.letter, ok
	}
	sort.SliceStable(ctls, func(i, j int) bool {
		ci, ri, li, oki := key(ctls[i])
		cj, rj, lj, okj := key(ctls[j])
		if oki != okj {
			return oki
		}
		if ci != cj {
			return ci < cj
		}
		if ri != rj {
			return ri < rj
		}
		return li < lj
	})
}

// drawDialCell is a control the page builds as a plain dial rather than a
// P-unit, in a P-unit's cell: its legend and reading over the knob, the
// knob as big as the cell lets it be.
func drawDialCell(p Painter, v racksurface.View, x, y int, c Control, sel bool) {
	rows := puRows()
	set := func(cx, cy int, ch rune, fg, bg color.RGBA) {
		sx, sy := x+cx-v.X, y+cy-v.Y
		if sx < 0 || sy < 0 || sx >= v.W || sy >= v.H {
			return
		}
		p.Set(sx, sy, rgbCell(ch, fg, bg))
	}
	for cy := range rows {
		for cx := range puCols {
			set(cx, cy, ' ', puGrey, puPlate)
		}
	}
	lfg, lbg := puGrey, puPlate
	dfg, dbg := puLED, puGlass
	if sel {
		lfg, lbg = puPlate, puLED
		dfg, dbg = puGlass, puLED
	}
	for i, ch := range []rune(clipStr(shortLabel(c), 3)) {
		set(i, 0, ch, lfg, lbg)
	}
	for i, ch := range []rune(display(readingOfPU(c), puDispW)) {
		set(puCols-puDispW+i, 0, ch, dfg, dbg)
	}
	// The widest knob that fits under the readout.
	kw, kr := puCols, 0
	var cells []panelart.Cell
	for ; kw >= 4; kw-- {
		cells, kr = panelart.KnobCells(kw, fracOf(c), detentsOf(c), panelart.Dark)
		if kr <= rows-1 {
			break
		}
	}
	if kr > rows-1 {
		return
	}
	x0 := (puCols - kw) / 2
	for ry := range kr {
		for rx := range kw {
			cl := cells[ry*kw+rx]
			set(x0+rx, 1+ry, panelart.HalfBlock, cl.Top, cl.Bottom)
		}
	}
}
