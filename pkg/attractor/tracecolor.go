package attractor

import (
	"github.com/0magnet/chaosrack/pkg/colormap"
	"github.com/0magnet/chaosrack/pkg/colorspace"
)

// The trace's coloring off the GPU: the fragment shader's source × map
// product (render.go, the trace program's fragment shader), for something
// that draws the model's points itself and has to color them as the page
// would. The Lattice does: it draws a model's figure as characters in its
// terminals, and with nothing to say what color each was, every voxel was the
// volume's own pastel, whatever the Colors module was set to.

// traceColoring is what the shader is told about color: SRC and MAP, the
// three swatches, the period, the rainbow's phase and the colormap's shift,
// whether it is reversed, and the audio sources' table along the trail.
type traceColoring struct {
	src, cols        int
	base, mid, top   [3]float32
	freq, phase, sft float32
	reverse          bool
	lut              []float32
}

// at is the color of the point p, age of the way along the trail, in a figure
// reaching lo to hi: the shader's, step for step.
func (c traceColoring) at(p [3]float32, age float32, lo, hi [3]float32) [3]float32 {
	var t float32
	switch {
	case c.src >= 0 && c.src <= 2:
		t = clamp01((p[c.src] - lo[c.src]) / max(hi[c.src]-lo[c.src], 0.001))
	case c.src >= 4 && c.src != GradientSourceOff:
		if n := len(c.lut); n > 0 {
			t = c.lut[int(clamp01(age)*float32(n-1)+0.5)]
		}
	default:
		t = age
	}
	return c.colorOf(t)
}

// colorOf is the color of the value t, 0..1 on the source's scale, through
// the map: the second half of at, and what the color key draws its bar with.
func (c traceColoring) colorOf(t float32) [3]float32 {
	if c.reverse {
		t = 1 - t
	}
	switch {
	case c.cols == 1 || c.src == GradientSourceOff:
		return c.base
	case c.cols == 3:
		if t < 0.5 {
			return mix3(c.base, c.mid, t*2)
		}
		return mix3(c.mid, c.top, (t-0.5)*2)
	case c.cols == 4:
		return colorspace.FromHSV(t*c.freq+c.phase, 1, 1)
	case c.cols >= colormap.First:
		if idx, ok := colormap.Index(c.cols); ok {
			r, g, b, _ := colormap.At(idx, float64(colormap.Fold(t*c.freq+c.sft))).RGBA()
			return [3]float32{float32(r) / 0xffff, float32(g) / 0xffff, float32(b) / 0xffff}
		}
	}
	return mix3(c.base, c.top, t)
}

func mix3(a, b [3]float32, t float32) [3]float32 {
	return [3]float32{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t}
}
