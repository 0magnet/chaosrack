package racktui

import (
	"image/color"
	"math"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/chaosrack/pkg/panelart"
	"github.com/0magnet/chaosrack/pkg/rackpic"
	"github.com/0magnet/chaosrack/pkg/racksurface"
)

// The page look: the panel as the page draws it, at a fixed scale.
//
// The other looks decide how big a control is in cells and so how much of
// one fits; this one does not decide anything. The page says where every box,
// disc, word and pointer of its panel is (pkg/rackpic), and they are painted
// at one cell per pagePxPerCol x pagePxPerRow of the page's pixels: boxes and
// discs as half-block pixels, words as characters.
//
// The scale is FIXED, and that is the point of it. A control is as many cells
// as it needs to be drawn properly, whatever the terminal's font, so making
// the font smaller does not lose detail — it makes the whole panel smaller,
// as stepping back from a real one does. The numbers are a cell at websh's
// smallest zoom (2.4 x 5 pixels), so a websh zoomed all the way out shows the
// panel at the page's own size, and at a normal font it is several times
// bigger and panned.

// The page's pixels per cell.
const (
	pagePxPerCol = 2.4
	pagePxPerRow = 5.0
)

// Pictured is a Source that can describe the panel as a picture and act on
// it. The page look needs one; any other Source gets the drawn looks.
type Pictured interface {
	// Picture is the whole panel now.
	Picture() (*rackpic.Picture, error)
	// PictureChanges is what changed since the picture of generation gen.
	PictureChanges(gen int) (*rackpic.Patch, error)
	// Act does kind ("click" or "wheel") at x, y of the picture of
	// generation gen, as the pointer would on the page.
	Act(gen int, x, y float64, kind string, delta float64) error
	// Scene is the model's canvas as w x h pixels each shape times as
	// tall as wide.
	Scene(w, h int, shape float64) (*rackpic.Image, error)
	// Canvases is the panel's canvases asked for, by item index.
	Canvases(gen int, want []rackpic.CanvasWant) (map[int]*rackpic.Image, error)
}

// pageSurface is the size of the picture in cells.
func pageSurface(p *rackpic.Picture) racksurface.Surface {
	return racksurface.Surface{
		Cols: int(math.Ceil(p.W / pagePxPerCol)),
		Rows: int(math.Ceil(p.H / pagePxPerRow)),
	}
}

// cellOfPage is the cell over the page's point x, y.
func cellOfPage(x, y float64) (int, int) {
	return int(math.Floor(x / pagePxPerCol)), int(math.Floor(y / pagePxPerRow))
}

// pageOfCell is the page's point at the middle of cell x, y.
func pageOfCell(x, y int) (float64, float64) {
	return (float64(x) + 0.5) * pagePxPerCol, (float64(y) + 0.5) * pagePxPerRow
}

// ctlRectOfPage is a control's part in cells.
func ctlRectOfPage(p *rackpic.Picture, id string) (x, y, w, h int, ok bool) {
	r, ok := p.Ctls[id]
	if !ok {
		return 0, 0, 0, 0, false
	}
	x0, y0 := cellOfPage(r[0], r[1])
	x1, y1 := cellOfPage(r[0]+r[2], r[1]+r[3])
	return x0, y0, max(x1-x0, 1), max(y1-y0, 1), true
}

// ctlAtPage is the control whose part holds the page's point, the smallest
// when parts nest.
func ctlAtPage(p *rackpic.Picture, x, y float64) (string, bool) {
	best, area := "", math.Inf(1)
	for id, r := range p.Ctls {
		if x >= r[0] && x < r[0]+r[2] && y >= r[1] && y < r[1]+r[3] && r[2]*r[3] < area {
			best, area = id, r[2]*r[3]
		}
	}
	return best, best != ""
}

// pageGround is what shows where the panel has nothing: the page's.
var pageGround = color.RGBA{0, 0, 0, 255}

// pointerInk is a pointer drawn without a color of its own.
var pointerInk = color.RGBA{230, 233, 236, 255}

// cursorInk outlines the control the keys work on. The page has no such
// thing, so it is a color the panel does not use.
var cursorInk = color.RGBA{255, 200, 40, 255}

// canvas is the window's worth of half-block pixels and the characters over
// them.
type canvas struct {
	v      racksurface.View
	px     []color.RGBA // v.W x 2*v.H
	ch     []rune       // v.W x v.H; 0 is none
	ink    []color.RGBA
	pw, ph int
}

func newCanvas(v racksurface.View) *canvas {
	c := &canvas{v: v, pw: v.W, ph: 2 * v.H}
	c.px = make([]color.RGBA, c.pw*c.ph)
	for i := range c.px {
		c.px[i] = pageGround
	}
	c.ch = make([]rune, v.W*v.H)
	c.ink = make([]color.RGBA, v.W*v.H)
	return c
}

// A pixel is half a cell tall: column cx, half-row py, both of the window.
func (c *canvas) set(cx, py int, col color.RGBA) {
	if cx < 0 || py < 0 || cx >= c.pw || py >= c.ph {
		return
	}
	c.px[py*c.pw+cx] = col
}

// pixel range an item covers, by pixel centers, in window coordinates; an
// item too small to hold a center gets the pixel its middle is in, so a
// three-pixel lamp is not lost between two.
func (c *canvas) span(x, y, w, h float64) (x0, x1, y0, y1 int) {
	const pxW, pxH = pagePxPerCol, pagePxPerRow / 2
	x0 = int(math.Ceil(x/pxW - 0.5))
	x1 = int(math.Floor((x+w)/pxW - 0.5))
	y0 = int(math.Ceil(y/pxH - 0.5))
	y1 = int(math.Floor((y+h)/pxH - 0.5))
	if x1 < x0 {
		x0 = int(math.Floor((x + w/2) / pxW))
		x1 = x0
	}
	if y1 < y0 {
		y0 = int(math.Floor((y + h/2) / pxH))
		y1 = y0
	}
	return x0 - c.v.X, x1 - c.v.X, y0 - 2*c.v.Y, y1 - 2*c.v.Y
}

func (c *canvas) fill(it *rackpic.Item, col color.RGBA) {
	const pxW, pxH = pagePxPerCol, pagePxPerRow / 2
	x0, x1, y0, y1 := c.span(it.X, it.Y, it.W, it.H)
	cx, cy := it.X+it.W/2, it.Y+it.H/2
	rx, ry := it.W/2, it.H/2
	for py := max(y0, 0); py <= min(y1, c.ph-1); py++ {
		for px := max(x0, 0); px <= min(x1, c.pw-1); px++ {
			if it.Round != 0 && rx > 0 && ry > 0 {
				dx := ((float64(px+c.v.X)+0.5)*pxW - cx) / rx
				dy := ((float64(py+2*c.v.Y)+0.5)*pxH - cy) / ry
				if dx*dx+dy*dy > 1 {
					continue
				}
			}
			c.px[py*c.pw+px] = col
		}
	}
	// A box over a word hides it, where it covers the word's whole cell.
	if it.Round == 0 {
		for row := max((y0+1)/2, 0); row <= min((y1-1)/2, c.v.H-1); row++ {
			for col := max(x0, 0); col <= min(x1, c.pw-1); col++ {
				c.ch[row*c.v.W+col] = 0
			}
		}
	}
}

func (c *canvas) outline(x, y, w, h float64, col color.RGBA) {
	x0, x1, y0, y1 := c.span(x, y, w, h)
	for px := x0; px <= x1; px++ {
		c.set(px, y0, col)
		c.set(px, y1, col)
	}
	for py := y0; py <= y1; py++ {
		c.set(x0, py, col)
		c.set(x1, py, col)
	}
}

// pointer draws a knob's pointer: a line from near the middle out toward the
// rim, at the item's angle.
func (c *canvas) pointer(it *rackpic.Item, col color.RGBA) {
	const pxW, pxH = pagePxPerCol, pagePxPerRow / 2
	a := *it.Angle * math.Pi / 180
	cx, cy := it.X+it.W/2, it.Y+it.H/2
	r := math.Min(it.W, it.H) / 2
	for t := r * 0.15; t <= r*0.85; t += 0.5 {
		x, y := cx+t*math.Sin(a), cy-t*math.Cos(a)
		c.set(int(math.Floor(x/pxW))-c.v.X, int(math.Floor(y/pxH))-2*c.v.Y, col)
	}
}

// text puts an item's words in characters, at the row through its middle.
func (c *canvas) text(it *rackpic.Item, col color.RGBA) {
	rs := []rune(it.Text)
	if it.Vertical != 0 {
		x, y0 := cellOfPage(it.X+it.W/2, it.Y)
		_, y1 := cellOfPage(it.X, it.Y+it.H)
		for i, r := range rs {
			if y0+i > y1 {
				break
			}
			c.put(x-c.v.X, y0+i-c.v.Y, r, col)
		}
		return
	}
	_, row := cellOfPage(it.X, it.Y+it.H/2)
	var x int
	switch it.Align {
	case "r":
		x = int(math.Round((it.X+it.W)/pagePxPerCol)) - len(rs)
	case "c":
		x = int(math.Round((it.X+it.W/2)/pagePxPerCol - float64(len(rs))/2))
	default:
		x = int(math.Round(it.X / pagePxPerCol))
	}
	for i, r := range rs {
		if r == ' ' {
			continue
		}
		c.put(x+i-c.v.X, row-c.v.Y, r, col)
	}
}

func (c *canvas) put(x, y int, r rune, col color.RGBA) {
	if x < 0 || y < 0 || x >= c.v.W || y >= c.v.H {
		return
	}
	c.ch[y*c.v.W+x] = r
	c.ink[y*c.v.W+x] = col
}

// sees reports whether an item can touch the window at all.
func (c *canvas) sees(it *rackpic.Item) bool {
	x0, y0 := float64(c.v.X)*pagePxPerCol, float64(c.v.Y)*pagePxPerRow
	x1, y1 := x0+float64(c.v.W)*pagePxPerCol, y0+float64(c.v.H)*pagePxPerRow
	// Text can run past its box, so a word is let in from a little further.
	slack := 0.0
	if it.Text != "" {
		slack = float64(len(it.Text)) * pagePxPerCol
	}
	return it.X+it.W+slack >= x0 && it.X-slack <= x1 && it.Y+it.H >= y0 && it.Y <= y1
}

// drawPicture paints the window of the picture, the control the keys are on
// outlined, and the canvases with the pixels in imgs (by item index).
func drawPicture(pt Painter, pic *rackpic.Picture, v racksurface.View, cursor string, imgs map[int]*rackpic.Image) {
	c := newCanvas(v)
	for i, it := range pic.Items {
		if it == nil || !c.sees(it) {
			continue
		}
		if m := imgs[i]; m != nil && it.Canvas != 0 {
			c.image(it, m)
			continue
		}
		if it.Pointer() {
			col := pointerInk
			if it.Fill != nil {
				col = rackpic.RGB(*it.Fill)
			}
			c.pointer(it, col)
			continue
		}
		if it.Fill != nil {
			c.fill(it, rackpic.RGB(*it.Fill))
		}
		if len(it.Ticks) > 0 {
			c.ticks(it)
		}
		if it.Border != nil {
			c.outline(it.X, it.Y, it.W, it.H, rackpic.RGB(*it.Border))
		}
		if it.Text != "" && it.Ink != nil {
			c.text(it, rackpic.RGB(*it.Ink))
		}
	}
	if r, ok := pic.Ctls[cursor]; ok {
		c.outline(r[0]-1, r[1]-1, r[2]+2, r[3]+2, cursorInk)
	}
	for y := range v.H {
		for x := range v.W {
			top, bot := c.px[2*y*c.pw+x], c.px[(2*y+1)*c.pw+x]
			if r := c.ch[y*v.W+x]; r != 0 {
				pt.Set(x, y, cellAt{Ch: r, Art: true, Top: rgb3(c.ink[y*v.W+x]), Bottom: rgb3(mix(top, bot))})
				continue
			}
			pt.Set(x, y, cellAt{Ch: '▀', Art: true, Top: rgb3(top), Bottom: rgb3(bot)})
		}
	}
}

func rgb3(c color.RGBA) [3]uint8 { return [3]uint8{c.R, c.G, c.B} }

// mix is the ground a character stands on: the two pixels it covers.
func mix(a, b color.RGBA) color.RGBA {
	return color.RGBA{uint8((int(a.R) + int(b.R)) / 2), uint8((int(a.G) + int(b.G)) / 2), uint8((int(a.B) + int(b.B)) / 2), 255} //nolint:gosec // the mean of two bytes is a byte
}

// ── the panel in the page look ───────────────────────────────────────────

// loadPicture takes the whole picture, when the look is the page's and the
// Source can give one. A Source that cannot leaves the panel on the drawn
// look, and says why.
func (p *panel) loadPicture() {
	p.pic = nil
	src, ok := p.src.(Pictured)
	if look != LookPage || !ok {
		return
	}
	pic, err := src.Picture()
	if err != nil {
		p.err = "the page look: " + err.Error()
		return
	}
	p.pic = pic
}

// refreshPicture brings the picture up to date, and reports a change.
func (p *panel) refreshPicture() bool {
	src, ok := p.src.(Pictured)
	if p.pic == nil || !ok {
		return false
	}
	d, err := src.PictureChanges(p.pic.Gen)
	if err != nil {
		p.err = err.Error()
		return true
	}
	return p.pic.Apply(d)
}

// surface is what the view moves over: the picture, or the drawn rack.
func (p *panel) surface() racksurface.Surface {
	if p.pic != nil {
		return pageSurface(p.pic)
	}
	return p.surf
}

// The panel's window over the scene, as the page floats its controls in a
// window over the model.
const (
	winHalf   = iota // the right half of the terminal, the scene beside it
	winFull          // the whole terminal: the panel and nothing else
	winHidden        // the scene alone
)

// winInk is the window's title bar, the page's.
var winInk = color.RGBA{0, 102, 255, 255}

// layoutWindow places the panel's window on a w x h terminal: the view's
// size and where it is drawn. The last row is the status line.
func (p *panel) layoutWindow(w, h int) {
	body := maxi(h-1, 1)
	switch p.winMode {
	case winFull:
		p.winX, p.winY = 0, 0
		p.view.W, p.view.H = maxi(w-1, 1), maxi(body-1, 1)
	case winHidden:
		p.winX, p.winY = w, 0
		p.view.W, p.view.H = 0, 0
	default:
		if p.nativeNow() {
			p.layoutFloat(w, h)
			return
		}
		ww := maxi(w/2, 24)
		p.winX, p.winY = maxi(w-ww, 0), 1
		p.view.W, p.view.H = maxi(ww-1, 1), maxi(body-2, 1)
	}
}

// layoutFloat places the window over a Native overlay, as the page places
// its controls: docked against an edge at the page's drawer size, or
// floating at the page's own window size where it was last put, kept on the
// terminal. floatX, floatY is the floating window's title row's left end;
// winR is the window either way, and its body starts a row below its top.
func (p *panel) layoutFloat(w, h int) {
	nv := p.src.(Native) //nolint:forcetypeassert // nativeNow checked
	if p.dock != "" {
		p.layoutDocked(nv, w, h)
		return
	}
	ww, wh := nv.WindowCells()
	if ww <= 0 || wh <= 0 {
		ww, wh = maxi(w/3, 24), maxi(h-4, 4)
	}
	ww, wh = min(max(ww, 8), w), min(max(wh, 3), h)
	p.floatX, p.floatY = min(max(p.floatX, 0), w-ww), min(max(p.floatY, 0), h-wh)
	p.placeWin(Rect{p.floatX, p.floatY, ww, wh})
}

// layoutDocked places the window against p.dock, across the whole edge.
func (p *panel) layoutDocked(nv Native, w, h int) {
	across := w
	if p.dock == "top" || p.dock == "bottom" {
		across = h
	}
	n := nv.DockCells(p.dock)
	if n <= 0 {
		n = across / 2
	}
	n = min(max(n, 3), across)
	switch p.dock {
	case "top":
		p.placeWin(Rect{0, 0, w, n})
	case "bottom":
		p.placeWin(Rect{0, h - n, w, n})
	case "left":
		p.placeWin(Rect{0, 0, n, h})
	default:
		p.placeWin(Rect{w - n, 0, n, h})
	}
}

// placeWin puts the window at r, its title row at the top. Its last column
// and row are the bars, as the half window's are.
func (p *panel) placeWin(r Rect) {
	p.winR = r
	p.winX, p.winY = r.X, r.Y+1
	p.view.W, p.view.H = maxi(r.W-1, 1), maxi(r.H-2, 1)
}

// drawPage is drawRack for the page look: the scene, and the panel's window
// over it.
func (p *panel) drawPage(sc tcell.Screen, w, h int) {
	p.scrW, p.scrH = w, h
	p.layoutWindow(w, h)
	if p.winMode != winFull {
		p.drawScene(sc, w, maxi(h-1, 1))
	}
	if p.winMode == winHidden {
		return
	}
	if p.winMode == winHalf {
		x0, x1, y := p.winX, w, 0
		if p.nativeNow() {
			x0, x1, y = p.winR.X, p.winR.X+p.winR.W, p.winR.Y
		}
		title := clip(" chaosrack controls", maxi(x1-x0, 0))
		st := styleOf(cellAt{Art: true, Top: [3]uint8{255, 255, 255}, Bottom: rgb3(winInk)})
		for x := x0; x < x1; x++ {
			sc.SetContent(x, y, ' ', nil, st)
		}
		puts(sc, x0, y, title, st)
	}
	vw, vh := p.view.W, p.view.H
	s := pageSurface(p.pic)
	p.view = p.view.Clamp(s)
	if c, ok := p.at(); ok && c.ID != p.shown {
		if x, y, cw, ch, ok := ctlRectOfPage(p.pic, c.ID); ok {
			p.view = p.view.Reveal(x, y, cw, ch, s)
		}
		p.shown = c.ID
	}
	cursor := ""
	if c, ok := p.at(); ok {
		cursor = c.ID
	}
	drawPicture(screenPainter{sc: sc, x0: p.winX, y0: p.winY, w: vw, h: vh}, p.pic, p.view, cursor, p.canv)
	// The bars, along the window's right and bottom edges, on a ground of
	// their own so the scene does not show through them.
	for y := range vh + 1 {
		sc.SetContent(p.winX+vw, p.winY+y, ' ', nil, stNormal)
	}
	for x := range vw {
		sc.SetContent(p.winX+x, p.winY+vh, ' ', nil, stNormal)
	}
	if pos, n := racksurface.Bar(p.view.Y, vh, s.Rows, vh); n > 0 {
		for i := range n {
			sc.SetContent(p.winX+vw, p.winY+pos+i, '█', nil, stFrame)
		}
	}
	if pos, n := racksurface.Bar(p.view.X, vw, s.Cols, vw); n > 0 {
		for i := range n {
			sc.SetContent(p.winX+pos+i, p.winY+vh, '▀', nil, stFrame)
		}
	}
}

// drawScene paints the model's canvas across w x h cells, half a cell a
// pixel, as it was last sampled.
func (p *panel) drawScene(sc tcell.Screen, w, h int) {
	m := p.scene
	for y := range h {
		for x := range w {
			top, bot := m.At(x, 2*y), m.At(x, 2*y+1)
			sc.SetContent(x, y, '▀', nil, styleOf(cellAt{Art: true, Top: rgb3(top), Bottom: rgb3(bot)}))
		}
	}
}

// refreshPixels samples the scene and the canvases the window shows, and
// reports whether there were any to sample.
func (p *panel) refreshPixels() bool {
	// The parts cover these cells; reading the page back for them would
	// cost the page a frame for pixels nobody sees.
	if p.nativeNow() {
		return false
	}
	src, ok := p.src.(Pictured)
	if p.pic == nil || !ok || p.scrW == 0 {
		return false
	}
	got := false
	if p.winMode != winFull {
		// A pixel is half a cell: its height over its width is half the
		// cell's.
		if m, err := src.Scene(p.scrW, 2*maxi(p.scrH-1, 1), panelart.CellAspect()/2); err == nil {
			quantize(m, p.levels())
			p.scene, got = m, true
		}
	}
	if p.winMode != winHidden {
		if want := canvasWants(p.pic, p.view); len(want) > 0 {
			if imgs, err := src.Canvases(p.pic.Gen, want); err == nil {
				for _, m := range imgs {
					quantize(m, p.levels())
				}
				p.canv, got = imgs, true
			}
		}
	}
	return got
}

// canvasWants is the canvases the view shows, each at the size it is drawn.
func canvasWants(pic *rackpic.Picture, v racksurface.View) []rackpic.CanvasWant {
	c := &canvas{v: v}
	var out []rackpic.CanvasWant
	for i, it := range pic.Items {
		if it == nil || it.Canvas == 0 || !c.sees(it) {
			continue
		}
		x0, x1, y0, y1 := c.span(it.X, it.Y, it.W, it.H)
		out = append(out, rackpic.CanvasWant{Index: i, W: x1 - x0 + 1, H: y1 - y0 + 1})
	}
	return out
}

// image paints a canvas's pixels over its box, sampled as the box is.
func (c *canvas) image(it *rackpic.Item, m *rackpic.Image) {
	x0, x1, y0, y1 := c.span(it.X, it.Y, it.W, it.H)
	w, h := x1-x0+1, y1-y0+1
	for py := max(y0, 0); py <= min(y1, c.ph-1); py++ {
		for px := max(x0, 0); px <= min(x1, c.pw-1); px++ {
			c.px[py*c.pw+px] = m.At((px-x0)*m.W/w, (py-y0)*m.H/h)
		}
	}
}

// mousePage is the mouse over the page look: the control under the pointer
// is chosen, and the click or the wheel goes to the page, to whatever is
// under the same point there — a button presses, a knob turns, a lamp
// lights, by the page's own handlers.
func (p *panel) mousePage(b tcell.ButtonMask, pressed bool, x, y int) {
	src, ok := p.src.(Pictured)
	if !ok {
		return
	}
	px, py := pageOfCell(p.view.X+x, p.view.Y+y)
	id, onCtl := ctlAtPage(p.pic, px, py)
	act := func(kind string, delta float64) {
		if err := src.Act(p.pic.Gen, px, py, kind, delta); err != nil {
			p.err = err.Error()
		}
	}
	switch {
	case b&(tcell.WheelUp|tcell.WheelDown) != 0:
		dir := 1
		if b&tcell.WheelDown != 0 {
			dir = -1
		}
		if !onCtl {
			p.pan(0, -dir)
			return
		}
		p.choose(id)
		act("wheel", float64(-100*dir))
	case b&tcell.WheelLeft != 0:
		p.pan(-1, 0)
	case b&tcell.WheelRight != 0:
		p.pan(1, 0)
	case pressed:
		if onCtl {
			p.choose(id)
		}
		act("click", 0)
	}
}

// ticks draws a dial's marks, the finest first so the major ones are on top.
func (c *canvas) ticks(it *rackpic.Item) {
	const pxW, pxH = pagePxPerCol, pagePxPerRow / 2
	cx, cy := it.X+it.W/2, it.Y+it.H/2
	r := math.Min(it.W, it.H) / 2
	in := it.Inner
	if in <= 0 || in >= 1 {
		in = 0.7
	}
	for i := len(it.Ticks) - 1; i >= 0; i-- {
		period, col := it.Ticks[i][0], rackpic.RGB(int(it.Ticks[i][2]))
		if period <= 0 {
			continue
		}
		for deg := 0.0; deg < 360-1e-9; deg += period {
			a := deg * math.Pi / 180
			for t := r * in; t <= r; t += 0.5 {
				x, y := cx+t*math.Sin(a), cy-t*math.Cos(a)
				c.set(int(math.Floor(x/pxW))-c.v.X, int(math.Floor(y/pxH))-2*c.v.Y, col)
			}
		}
	}
}

// mouseWindow takes the mouse in the page look: the window's scroll bars and
// the panel in it; and the scene around it (scenemouse.go).
func (p *panel) mouseWindow(b tcell.ButtonMask, pressed bool, x, y int) {
	if p.mouseScene(b, pressed, x, y) {
		return
	}
	if p.winMode == winHidden {
		return
	}
	lx, ly := x-p.winX, y-p.winY
	if p.scrollBar(b, pressed, lx, ly, p.view.W, p.view.H) {
		return
	}
	if lx < 0 || ly < 0 || lx >= p.view.W || ly >= p.view.H {
		return
	}
	p.mousePage(b, pressed, lx, ly)
}

// Quantizer is a Source whose terminal pays for every new color: xterm-go,
// in a page, rasterizes a glyph for each new foreground and background pair
// and uploads its whole atlas again. A scene shrunk to cells is a stream of
// colors never seen before, so there it is drawn in a bounded palette of
// Levels() steps a channel, and the atlas fills once and stops.
type Quantizer interface {
	Levels() int
}

// quantize puts an image's pixels on n steps a channel; n < 2 leaves it.
func quantize(m *rackpic.Image, n int) {
	if m == nil || n < 2 {
		return
	}
	step := 255.0 / float64(n-1)
	for i, v := range m.Px {
		m.Px[i] = uint8(math.Round(math.Round(float64(v)/step) * step)) //nolint:gosec // within 0..255 by construction
	}
}

// levels is how many steps a channel the Source asks for, 0 for any.
func (p *panel) levels() int {
	if q, ok := p.src.(Quantizer); ok {
		return q.Levels()
	}
	return 0
}
