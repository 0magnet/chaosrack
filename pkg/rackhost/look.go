package rackhost

import (
	"strconv"

	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// sceneLook is how the model is drawn: the Layers · Colors module's gradient
// and the Display module's points.
type sceneLook struct {
	gradient rasterview.Gradient
	dots     bool
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
	return sceneLook{gradient: g, dots: r.value("use-points") == "1"}
}
