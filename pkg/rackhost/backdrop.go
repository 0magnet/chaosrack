package rackhost

import (
	"image"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// The Layers module's BEHIND, natively: a second picture filling the scene
// behind an ordinary model — the spectrogram, the XY scope, the Terminal,
// the animation or the desk — so the model floats over it, as the page
// paints its backdrop before the model (backdrop_js.go). Water, which the
// page draws through the finished frame rather than behind it, is not here.

// backdrop is the picture behind model key, w by h, or nil: none chosen, the
// rack off, or a model that fills the scene itself. Called with r.mu held.
func (r *Rack) backdrop(key string, on bool, w, h int) *image.RGBA {
	kind := r.value("bg-visual")
	if kind == "" || kind == "water" || !on || listens(key) || isTerminalModel(key) {
		return nil
	}
	nowMs := float64(r.sound.now().UnixNano()) / 1e6
	switch kind {
	case "spectrogram":
		if q := r.spectrogram(nowMs, false); q != nil {
			return drawFlat(q, w, h)
		}
	case "xy":
		fig, _, fit := r.audioFigure("xy")
		return attractor.DrawFigure(fig, 0, len(fig.Points), attractor.DrawOptions{
			Width: w, Height: h, Fit: fit, Background: sceneBack,
			View: rasterview.View{Scale: 0.85, PixelAspect: 1},
		})
	case "terminal":
		return drawFlat(r.shellOf("terminal").figure(), w, h)
	case "termanim":
		if q := r.animFigure(); q != nil {
			return drawFlat(q, w, h)
		}
	case "desk":
		r.desk.sync()
		if qs := r.desk.quads("flat"); len(qs) > 0 {
			return drawQuads(qs, rasterview.View{Scale: 0.85, PixelAspect: 1}, w, h)
		}
	}
	return nil
}

// withBackdrop is frame m over backdrop bd, a picture of its own: m is kept
// as it was drawn, for Persist and Glow to draw the next over.
func withBackdrop(m, bd *image.RGBA) *image.RGBA {
	if bd == nil || bd.Bounds() != m.Bounds() {
		return m
	}
	out := image.NewRGBA(m.Rect)
	copy(out.Pix, m.Pix)
	under(out, bd)
	return out
}
