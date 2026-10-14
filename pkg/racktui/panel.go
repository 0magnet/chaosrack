package racktui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"

	"github.com/0magnet/chaosrack/pkg/panelart"
	"github.com/0magnet/chaosrack/pkg/rackpic"
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
	// typing is set by /: the next letter starts a filter even where it is
	// also a command (p, q, r, w, 0), so a filter can begin with one.
	typing bool
	list   bool   // show the table instead of the rack
	msg    string // the last thing that happened
	err    string

	// The rack, laid out once, and the window onto it.
	//
	// The surface is built from ALL the controls rather than the filtered
	// ones, so typing a filter does not make the rack rearrange itself under
	// the cursor. A fixed surface that moves when you search is not fixed.
	items []racksurface.Item
	mods  []moduleCtls
	surf  racksurface.Surface
	view  racksurface.View
	// shown is the control the view was last moved to show. The view follows
	// the cursor only when the cursor MOVES, so panning away from it sticks.
	shown string
	// held is the mouse buttons down at the last event, so a press is told
	// from a drag or a release.
	held tcell.ButtonMask
	// dragging is the scroll bar the button went down on: 'v', 'h' or 0.
	dragging rune

	// pic is the panel as the page draws it, for the page look; nil for the
	// drawn looks, or when the Source cannot describe it. Read it through
	// picture(): while the page's own parts are laid over the cells nothing
	// draws it, and it is taken only when something does.
	pic *rackpic.Picture
	// paged is the page look, from a Source that can describe its panel.
	paged bool
	// The page look's pixels: the scene behind the window, and the panel's
	// canvases by item index; and the window, which winMode places.
	scene      *rackpic.Image
	canv       map[int]*rackpic.Image
	winMode    int
	winX, winY int
	// floatX, floatY is where the window floats over a Native overlay, its
	// title row's left end (layoutFloat).
	floatX, floatY int
	// dock is the edge the window is docked against, or "" floating; winR is
	// the window as laid out, docked or floating, its title row included.
	dock       string
	winR       Rect
	scrW, scrH int
	// turning is a drag on the scene, which keeps the mouse until let go;
	// sceneQ sends what is done to the scene, and q posts back to the loop
	// (nil when the panel runs on no screen).
	turning bool
	sceneQ  sceneQueue
	q       *poster
	// hover is the control under the pointer, outlined and named on the
	// status line as the page's tooltips name it; "" over none.
	hover string
	// winGrab is a drag on the window's frame on a host: 'm' moving it by
	// its title, 'r' sizing it by its corner, 0 none. grabDX, grabDY is
	// where on the title the drag took hold; hostW, hostH is the window's
	// size on a host (0 until it is first laid out).
	winGrab        rune
	grabDX, grabDY int
	hostW, hostH   int
	// overlay lays the page's parts over the cells, where the source and
	// the terminal can (overlay.go); laidOut is what was last written for it.
	overlay bool
	laidOut string
	// sceneNote is why the scene is not drawn as it should be, while it is not.
	sceneNote string
	// shared is a panel that found its parts another's: it draws in cells
	// and leaves the overlay, which is that other panel's, alone.
	shared bool
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
	// Posting stops before Fini closes the queue (live.go).
	q := &poster{sc: sc, every: paceOf(src)}
	done := make(chan struct{})
	defer sc.Fini()
	defer close(done)
	defer q.close()
	sc.SetStyle(stNormal)
	// Motion too, with no button down: what the pointer is over is shown,
	// as the page shows it.
	sc.EnableMouse(tcell.MouseButtonEvents | tcell.MouseDragEvents | tcell.MouseMotionEvents)

	// The dials are sampled into cells, so their shape depends on the shape
	// of a cell. See CellShaper.
	if sh, ok := src.(CellShaper); ok {
		panelart.SetCellAspect(sh.CellAspect())
	}

	// Where the page first floats its controls: near the top left.
	p := &panel{src: src, q: q, overlay: !plain, floatX: 2, floatY: 2}
	if p.overlay && partsTaken(src) {
		p.overlay, p.shared, p.msg = false, true, partsTakenMsg
	}
	defer p.clearOverlay(sc) // before Fini, which is deferred first
	if nv, ok := src.(Native); ok && !p.shared {
		// What is done to the window in the page comes back as events, so
		// it is done on this loop and nowhere else.
		// Off the caller, which is the page's event handler, and tried again
		// while the loop is behind: a move dropped is a window left behind.
		nv.OnWindow(func(a WindowAct) { go q.postSoon(tcell.NewEventInterrupt(a)) })
		defer nv.OnWindow(nil)
		p.dock = dockEdge(nv.Dock())
	}
	p.reload()
	p.loadPicture()
	go q.tick(done)
	// v3 hands events over a channel and reports key RELEASES as well as
	// presses — without the Pressed check every keystroke would move the
	// cursor twice, which reads as a panel that has lost its detents.
	//
	// Everything already waiting is taken before the screen is drawn once
	// for all of it: a mouse moved over a big screen sends a move for every
	// cell it crosses, and a draw for each, each costing what the whole
	// screen costs, fell further behind with every one.
	//
	// A refresh is done when the draw after it is: what it cost, the asking
	// and the drawing together, sets how long the ticker rests (live.go).
	dirty := true
	var refreshing, drawn time.Time
	for {
		// Drawn at least every tenth of a second all the same, so a stream of
		// events that never lets the queue empty still shows.
		if dirty && (len(sc.EventQ()) == 0 || time.Since(drawn) > 100*time.Millisecond) {
			p.draw(sc)
			dirty, drawn = false, time.Now()
		}
		if !refreshing.IsZero() && !dirty {
			q.cost(time.Since(refreshing))
			q.waiting.Store(false)
			refreshing = time.Time{}
		}
		changed := true
		switch ev := (<-sc.EventQ()).(type) {
		case *tcell.EventResize:
			// Nothing more to do than draw: tcell has already repainted every
			// cell for the new size. Syncing here painted them all again, and
			// zoomed out each of those took seconds.
		case *tcell.EventKey:
			if !ev.Pressed() {
				continue
			}
			if p.key(ev) {
				return nil
			}
		case *tcell.EventMouse:
			w, h := sc.Size()
			changed = p.mouse(ev, w, h)
		case *tcell.EventInterrupt:
			switch d := ev.Data().(type) {
			case refreshTick:
				// A refresh that found nothing new draws nothing.
				refreshing = time.Now()
				changed = p.refresh()
			case sceneErr:
				p.err = string(d)
			case WindowAct:
				p.windowAct(d)
			}
		}
		dirty = dirty || changed
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
	perModule := map[string][]Control{}
	for _, c := range p.all {
		perModule[fold(c.Module)] = append(perModule[fold(c.Module)], c)
	}
	p.items = append(p.items[:0], mods...)
	for i := range p.items {
		ctls := perModule[fold(p.items[i].Key)]
		p.items[i].Rows = ModuleRows(len(ctls), p.items[i].Slots, metrics().SlotCols)
		if look == LookPUnit && hasLocs(ctls) {
			p.items[i].Rows = placedRows()
		}
	}
	p.surf = racksurface.Build(p.items, capacity, nil, metrics())
	p.mods = attach(p.items, p.all)
	if look == LookPUnit {
		// The cursor walks the rack as the page letters it, module by module
		// and down each column, rather than in the order things were wired.
		p.all = p.all[:0:0]
		for i := range p.mods {
			byAddress(p.mods[i].Ctls)
			p.all = append(p.all, p.mods[i].Ctls...)
		}
	}
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
		if p.typing && ev.Key() == tcell.KeyEscape {
			// Escape takes back a / that has not been typed after yet.
			p.typing = false
			return false
		}
		if p.filt != "" && ev.Key() == tcell.KeyEscape {
			// And a filter, leaving the cursor on the control it found: it
			// quit the whole rack, which is not what Escape out of a search
			// means.
			p.clearFilter()
			return false
		}
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
		// A letter with Ctrl, Alt or Meta held is somebody else's key — the
		// browser's Ctrl+Shift+R, a window manager's — and not the letter.
		// Taken as the letter, Ctrl+Shift+R re-read the whole rack (r), and
		// held down it queued seconds of that.
		if ev.Modifiers()&(tcell.ModCtrl|tcell.ModAlt|tcell.ModMeta) != 0 {
			return false
		}
		r := firstRune(ev.Str())
		switch r {
		case 'q':
			if p.commands() {
				return true
			}
			p.typeFilter(r)
		case '\t':
			p.list = !p.list
		case '/':
			p.filt = ""
			p.typing = true
			p.refilter()
		case 'p':
			if p.commands() && p.paged {
				// The window: half, the whole terminal, or out of the way.
				p.winMode = (p.winMode + 1) % 3
				p.scene, p.canv = nil, nil
				return false
			}
			p.typeFilter(r)
		case 'w':
			if p.shared && p.commands() {
				p.msg = partsTakenMsg
				return false
			}
			if _, ok := p.src.(Overlay); ok && p.commands() {
				// The page's parts over the cells, or the cells alone: the
				// two drawings of one panel, compared in place.
				p.overlay = !p.overlay
				p.msg = "web parts off: cells only"
				if p.overlay {
					p.msg = "web parts on"
				}
				return false
			}
			p.typeFilter(r)
		case 'r':
			if p.commands() {
				p.reload()
				p.loadPicture()
				p.msg = "reloaded"
				return false
			}
			p.typeFilter(r)
		case '0':
			if p.commands() {
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

// commands reports whether a letter is a command rather than the start of a
// filter: with no filter, and none begun with /.
func (p *panel) commands() bool { return p.filt == "" && !p.typing }

// clearFilter shows every control again, the cursor still on the one it was.
func (p *panel) clearFilter() {
	id := ""
	if p.cur >= 0 && p.cur < len(p.ctls) {
		id = p.ctls[p.cur].ID
	}
	p.filt = ""
	p.refilter()
	for i, c := range p.ctls {
		if c.ID == id {
			p.cur = i
		}
	}
}

func (p *panel) typeFilter(r rune) {
	p.typing = false
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
	p.fresh()
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
	p.fresh()
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
	resynced(sc)
	sc.Clear()
	w, h := sc.Size()

	if !p.list {
		p.drawRack(sc, w, h)
	} else {
		p.drawList(sc, 0, w, h)
	}

	// The status line: what just happened, or how to work it.
	status := "↑↓ move   ←→ turn   / find   0 reset   ctrl+↑↓←→ pan   tab rack/list   r reload   q quit"
	if p.paged {
		status += "   p window"
	}
	if _, ok := p.src.(Overlay); ok {
		status += "   w web"
	}
	st := stDim
	switch {
	case p.err != "":
		status, st = "error: "+p.err, stErr
	case p.typing:
		status = "filter: _   type to find a control"
	case p.filt != "":
		status = "filter: " + p.filt + "   (" + strconv.Itoa(len(p.ctls)) + " of " + strconv.Itoa(len(p.all)) + ")   backspace to edit, esc or / to clear"
	case p.hover != "":
		status = p.hoverLine()
	case p.sceneNote != "":
		status = "the scene: " + p.sceneNote
	case p.msg != "":
		status = p.msg
	}
	puts(sc, 0, h-1, clip(status, w), st)
	sc.Show()
	p.syncOverlay(sc)
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
	if p.paged {
		p.drawPage(sc, w, h)
		return
	}
	p.view = p.view.Clamp(p.surf)

	cur := p.cursor()
	if c, ok := p.at(); ok && c.ID != p.shown {
		if x, y, cw, ch, ok := ctlRect(p.surf, p.mods, cur); ok {
			p.view = p.view.Reveal(x, y, cw, ch, p.surf)
		}
		p.shown = c.ID
	}
	DrawSurface(screenPainter{sc: sc, w: vw, h: vh}, p.surf, p.view, p.mods, cur)

	// Where in the rack this is. The bars are the whole reason a fixed
	// surface is navigable: without them a window onto a rack six times the
	// terminal's height is a panel with no edges.
	if pos, n := racksurface.Bar(p.view.Y, vh, p.surf.Rows, vh); n > 0 {
		for i := range n {
			sc.SetContent(w-1, pos+i, '█', nil, stFrame)
		}
	}
	if pos, n := racksurface.Bar(p.view.X, vw, p.surf.Cols, vw); n > 0 {
		for i := range n {
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
	p.view = p.view.Pan(dx*maxi(p.view.W/2, 1), dy*maxi(p.view.H/2, 1), p.surface())
	p.msg = ""
}

// drawList draws the controls as a table, which is the view for finding one
// by name among seventy rather than for turning it.
func (p *panel) drawList(sc tcell.Screen, y, w, h int) {
	puts(sc, 0, y, fmt.Sprintf("%-24s %-14s %-12s %s", "CONTROL", "LABEL", "VALUE", "RANGE"), stHead)
	y++
	rows := max(h-y-1, 1)
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

// resynced brings tcell's idea of the screen's size back to the terminal's.
//
// tcell takes each resize on a goroutine of its own, so a burst of them (a
// font size changed step after step) can finish out of order and leave the
// screen the size of one of the middle steps: the panel then drew its status
// line part way down a terminal whose other rows kept whatever was last on
// them. A Sync re-reads the size, which is what the Sync on every resize used
// to hide; done only when the two disagree, an ordinary resize stays one
// repaint, which tcell has already made.
func resynced(sc tcell.Screen) {
	t, ok := sc.Tty()
	if !ok {
		return
	}
	ws, err := t.WindowSize()
	if err != nil {
		return
	}
	if w, h := sc.Size(); ws.Width != w || ws.Height != h {
		sc.Sync()
	}
}
