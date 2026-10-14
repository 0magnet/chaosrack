package rackhost

import (
	"image"
	"image/draw"
	"math"
	"time"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/colormap"
)

// The analyzers, drawn from the rack's signal as the page draws them from its
// tap: the measurement and the lines are the page's own (pkg/attractor), and
// the lines are laid flat over the scene, the page's clip space stretched to
// its edges as the page stretches it over its canvas.

// analyzers are the native rack's analyzers and when they last looked.
type analyzers struct {
	rta  *attractor.RTA
	xfer *attractor.Transfer
	last time.Time
}

func newAnalyzers() analyzers {
	return analyzers{rta: attractor.NewRTA(), xfer: attractor.NewTransfer()}
}

// isAnalyzer reports whether the native rack draws model key as a flat
// analyzer.
func isAnalyzer(key string) bool {
	return key == "rta" || key == "xfer"
}

// listens reports whether model key is drawn from the rack's signal.
func listens(key string) bool {
	return attractor.IsAudioModel(key) || isAnalyzer(key)
}

// plotPass is lines drawn at one width.
type plotPass struct {
	v     []attractor.PlotVertex
	width int
}

// analyzerPlot is analyzer key's lines now, measured from the newest of the
// rack's signal at the panel's knobs, for a frame w wide. Called with r.mu
// held.
func (r *Rack) analyzerPlot(key string, w int) []plotPass {
	a := &r.analyzers
	now := r.sound.now()
	// The page's ballistics are per frame at 60 a second; the panel's frames
	// come slower, so each look advances by the frames it stood in for.
	frames := 1.0
	if !a.last.IsZero() {
		frames = math.Min(now.Sub(a.last).Seconds()*60, 60)
	}
	a.last = now
	nowMs := float64(now.UnixNano()) / 1e6
	switch key {
	case "rta":
		m := a.rta
		for _, id := range attractor.RTAKnobs {
			m.Set(id, float32(r.number(id)))
		}
		if l, rt := r.sound.window(attractor.RTAFFT); len(l) == attractor.RTAFFT && m.Due(nowMs) {
			m.Analyze(m.Fold(l, rt), sampleRate)
		}
		m.Advance(frames)
		bars, marks := m.Lines(r.analyzerColors())
		// The page widens its bars to about 2.5 of its thousand-odd pixels
		// and draws its marks one pixel high.
		return []plotPass{
			{v: clone(bars), width: max(1, int(math.Round(float64(w)/400)))},
			{v: clone(marks), width: 1},
		}
	case "xfer":
		m := a.xfer
		for _, id := range attractor.TransferKnobs {
			m.Set(id, float32(r.number(id)))
		}
		if l, rt := r.sound.window(attractor.XferFFT); len(l) == attractor.XferFFT && m.Due(nowMs) {
			m.Analyze(l, rt, sampleRate)
		}
		// The page draws its curves about three of its pixels thick.
		return []plotPass{{v: clone(m.Lines(r.analyzerColors())), width: max(1, int(math.Round(float64(w)/400)))}}
	}
	return nil
}

// analyzerColors is how the Colors module colors an analyzer: its colormap,
// when it names one, and the scope's green otherwise. Called with r.mu held.
func (r *Rack) analyzerColors() attractor.AnalyzerColors {
	m, ok := colormap.Index(int(r.number("gradient-colors")))
	return attractor.AnalyzerColors{Map: m, Mapped: ok, Flat: attractor.ScopeGreen}
}

func clone(v []attractor.PlotVertex) []attractor.PlotVertex {
	return append([]attractor.PlotVertex(nil), v...)
}

// drawPlot draws passes over a w by h frame of the scene's background.
func drawPlot(passes []plotPass, w, h int) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(m, m.Bounds(), &image.Uniform{sceneBack}, image.Point{}, draw.Src)
	for _, p := range passes {
		for i := 0; i+1 < len(p.v); i += 2 {
			plotLine(m, p.v[i], p.v[i+1], p.width)
		}
	}
	return m
}

// plotLine draws a line between two clip-space ends, its color running from
// one end's to the other's, width pixels across.
func plotLine(m *image.RGBA, a, b attractor.PlotVertex, width int) {
	w, h := float64(m.Rect.Dx()), float64(m.Rect.Dy())
	// Clip space's -1..1 spans the frame; a pixel's center is at +0.5.
	ax, ay := (float64(a.X)+1)/2*w-0.5, (1-float64(a.Y))/2*h-0.5
	bx, by := (float64(b.X)+1)/2*w-0.5, (1-float64(b.Y))/2*h-0.5
	steps := int(math.Ceil(math.Max(math.Abs(bx-ax), math.Abs(by-ay))))
	lo := -(width - 1) / 2
	for s := 0; s <= steps; s++ {
		t := 0.0
		if steps > 0 {
			t = float64(s) / float64(steps)
		}
		x, y := int(math.Round(ax+(bx-ax)*t)), int(math.Round(ay+(by-ay)*t))
		var c [3]uint8
		for k := range c {
			c[k] = uint8(math.Round(255 * math.Max(0, math.Min(1, float64(a.C[k]+(b.C[k]-a.C[k])*float32(t))))))
		}
		for dy := lo; dy < lo+width; dy++ {
			for dx := lo; dx < lo+width; dx++ {
				if !(image.Point{x + dx, y + dy}).In(m.Rect) {
					continue
				}
				o := m.PixOffset(x+dx, y+dy)
				m.Pix[o], m.Pix[o+1], m.Pix[o+2], m.Pix[o+3] = c[0], c[1], c[2], 255
			}
		}
	}
}

// heard is what the rack's signal draws this frame: an audio model's figure,
// the lines and frame it is read against and the XY's glow, or an analyzer's
// lines.
type heard struct {
	fig, grat attractor.Figure
	fit       [][3]float64
	glow      float64
	plot      []plotPass
}

// hear measures what model key draws from the rack's signal, in a frame w
// wide, while the rack is on. Called with r.mu held.
func (r *Rack) hear(key string, on bool, w int) (h heard) {
	switch {
	case attractor.IsAudioModel(key):
		h.fig, h.grat, h.fit = r.audioFigure(key)
		if key == "xy" {
			h.glow = r.number("xy-persist")
		}
	case isAnalyzer(key) && on:
		h.plot = r.analyzerPlot(key, w)
	}
	return h
}
