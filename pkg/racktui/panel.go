package racktui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"

	"github.com/0magnet/chaosrack/pkg/panelart"
	"github.com/0magnet/chaosrack/pkg/racksurface"
)

// The panel: the rack on a surface of its own size, and a window onto it.
//
// The rack is drawn at the size the RACK is. A terminal too small to hold it
// shows part of it and pans, exactly as the page does when the rack is put in
// a window too small for it — which is also why the two now agree about what a
// bay is. The older arrangement wrapped modules to the terminal's width, and
// that is a second layout: it had no bays in it, and a module's neighbors were
// whatever the wrap happened to put beside it.

type panel struct {
	src  Source
	ctls []Control
	all  []Control // before the filter
	cur  int
	top  int    // first row drawn in the LIST view, for scrolling
	filt string // substring filter
	list bool   // show the table instead of the rack
	msg  string // the last thing that happened
	err  string

	// The rack, laid out once, and the window onto it.
	//
	// The surface is built from ALL the controls rather than the filtered
	// ones, so typing a filter does not make the rack rearrange itself under
	// the cursor. A fixed surface that moves when you search is not fixed.
	items []racksurface.Item
	mods  []moduleCtls
	surf  racksurface.Surface
	view  racksurface.View
}

var (
	stNormal = tcell.StyleDefault
	stDim    = tcell.StyleDefault.Foreground(color.Gray)
	stHead   = tcell.StyleDefault.Bold(true)
	stCursor = tcell.StyleDefault.Reverse(true)
	stValue  = tcell.StyleDefault.Foreground(color.Green)
	stErr    = tcell.StyleDefault.Foreground(color.Red)
)

// RunOn draws the panel on a screen the caller already has.
func RunOn(sc tcell.Screen, src Source) error {
	if err := sc.Init(); err != nil {
		return err
	}
	defer sc.Fini()
	sc.SetStyle(stNormal)

	// The dials are sampled into cells, so their shape depends on the shape
	// of a cell. See CellShaper.
	if sh, ok := src.(CellShaper); ok {
		panelart.SetCellAspect(sh.CellAspect())
	}

	p := &panel{src: src}
	p.reload()
	// v3 hands events over a channel and reports key RELEASES as well as
	// presses — without the Pressed check every keystroke would move the
	// cursor twice, which reads as a panel that has lost its detents.
	for {
		p.draw(sc)
		switch ev := (<-sc.EventQ()).(type) {
		case *tcell.EventResize:
			sc.Sync()
		case *tcell.EventKey:
			if !ev.Pressed() {
				continue
			}
			if p.key(ev) {
				return nil
			}
		}
	}
}

// reload asks the rack what it is now, and lays it out again.
//
// The layout is redone on every reload rather than once at the start, because
// a module can be switched out while the panel is open and the rack reflows
// when it is. The VIEW survives: the window stays where it was looking, which
// is what makes a knob change not throw the reader somewhere else.
func (p *panel) reload() {
	mods, capacity, err := p.src.Modules()
	if err != nil {
		p.err = err.Error()
		return
	}
	c, err := p.src.Controls()
	if err != nil {
		p.err = err.Error()
		return
	}
	p.all, p.err = c, ""
	p.layout(mods, capacity)
	p.refilter()
}

// layout builds the surface: the module widths come from the rack, and the
// module HEIGHTS from how tall this renderer's controls are.
//
// The asymmetry is the design. A module's width is the rack's business — a
// whole number of slots in a frame a fixed number of slots across — but how
// many rows its controls need depends on how they are drawn, which is a
// different answer in a browser and in a terminal. So the renderer says how
// tall, and the packer makes each bay as tall as the tallest module in it.
func (p *panel) layout(mods []racksurface.Item, capacity int) {
	if capacity < 1 {
		capacity = 12
	}
	perModule := map[string]int{}
	for _, c := range p.all {
		perModule[fold(c.Module)]++
	}
	p.items = append(p.items[:0], mods...)
	for i := range p.items {
		p.items[i].Rows = ModuleRows(perModule[fold(p.items[i].Key)],
			p.items[i].Slots, racksurface.DefaultMetrics.SlotCols)
	}
	p.surf = racksurface.Build(p.items, capacity, nil, racksurface.DefaultMetrics)
	p.mods = attach(p.items, p.all)
}

func (p *panel) refilter() {
	if p.filt == "" {
		p.ctls = p.all
	} else {
		p.ctls = nil
		f := strings.ToLower(p.filt)
		for _, c := range p.all {
			if strings.Contains(strings.ToLower(c.ID), f) || strings.Contains(strings.ToLower(c.Label), f) {
				p.ctls = append(p.ctls, c)
			}
		}
	}
	if p.cur >= len(p.ctls) {
		p.cur = len(p.ctls) - 1
	}
	if p.cur < 0 {
		p.cur = 0
	}
}

// key handles one keystroke and reports whether to quit.
func (p *panel) key(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyEscape, tcell.KeyCtrlC:
		return true
	case tcell.KeyUp:
		// Control pans the window instead of moving the cursor. The arrows
		// are already the two things a panel does — choose a control, turn it
		// — so panning takes the modifier rather than a second set of keys to
		// remember.
		if ctrlHeld(ev) {
			p.pan(0, -1)
			return false
		}
		p.move(-1)
	case tcell.KeyDown:
		if ctrlHeld(ev) {
			p.pan(0, 1)
			return false
		}
		p.move(1)
	case tcell.KeyPgUp:
		p.move(-10)
	case tcell.KeyPgDn:
		p.move(10)
	case tcell.KeyHome:
		p.cur = 0
	case tcell.KeyEnd:
		p.cur = len(p.ctls) - 1
	case tcell.KeyLeft:
		if ctrlHeld(ev) {
			p.pan(-1, 0)
			return false
		}
		p.nudge(-1)
	case tcell.KeyRight:
		if ctrlHeld(ev) {
			p.pan(1, 0)
			return false
		}
		p.nudge(1)
	case tcell.KeyTab:
		p.list = !p.list
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if p.filt != "" {
			p.filt = p.filt[:len(p.filt)-1]
			p.refilter()
		}
	case tcell.KeyRune:
		r := firstRune(ev.Str())
		switch r {
		case 'q':
			if p.filt == "" {
				return true
			}
			p.typeFilter(r)
		case '\t':
			p.list = !p.list
		case '/':
			p.filt = ""
			p.refilter()
		case 'r':
			if p.filt == "" {
				p.reload()
				p.msg = "reloaded"
				return false
			}
			p.typeFilter(r)
		case '0':
			if p.filt == "" {
				p.reset()
				return false
			}
			p.typeFilter(r)
		default:
			p.typeFilter(r)
		}
	}
	return false
}

func (p *panel) typeFilter(r rune) {
	p.filt += string(r)
	p.refilter()
}

func (p *panel) move(n int) {
	if len(p.ctls) == 0 {
		return
	}
	p.cur += n
	if p.cur < 0 {
		p.cur = 0
	}
	if p.cur >= len(p.ctls) {
		p.cur = len(p.ctls) - 1
	}
}

// reset puts the control under the cursor back to its default, which the
// registry knows without asking the rack.
func (p *panel) reset() {
	c, ok := p.at()
	if !ok {
		return
	}
	v := c.SelectDef
	if !c.IsSelect {
		v = trimFloat(c.Def)
	}
	p.apply(c, v)
}

// nudge moves the control under the cursor one detent or one step.
func (p *panel) nudge(dir int) {
	c, ok := p.at()
	if !ok {
		return
	}
	// A switch is thrown, not turned. Either arrow sets the position it points
	// at — left off, right on — rather than "the other one", because a toggle
	// whose result depends on where it already was cannot be driven blind.
	//
	// Caught BEFORE the numeric path, which is where it used to land: a switch
	// carries no range, so that path read "1", added a step of 1 and wrote
	// "2" — which the rack reads as off. Turning a switch up turned it off.
	if c.IsSwitch {
		if dir > 0 {
			p.apply(c, "1")
		} else {
			p.apply(c, "0")
		}
		return
	}
	if c.IsSelect {
		p.apply(c, nextOption(c, dir))
		return
	}
	step := c.Step
	if step == 0 {
		step = 1
	}
	v, err := strconv.ParseFloat(c.Value, 64)
	if err != nil {
		v = c.Def
	}
	v += float64(dir) * step
	if c.Max != c.Min {
		if v > c.Max {
			v = c.Max
		}
		if v < c.Min {
			v = c.Min
		}
	}
	p.apply(c, trimFloat(v))
}

// nextOption is the detent dir away from the one showing, stopping at the
// ends: a rotary switch does not wrap and neither does this.
func nextOption(c Control, dir int) string {
	if len(c.Options) == 0 {
		return c.Value
	}
	at := 0
	for i, o := range c.Options {
		if o == c.Value {
			at = i
			break
		}
	}
	at += dir
	if at < 0 {
		at = 0
	}
	if at >= len(c.Options) {
		at = len(c.Options) - 1
	}
	return c.Options[at]
}

func (p *panel) at() (Control, bool) {
	if p.cur < 0 || p.cur >= len(p.ctls) {
		return Control{}, false
	}
	return p.ctls[p.cur], true
}

// apply sends the change and then asks the rack what actually happened: a
// control can clamp, quantize to a detent, or refuse, and showing what was
// asked for rather than what was taken is how a panel comes to lie.
func (p *panel) apply(c Control, v string) {
	if err := p.src.Set(c.ID, v); err != nil {
		p.err = err.Error()
		return
	}
	p.err = ""
	p.msg = c.ID + " = " + v
	p.reload()
}

func trimFloat(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// ── drawing ──────────────────────────────────────────────────────────────

func (p *panel) draw(sc tcell.Screen) {
	sc.Clear()
	w, h := sc.Size()

	if !p.list {
		p.drawRack(sc, w, h)
	} else {
		p.drawList(sc, 0, w, h)
	}

	// The status line: what just happened, or how to work it.
	status := "↑↓ move   ←→ turn   0 reset   ctrl+↑↓←→ pan   tab rack/list   r reload   q quit"
	st := stDim
	switch {
	case p.err != "":
		status, st = "error: "+p.err, stErr
	case p.filt != "":
		status = "filter: " + p.filt + "   (" + strconv.Itoa(len(p.ctls)) + " of " + strconv.Itoa(len(p.all)) + ")   backspace to edit, / to clear"
	case p.msg != "":
		status = p.msg
	}
	puts(sc, 0, h-1, clip(status, w), st)
	sc.Show()
}

func rangeOf(c Control) string {
	if c.IsSelect {
		if len(c.Options) > 0 {
			return strings.Join(c.Options, " ")
		}
		return "select"
	}
	return fmt.Sprintf("%g .. %g / %g", c.Min, c.Max, c.Step)
}

func puts(sc tcell.Screen, x, y int, s string, st tcell.Style) {
	for _, r := range s {
		sc.SetContent(x, y, r, nil, st)
		x++
	}
}

func clip(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w <= 0 {
		return ""
	}
	return string(r[:w])
}

// firstRune is the key a KeyRune event carries. v3 reports a string because a
// keystroke can be a composed sequence; a panel of single-key commands wants
// the first of it and nothing else.
func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}

// drawRack paints the window onto the surface, and the bars that say where in
// the rack the window is.
//
// The view is moved only to keep the selected control visible, and only by as
// much as that takes. A view that recentered on every step would make the
// whole rack lurch for a keypress that moved one control.
func (p *panel) drawRack(sc tcell.Screen, w, h int) {
	// One row for the status line, one for the horizontal bar, one column for
	// the vertical one.
	vw, vh := maxi(w-1, 1), maxi(h-2, 1)
	p.view.W, p.view.H = vw, vh
	p.view = p.view.Clamp(p.surf)

	cur := p.cursor()
	if x, y, cw, ch, ok := ctlRect(p.surf, p.mods, cur); ok {
		p.view = p.view.Reveal(x, y, cw, ch, p.surf)
	}
	DrawSurface(screenPainter{sc: sc, w: vw, h: vh}, p.surf, p.view, p.mods, cur)

	// Where in the rack this is. The bars are the whole reason a fixed
	// surface is navigable: without them a window onto a rack six times the
	// terminal's height is a panel with no edges.
	if pos, n := racksurface.Bar(p.view.Y, vh, p.surf.Rows, vh); n > 0 {
		for i := 0; i < n; i++ {
			sc.SetContent(w-1, pos+i, '█', nil, stFrame)
		}
	}
	if pos, n := racksurface.Bar(p.view.X, vw, p.surf.Cols, vw); n > 0 {
		for i := 0; i < n; i++ {
			sc.SetContent(pos+i, h-2, '▀', nil, stFrame)
		}
	}
	blank, share := p.surf.Blank()
	puts(sc, 0, h-2, clip(fmt.Sprintf("%d bays  %dx%d  at %d,%d  %d blank (%.0f%%)",
		len(p.surf.Bays), p.surf.Cols, p.surf.Rows, p.view.X, p.view.Y,
		blank, share*100), maxi(w/2, 1)), stDim)
}

// cursor is where the selected control sits on the surface.
//
// It is looked up BY ID rather than kept as a position, because the surface is
// rebuilt on every reload and a position would name a different control after
// a module was switched out.
func (p *panel) cursor() ctlAt {
	c, ok := p.at()
	if !ok {
		return ctlAt{Module: -1}
	}
	if a, found := cursorOf(p.mods, c.ID); found {
		return a
	}
	return ctlAt{Module: -1}
}

// pan moves the window by a fraction of itself, which is what a scroll does:
// a whole screen is disorienting and one cell is useless.
func (p *panel) pan(dx, dy int) {
	p.view = p.view.Pan(dx*maxi(p.view.W/2, 1), dy*maxi(p.view.H/2, 1), p.surf)
	p.msg = ""
}

// drawList draws the controls as a table, which is the view for finding one
// by name among seventy rather than for turning it.
func (p *panel) drawList(sc tcell.Screen, y, w, h int) {
	puts(sc, 0, y, fmt.Sprintf("%-24s %-14s %-12s %s", "CONTROL", "LABEL", "VALUE", "RANGE"), stHead)
	y++
	rows := h - y - 1
	if rows < 1 {
		rows = 1
	}
	if p.cur < p.top {
		p.top = p.cur
	}
	if p.cur >= p.top+rows {
		p.top = p.cur - rows + 1
	}
	for i := p.top; i < len(p.ctls) && i < p.top+rows; i++ {
		c := p.ctls[i]
		st := stNormal
		if i == p.cur {
			st = stCursor
		}
		puts(sc, 0, y, fmt.Sprintf("%-24s %-14s", clip(c.ID, 24), clip(c.Label, 14)), st)
		vs := stValue
		if i == p.cur {
			vs = st
		}
		puts(sc, 40, y, fmt.Sprintf("%-12s", clip(c.Value, 12)), vs)
		puts(sc, 53, y, clip(rangeOf(c), max(0, w-54)), stDim)
		y++
	}
}

// ctrlHeld reports the control modifier, which is what separates panning the
// window from moving the cursor.
func ctrlHeld(ev *tcell.EventKey) bool { return ev.Modifiers()&tcell.ModCtrl != 0 }
