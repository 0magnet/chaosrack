package rackhost

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"math"
	"strconv"

	"github.com/0magnet/sh/v3/interp"
	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/chaosrack/pkg/racktui"
	"github.com/0magnet/chaosrack/pkg/rasterview"
	"github.com/0magnet/tuiwasm/demos"
)

// The Desk model, natively: windows that are terminals, as the page's desk
// is windows over panes. It opens what the page's does — a websh terminal,
// a shell on the host, and the terminal demos — and starts, as the page's
// does, with one terminal, in which `open NAME`, `term`, `host` and `apps`
// open and list the others.
//
// The page's five desktops are its style selector's here too. Flat tiles
// the windows, and on a terminal is drawn as their text; the four
// reproductions put pictures of the windows in space — Looking Glass's
// leaning stack, Compiz's cube, Metisse's turned windows and BumpTop's
// piles — to be turned by hand like any other model.
//
// The keys, with the window put away, are the focused window's, but these:
// Alt+1..9 focuses a window, Alt+T opens a terminal and Alt+W closes the
// focused window. A window also closes when its program ends.

// deskWin is a window: a shell or a demo, and what its title says.
type deskWin struct {
	app, title string
	shell      *shellView
	anim       *animView
}

// deskView is the desk's windows, the focused one, and what its terminals
// have asked it to open.
type deskView struct {
	wins  []*deskWin
	focus int
	// opens is what websh's open, term and host ask to have opened. They run
	// on a shell's goroutine; the desk opens windows on the panel's.
	opens chan []string
	begun bool
}

func newDesk() deskView { return deskView{opens: make(chan []string, 16)} }

// deskApp is something the desk opens.
type deskApp struct{ name, help string }

// deskApps is what the desk opens: its two shells, then every demo.
func deskApps() []deskApp {
	apps := []deskApp{
		{"term", "a websh terminal"},
		{"host", "a shell on this machine"},
	}
	for _, d := range demos.All() {
		apps = append(apps, deskApp{d.Name, d.Desc})
	}
	return apps
}

// deskOpens reports whether the desk opens app name.
func deskOpens(name string) bool {
	if name == "term" || name == "host" {
		return true
	}
	_, ok := demos.Lookup(name)
	return ok
}

// exec is the desk's commands in its terminals (console.Options.Exec): the
// page desk's applets of the same names.
func (d *deskView) exec(ctx context.Context, args []string) (int, bool) {
	hc := interp.HandlerCtx(ctx)
	list := func() {
		for _, a := range deskApps() {
			say(hc.Stdout, "%-14s %s\n", a.name, a.help)
		}
	}
	switch args[0] {
	case "apps":
		list()
		return 0, true
	case "open":
		if len(args) < 2 || args[1] == "-l" {
			if len(args) < 2 {
				say(hc.Stderr, "usage: open NAME (open -l lists them)\n")
				return 2, true
			}
			list()
			return 0, true
		}
		if !deskOpens(args[1]) {
			say(hc.Stderr, "open: no app called %s (apps lists them)\n", args[1])
			return 1, true
		}
		d.ask(args[1])
		return 0, true
	case "term", "host":
		d.ask(args[0])
		return 0, true
	}
	return 0, false
}

// say writes to a command's output; a terminal gone has no one to tell.
func say(w io.Writer, format string, a ...any) {
	if _, err := fmt.Fprintf(w, format, a...); err != nil {
		return
	}
}

// ask asks for app to be opened, from a shell's goroutine.
func (d *deskView) ask(app string) {
	select {
	case d.opens <- []string{app}:
	default: // a desk asked for sixteen windows at once has enough to open
	}
}

// open opens a window on app. Called with r.mu held.
func (d *deskView) open(app string) {
	w := &deskWin{app: app}
	switch app {
	case "term":
		w.title = "terminal"
		w.shell = &shellView{run: webshShell(d.exec)}
		w.shell.start()
	case "host":
		w.title = "host shell"
		w.shell = &shellView{run: hostShell}
		w.shell.start()
	default:
		w.title = app
		w.anim = &animView{}
		w.anim.start(app)
	}
	d.wins = append(d.wins, w)
	d.focus = len(d.wins) - 1
}

// sync opens what has been asked for, starts the desk with a terminal, and
// closes the windows whose programs have ended. Called with r.mu held.
func (d *deskView) sync() {
	for more := true; more; {
		select {
		case a := <-d.opens:
			d.open(a[0])
		default:
			more = false
		}
	}
	if !d.begun {
		d.begun = true
		if len(d.wins) == 0 {
			d.open("term")
		}
	}
	kept := d.wins[:0]
	for _, w := range d.wins {
		switch {
		case w.shell != nil && !w.shell.running():
		case w.anim != nil && w.anim.ended():
			w.anim.close()
		default:
			kept = append(kept, w)
		}
	}
	clear(d.wins[len(kept):])
	d.wins = kept
	d.focus = min(d.focus, max(len(d.wins)-1, 0))
}

// closeFocused closes the focused window, hanging up on its program.
func (d *deskView) closeFocused() {
	if len(d.wins) == 0 {
		return
	}
	w := d.wins[d.focus]
	if w.shell != nil {
		w.shell.kill()
	} else {
		w.anim.close()
	}
	d.wins = append(d.wins[:d.focus], d.wins[d.focus+1:]...)
	d.focus = min(d.focus, max(len(d.wins)-1, 0))
}

// typeKey is a key at the desk: its own, or the focused window's. Called with
// r.mu held.
func (d *deskView) typeKey(ev *tcell.EventKey) {
	if ev.Key() == tcell.KeyRune && ev.Modifiers()&tcell.ModAlt != 0 {
		switch s := ev.Str(); {
		case len(s) == 1 && s[0] >= '1' && s[0] <= '9':
			if n := int(s[0] - '1'); n < len(d.wins) {
				d.focus = n
			}
			return
		case s == "t" || s == "T":
			d.open("term")
			return
		case s == "w" || s == "W":
			d.closeFocused()
			return
		}
	}
	if len(d.wins) == 0 {
		return
	}
	if w := d.wins[d.focus]; w.shell != nil {
		w.shell.typeKey(ev)
	} else {
		w.anim.typeKey(ev)
	}
}

// resize makes the window's terminal cols by rows.
func (w *deskWin) resize(cols, rows int) {
	switch {
	case w.shell != nil:
		w.shell.resize(cols, rows)
	case w.anim.sim != nil:
		w.anim.sim.SetSize(cols, rows)
	case w.anim.term != nil:
		w.anim.term.resize(cols, rows)
	}
}

// cells is the window's terminal as cells.
func (w *deskWin) cells() ([]racktui.TextCell, int, int) {
	switch {
	case w.shell != nil:
		return w.shell.term.textCells()
	case w.anim.sim != nil:
		return simCells(w.anim.sim)
	case w.anim.term != nil:
		return w.anim.term.textCells()
	}
	return nil, 0, 0
}

// picture is the window's terminal as a picture.
func (w *deskWin) picture() *image.RGBA {
	switch {
	case w.shell != nil:
		return w.shell.term.picture()
	case w.anim.sim != nil:
		return simPicture(w.anim.sim)
	case w.anim.term != nil:
		return w.anim.term.picture()
	}
	return cellPicture(1, 1)
}

// The desk's colors: the page's window blue for the focused frame.
var (
	deskGround = color.RGBA{16, 24, 40, 255}
	deskFrame  = color.RGBA{96, 104, 120, 255}
	deskFocus  = color.RGBA{0, 102, 255, 255}
	deskBar    = color.RGBA{32, 36, 44, 255}
	deskInk    = color.RGBA{220, 224, 232, 255}
)

// deskBox is a window's place on the flat desk, in cells.
type deskBox struct{ x, y, w, h int }

// tile lays n windows over w by h cells: as near square a grid as holds
// them, the last row's windows widened to fill it.
func tile(n, w, h int) []deskBox {
	if n == 0 {
		return nil
	}
	cols := int(math.Ceil(math.Sqrt(float64(n))))
	rows := (n + cols - 1) / cols
	boxes := make([]deskBox, 0, n)
	for r := range rows {
		inRow := cols
		if r == rows-1 {
			inRow = n - cols*(rows-1)
		}
		y0, y1 := r*h/rows, (r+1)*h/rows
		for c := range inRow {
			x0, x1 := c*w/inRow, (c+1)*w/inRow
			boxes = append(boxes, deskBox{x0, y0, x1 - x0, y1 - y0})
		}
	}
	return boxes
}

// text is the flat desk on w by h cells: its windows tiled, each framed and
// titled and its terminal sized to its frame, over the panel's row. Called
// with r.mu held.
func (d *deskView) text(w, h int) []racktui.TextCell {
	grid := make([]racktui.TextCell, w*h)
	for i := range grid {
		grid[i] = racktui.TextCell{Ch: ' ', Bg: deskGround}
	}
	set := func(x, y int, c racktui.TextCell) {
		if x >= 0 && y >= 0 && x < w && y < h {
			grid[y*w+x] = c
		}
	}
	for i, b := range tile(len(d.wins), w, h-1) {
		win := d.wins[i]
		win.resize(max(b.w-2, 1), max(b.h-2, 1))
		frame := deskFrame
		if i == d.focus {
			frame = deskFocus
		}
		drawFrame(set, b, frame, " "+strconv.Itoa(i+1)+" "+win.title+" ")
		cells, cols, rows := win.cells()
		for y := range min(rows, b.h-2) {
			for x := range min(cols, b.w-2) {
				set(b.x+1+x, b.y+1+y, cells[y*cols+x])
			}
		}
	}
	bar := d.panelText()
	for x := range w {
		c := racktui.TextCell{Ch: ' ', Fg: deskInk, Bg: deskBar}
		if x < len(bar) {
			c.Ch = bar[x]
		}
		set(x, h-1, c)
	}
	return grid
}

// drawFrame frames box b in color fg, title on its top edge.
func drawFrame(set func(x, y int, c racktui.TextCell), b deskBox, fg color.RGBA, title string) {
	cell := func(ch rune) racktui.TextCell { return racktui.TextCell{Ch: ch, Fg: fg, Bg: deskGround} }
	x1, y1 := b.x+b.w-1, b.y+b.h-1
	for x := b.x + 1; x < x1; x++ {
		set(x, b.y, cell('─'))
		set(x, y1, cell('─'))
	}
	for y := b.y + 1; y < y1; y++ {
		set(b.x, y, cell('│'))
		set(x1, y, cell('│'))
	}
	set(b.x, b.y, cell('┌'))
	set(x1, b.y, cell('┐'))
	set(b.x, y1, cell('└'))
	set(x1, y1, cell('┘'))
	for i, ch := range []rune(title) {
		if x := b.x + 2 + i; x < x1 {
			set(x, b.y, racktui.TextCell{Ch: ch, Fg: deskInk, Bg: fg})
		}
	}
}

// panelText is the desk's panel: its windows, and its keys.
func (d *deskView) panelText() []rune {
	s := " desk "
	for i, w := range d.wins {
		mark := " "
		if i == d.focus {
			mark = "*"
		}
		s += "│ " + strconv.Itoa(i+1) + mark + w.title + " "
	}
	s += "│ Alt+1-9 focus · Alt+T terminal · Alt+W close · open NAME, apps"
	return []rune(s)
}

// framed is window i's picture in its frame, title and all, as the page's
// compositor draws a window's chrome into its canvas.
func (d *deskView) framed(i int) *image.RGBA {
	win := d.wins[i]
	content := win.picture()
	cols := content.Rect.Dx()/cellW + 2
	rows := content.Rect.Dy()/cellH + 2
	m := cellPicture(cols, rows)
	frame := deskFrame
	if i == d.focus {
		frame = deskFocus
	}
	drawFrame(func(x, y int, c racktui.TextCell) { paintCell(m, x, y, c.Ch, c.Fg, c.Bg) },
		deskBox{0, 0, cols, rows}, frame, " "+strconv.Itoa(i+1)+" "+win.title+" ")
	draw.Draw(m, content.Rect.Add(image.Pt(cellW, cellH)), content, image.Point{}, draw.Src)
	return m
}

// quads is the desk's windows in space for style, each a picture on a quad
// two units wide. Called with r.mu held.
func (d *deskView) quads(style string) []*rasterview.Quad {
	n := len(d.wins)
	out := make([]*rasterview.Quad, 0, n)
	for i := range n {
		pic := d.framed(i)
		hw := 1.0
		hh := hw * float64(pic.Rect.Dy()) / float64(pic.Rect.Dx())
		c, u, up := deskPlace(style, i, n, hw, hh)
		out = append(out, &rasterview.Quad{
			Pic: pic,
			BL:  vadd(c, vscale(u, -hw), vscale(up, -hh)),
			BR:  vadd(c, vscale(u, hw), vscale(up, -hh)),
			TL:  vadd(c, vscale(u, -hw), vscale(up, hh)),
		})
	}
	return out
}

// deskPlace is where window i of n stands in style: its middle, and the
// directions of its width and its height, for a window hw by hh either side
// of its middle.
func deskPlace(style string, i, n int, hw, hh float64) (c, u, up [3]float64) {
	u, up = [3]float64{1, 0, 0}, [3]float64{0, 1, 0}
	grid := func() [3]float64 {
		cols := int(math.Ceil(math.Sqrt(float64(n))))
		rows := (n + cols - 1) / cols
		col, row := i%cols, i/cols
		return [3]float64{
			(float64(col) - float64(cols-1)/2) * (2*hw + 0.2),
			(float64(rows-1)/2 - float64(row)) * (2*hh + 0.2),
			0,
		}
	}
	switch style {
	case "glass":
		// The newest stands nearly upright and those behind lean away about
		// their left edges, each a step further back: Looking Glass's stack.
		depth := float64(n - i)
		a := math.Min(18+7*depth, 62) * math.Pi / 180
		u = [3]float64{math.Cos(a), 0, -math.Sin(a)}
		// Centered on the middle of the stack, which is what frames it.
		step := depth - 1 - float64(n-1)/2
		hinge := [3]float64{-hw + 0.2*step, 0, -0.3 * step}
		c = vadd(hinge, vscale(u, hw))
	case "cube":
		// A workspace a face, round-robin by the order opened, as an
		// unconfigured Compiz did.
		yaw := float64(i%4) * math.Pi / 2
		r := hw + 0.04*float64(i/4)
		c = [3]float64{r * math.Sin(yaw), 0, r * math.Cos(yaw)}
		u = [3]float64{math.Cos(yaw), 0, -math.Sin(yaw)}
	case "metisse":
		// Tiled, and each turned on its own axes, as Metisse's windows are
		// turned by hand.
		c = grid()
		ry := 24.0 * math.Pi / 180
		if i%2 == 1 {
			ry = -ry
		}
		rx := 10.0 * math.Pi / 180
		u = [3]float64{math.Cos(ry), 0, -math.Sin(ry)}
		up = [3]float64{math.Sin(rx) * math.Sin(ry), math.Cos(rx), math.Sin(rx) * math.Cos(ry)}
	case "bump":
		// Shrunk, and fallen into piles three wide, each lying a little
		// askew: BumpTop's.
		const s = 0.45
		col, level := i%3, i/3
		c = [3]float64{(float64(col) - 1) * 2.2 * hw * s, -1.2 + hh*s + float64(level)*(2*hh*s+0.04), 0}
		tilt := float64((i*7)%5-2) * 4 * math.Pi / 180
		u = vscale([3]float64{math.Cos(tilt), math.Sin(tilt), 0}, s)
		up = vscale([3]float64{-math.Sin(tilt), math.Cos(tilt), 0}, s)
	default:
		c = grid()
	}
	return c, u, up
}

func vadd(vs ...[3]float64) [3]float64 {
	var s [3]float64
	for _, v := range vs {
		s[0], s[1], s[2] = s[0]+v[0], s[1]+v[1], s[2]+v[2]
	}
	return s
}

func vscale(v [3]float64, k float64) [3]float64 { return [3]float64{v[0] * k, v[1] * k, v[2] * k} }
