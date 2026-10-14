package attractor

import "github.com/0magnet/chaosrack/pkg/colormap"

// What the flat analyzers draw, shared by the page and a native rack: lines
// in the page's clip space, each end with its own color. The page uploads
// them to its per-vertex-color program (vcolor_js.go); a native rack
// rasterizes them.

// PlotVertex is one end of a flat analyzer's line, in clip space: -1..1
// across the screen and up it.
type PlotVertex struct {
	X, Y float32
	C    [3]float32
}

// AnalyzerColors is how a flat analyzer colors what it measures: through the
// colormap the Colors module names, when it names one, and in one trace color
// otherwise.
type AnalyzerColors struct {
	Map    int // colormap.Index's
	Mapped bool
	Flat   [3]float32
}

// ScopeGreen is a flat analyzer's trace color with no colormap and no
// phosphor: what these displays drew in before they could be colored.
var ScopeGreen = [3]float32{0.4, 1.0, 0.45}

// At is the color of a 0..1 value.
func (c AnalyzerColors) At(v float64) [3]float32 {
	if !c.Mapped {
		return c.Flat
	}
	return analyzerColorAt(c.Map, v)
}

// analyzerColorAt samples a colormap at a 0..1 value, as three floats.
func analyzerColorAt(idx int, v float64) [3]float32 {
	c := colormap.At(idx, v)
	r, g, b, _ := c.RGBA()
	return [3]float32{float32(r) / 65535, float32(g) / 65535, float32(b) / 65535}
}
