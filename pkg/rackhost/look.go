package rackhost

import (
	"math"
	"strconv"

	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// sceneLook is how the model is drawn: the Layers · Colors module's gradient,
// the Display module's points and line, and the View module's pan.
type sceneLook struct {
	gradient rasterview.Gradient
	dots     bool
	line     float64    // Display's Line, in the page's pixels
	pan      [2]float64 // View's X and Y, in the model's units
}

// look reads the modules that say how the model is drawn. Called with r.mu
// held.
//
// The gradient's source and map are the page's values for them: X, Y, Z and
// the trail are the renderer's sources too, "off" is one color, and the
// sources the page takes from audio (level, dB, pitch and the rest) follow Z
// here, where there is no audio yet. The maps are the renderer's palettes and
// then the named colormaps, in the page's order.
func (r *Rack) look() sceneLook {
	g := rasterview.DefaultGradient()
	switch src := int(r.number("gradient-source")); src {
	case 0, 1, 2, 3:
		g.Source = src
	case 5:
		g.Colors = 1
	default:
		g.Source = 2
	}
	if g.Colors != 1 {
		if cols := int(r.number("gradient-colors")); cols >= 2 {
			g.Colors = cols
		}
	}
	g.Reverse = r.value("gradient-reverse") == "1"
	if f, err := strconv.ParseFloat(r.value("rainbow-freq"), 32); err == nil && f > 0 {
		g.Freq = float32(f)
	}
	g.Phase = float32(r.number("palette-shift"))
	return sceneLook{
		gradient: g,
		dots:     r.value("use-points") == "1",
		line:     r.number("line-width"),
		pan:      [2]float64{r.number("pan-x"), r.number("pan-y")},
	}
}

// pageWidth is about how wide the page draws its model, in its pixels: what
// a Line width is measured against.
const pageWidth = 1000.0

// lineWidth is Display's Line in pixels of a frame w wide: the same share of
// the frame the page's line takes of its own, but never less than 0.3 of a
// pixel a step. A terminal's frame is a few hundred half cells across, and
// each is many of the page's pixels, so a share alone would leave Line 10 a
// single cell wide and the knob doing nothing; at 0.3 it is three.
func lineWidth(line float64, w int) int {
	scale := max(float64(w)/pageWidth, 0.3)
	return max(1, int(math.Round(line*scale)))
}

// flowValues is each flow constant's value as the rack holds it, keyed by the
// variable the integrator reads. Called with r.mu held.
func (r *Rack) flowValues() map[*float32]float32 {
	out := make(map[*float32]float32, len(r.flow))
	for id, p := range r.flow {
		if f, err := strconv.ParseFloat(r.value(id), 32); err == nil {
			out[p.Value] = float32(f)
		}
	}
	return out
}
