package rackhost

import (
	"math"
	"time"

	"github.com/0magnet/chaosrack/pkg/attractor"
)

// The scope models the native rack draws a frame at a time, from the page's
// own generators (pkg/attractor): the Graphic Artist's drifting wireframe,
// the scope clock, and Fourier Text's banner.

// scopeModels are their running state.
type scopeModels struct {
	ga   *attractor.GraphicArtist
	text *attractor.FourierText
	last time.Time
}

func newScopeModels() scopeModels {
	return scopeModels{ga: attractor.NewGraphicArtist(), text: attractor.NewFourierText()}
}

// isScopeModel reports whether model key is one of them.
func isScopeModel(key string) bool {
	return key == "graphicartist" || key == "scopeclock" || key == "scopetext"
}

// scopeFigure is scope model key now, stepped on to now while running, and
// the frame it is drawn in. Called with r.mu held.
func (r *Rack) scopeFigure(key string, running bool) (attractor.Figure, [][3]float64) {
	sm := &r.analyzers.scope
	now := r.sound.now()
	frames := 0.0
	if running && !sm.last.IsZero() {
		// The page moves them on a frame at 60 a second.
		frames = math.Min(now.Sub(sm.last).Seconds()*60, 60)
	}
	sm.last = now
	sub, scale := speedSteps(r.number("speed-slider"))
	steps := max(int(r.number("trail-slider")), 2)
	switch key {
	case "graphicartist":
		for _, id := range attractor.GraphicArtistKnobs {
			sm.ga.Set(id, float32(r.number(id)))
		}
		sm.ga.Drift(frames, float32(scale))
		f := attractor.Figure{Kind: attractor.FigurePath, Points: make([][3]float64, steps)}
		sm.ga.Trace(steps, func(i int, x, y, z float64) { f.Points[i] = [3]float64{x, y, z} })
		return f, [][3]float64{{-1.5, 0, 0}, {1.5, 0, 0}}
	case "scopeclock":
		f := attractor.Figure{Kind: attractor.FigurePath, Points: make([][3]float64, steps)}
		if !attractor.ClockTrace(time.Now(), steps, func(i int, x, y float64) { f.Points[i] = [3]float64{x, y, 0} }) {
			f.Points = nil
		}
		return f, square(f.Points)
	case "scopetext":
		sm.text.Set("stext-harm", float32(r.number("stext-harm")))
		sm.text.Sweep(frames, sub, float32(scale))
		f := attractor.Figure{Kind: attractor.FigureLines}
		for _, s := range sm.text.Strokes() {
			// Each stroke on its own, as the beam is blanked between them.
			for k := 0; k+1 < len(s); k += 2 {
				if k > 0 && len(f.Points) < 65535 {
					f.Edges = append(f.Edges, uint16(len(f.Points)-1), uint16(len(f.Points))) //nolint:gosec // held under the index type's reach
				}
				f.Points = append(f.Points, [3]float64{s[k], s[k+1], 0})
			}
		}
		return f, square(f.Points)
	}
	return attractor.Figure{}, nil
}

// square is the frame a plane figure is drawn in: its own reach either way,
// so it holds still while it is drawn.
func square(pts [][3]float64) [][3]float64 {
	e := 0.0
	for _, p := range pts {
		e = math.Max(e, math.Max(math.Abs(p[0]), math.Abs(p[1])))
	}
	if e == 0 {
		e = 1
	}
	return [][3]float64{{-e, -e, 0}, {e, e, 0}}
}
