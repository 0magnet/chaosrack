package attractor

import "testing"

// TestTheTraceColoringIsTheShaders: a figure drawn off the GPU (the Lattice's)
// is colored as the trace shader colors it, for the Colors module's settings.
func TestTheTraceColoringIsTheShaders(t *testing.T) {
	red, green, blue := [3]float32{1, 0, 0}, [3]float32{0, 1, 0}, [3]float32{0, 0, 1}
	lo, hi := [3]float32{-1, -1, -1}, [3]float32{1, 1, 1}
	low, mid, high := [3]float32{0, 0, -1}, [3]float32{0, 0, 0}, [3]float32{0, 0, 1}
	c := traceColoring{src: 2, cols: 2, base: red, mid: green, top: blue, freq: 1, lut: []float32{0, 1}}
	near := func(a, b [3]float32) bool {
		for i := range 3 {
			if d := a[i] - b[i]; d > 0.01 || d < -0.01 {
				return false
			}
		}
		return true
	}
	for _, k := range []struct {
		name string
		c    traceColoring
		p    [3]float32
		age  float32
		want [3]float32
	}{
		{"Z, two-color, the bottom", c, low, 0, red},
		{"Z, two-color, the top", c, high, 0, blue},
		{"reversed", func() traceColoring { r := c; r.reverse = true; return r }(), low, 0, blue},
		{"SRC off is the start swatch", func() traceColoring { r := c; r.src = GradientSourceOff; return r }(), high, 0, red},
		{"three-color through the middle", func() traceColoring { r := c; r.cols = 3; return r }(), mid, 0, green},
		{"trail age", func() traceColoring { r := c; r.src = 3; return r }(), low, 1, blue},
		{"an audio source reads its table", func() traceColoring { r := c; r.src = 4; return r }(), low, 1, blue},
	} {
		if got := k.c.at(k.p, k.age, lo, hi); !near(got, k.want) {
			t.Errorf("%s: %v, want %v", k.name, got, k.want)
		}
	}
}
