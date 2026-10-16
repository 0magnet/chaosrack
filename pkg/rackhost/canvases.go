package rackhost

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/0magnet/chaosrack/pkg/rackpic"
)

// The panel's screens, natively: what the page draws into the canvases set
// in its panel — the model bay's monitor (rowmonitor_js.go), the Record
// module's monitor and the four scopes' tubes — drawn here for the panel to
// lay over its picture of them.

// The kinds of screen.
const (
	canvasMonitor = iota // the model bay's: the model, or STANDBY
	canvasRecord         // the Record module's: what would be recorded
	canvasScope          // a scope's tube
)

// panelCanvas is what one of the picture's canvases is: its kind, and for a
// scope which.
type panelCanvas struct{ kind, n int }

// canvasesOf is what each canvas of the picture is, by its item: each is
// told by the control nearest under it — the monitor's power switch, a
// scope's VOLTS — and the one with neither is the Record module's. Called
// with r.mu held.
func (r *Rack) canvasesOf() map[int]panelCanvas {
	if r.canvasKinds != nil || r.pic == nil {
		return r.canvasKinds
	}
	under := map[string]panelCanvas{"bay-visual-0-mon-on": {kind: canvasMonitor}}
	for n := range scopeCount {
		under[scopePrefix(n)+"-volts"] = panelCanvas{kind: canvasScope, n: n}
	}
	r.canvasKinds = map[int]panelCanvas{}
	for i, it := range r.pic.Items {
		if it == nil || it.Canvas == 0 {
			continue
		}
		bx, by := it.X+it.W/2, it.Y+it.H
		best, kind := 350.0, panelCanvas{kind: canvasRecord}
		for id, k := range under {
			b, ok := r.pic.Ctls[id]
			if !ok {
				continue
			}
			if d := math.Hypot(b[0]+b[2]/2-bx, b[1]-by); d < best {
				best, kind = d, k
			}
		}
		r.canvasKinds[i] = kind
	}
	return r.canvasKinds
}

// Canvases draws the screens the panel shows, each at the size it is drawn.
func (r *Rack) Canvases(_ int, want []rackpic.CanvasWant) (map[int]*rackpic.Image, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kinds := r.canvasesOf()
	out := map[int]*rackpic.Image{}
	for _, w := range want {
		k, ok := kinds[w.Index]
		if !ok || w.W <= 0 || w.H <= 0 {
			continue
		}
		var m *image.RGBA
		switch k.kind {
		case canvasScope:
			m = r.scopeFrame(k.n, w.W, w.H)
		case canvasMonitor:
			m = r.monitorFrame(w.W, w.H, r.value("bay-visual-0-mon-on") != "0", "")
		default:
			// No recorder runs on the host: the monitor shows what one would
			// take, standing by.
			m = r.monitorFrame(w.W, w.H, true, "STBY")
		}
		out[w.Index] = toRackImage(m)
	}
	return out, nil
}

// monitorFrame is a monitor w by h: the model as last drawn, fitted and
// captioned, or with the rack off or the screen's switch down, standing by
// with the model its bay would play. tally, when set, is burned in at the
// top right. Called with r.mu held.
func (r *Rack) monitorFrame(w, h int, power bool, tally string) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRGBA(m, scopeDark, 1)
	name := r.modelName(r.model())
	if !power || r.value("power-sw") == "0" {
		drawCentered(m, h/2-7, "STANDBY", color.RGBA{42, 74, 58, 255})
		drawCentered(m, h/2+8, name, color.RGBA{58, 106, 82, 255})
		return m
	}
	var shown *image.RGBA
	if src, ok := r.monitorTrend(m); ok {
		// Recurrence's model is on the scene already: with TREND up the
		// monitor carries the history of RR, DET and LAM instead.
		name = "RQA · " + src
	} else {
		// Without the scene's lock, which is taken before r.mu and never
		// under it (scene.mu).
		shown = r.scene.monitor.Load()
	}
	if shown != nil {
		// Fit, not fill: a monitor that stretched its picture would make
		// every model the shape of the monitor.
		sw, sh := float64(shown.Rect.Dx()), float64(shown.Rect.Dy())
		k := math.Min(float64(w)/sw, float64(h)/sh)
		dw, dh := int(sw*k), int(sh*k)
		ox, oy := (w-dw)/2, (h-dh)/2
		for y := range dh {
			for x := range dw {
				m.SetRGBA(ox+x, oy+y, shown.RGBAAt(int(float64(x)/k), int(float64(y)/k)))
			}
		}
	}
	if name != "" {
		// The caption, on a dark band at the bottom left, as the page's.
		tw := font.MeasureString(basicfont.Face7x13, name).Ceil()
		band := image.Rect(0, h-14, tw+10, h)
		draw.Draw(m, band, &image.Uniform{color.RGBA{0, 0, 0, 140}}, image.Point{}, draw.Over)
		drawText(m, 5, h-13, name, color.RGBA{143, 227, 176, 255}, 1)
	}
	if tally != "" {
		tw := font.MeasureString(basicfont.Face7x13, tally).Ceil()
		drawText(m, w-tw-5, 2, tally, color.RGBA{220, 60, 50, 255}, 1)
	}
	return m
}

// drawText writes s with its top left at x, y, in c at alpha a. A character
// basicfont has no glyph for is written as the nearest it has: a dash for a
// dot or a long dash, a space for the rest.
func drawText(m *image.RGBA, x, y int, s string, c color.RGBA, a float64) {
	s = strings.Map(func(ch rune) rune {
		if _, ok := basicfont.Face7x13.GlyphAdvance(ch); ok {
			return ch
		}
		if strings.ContainsRune("·•–—", ch) {
			return '-'
		}
		return ' '
	}, s)
	k := max(0, min(a, 1))
	src := color.RGBA{uint8(float64(c.R) * k), uint8(float64(c.G) * k), uint8(float64(c.B) * k), uint8(255 * k)} //nolint:gosec // scaled bytes are bytes
	d := font.Drawer{Dst: m, Src: image.NewUniform(src), Face: basicfont.Face7x13,
		Dot: fixed.P(x, y+basicfont.Face7x13.Ascent)}
	d.DrawString(s)
}

// drawCentered writes s centered across m with its middle at y.
func drawCentered(m *image.RGBA, y int, s string, c color.RGBA) {
	if s == "" {
		return
	}
	tw := font.MeasureString(basicfont.Face7x13, s).Ceil()
	drawText(m, (m.Rect.Dx()-tw)/2, y-6, s, c, 1)
}

// toRackImage is m as the panel takes a canvas: three bytes a pixel.
func toRackImage(m *image.RGBA) *rackpic.Image {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	out := &rackpic.Image{W: w, H: h, Px: make([]byte, 3*w*h)}
	for i := range w * h {
		copy(out.Px[3*i:3*i+3], m.Pix[4*i:4*i+3])
	}
	return out
}

// modelName is model key as the page names it: the selector's option reads
// "Lorenz" padded with no-break spaces to a count beside it, and the name
// is the part before them.
func (r *Rack) modelName(key string) string {
	i, ok := r.byID[r.selector]
	if !ok {
		return key
	}
	name, _, _ := strings.Cut(labelOf(&r.ctls[i], key), "\u00a0")
	return strings.TrimSpace(name)
}
