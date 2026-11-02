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

// TestAColoringsKeyIsWhatItColorsBy: two colorings with one key color every
// point of the trail alike, whatever the fields the setting does not read
// hold, and a field it does read moves the key.
func TestAColoringsKeyIsWhatItColorsBy(t *testing.T) {
	seed := uint32(12345)
	next := func() float32 { seed = seed*1664525 + 1013904223; return float32(seed>>8) / (1 << 24) }
	swatch := func() [3]float32 { return [3]float32{next(), next(), next()} }
	random := func(cols int) traceColoring {
		return traceColoring{src: int(next() * 3), cols: cols, base: swatch(), mid: swatch(), top: swatch(),
			freq: 0.5 + next(), phase: next(), sft: next(), reverse: next() < 0.5}
	}
	for cols := 1; cols <= 12; cols++ {
		for range 50 {
			a, b := random(cols), random(cols)
			// b takes over what a's setting reads, keeping its own phase and
			// shift unless the setting reads one of them.
			b.src, b.reverse, b.base, b.mid, b.top, b.freq = a.src, a.reverse, a.base, a.mid, a.top, a.freq
			if cols == 4 {
				b.phase = a.phase
			}
			if cols >= 5 {
				b.sft = a.sft
			}
			if a.keyFor() != b.keyFor() {
				t.Fatalf("cols %d: keys differ for one coloring\n%+v\n%+v", cols, a.keyFor(), b.keyFor())
			}
			for i := range 33 {
				x := float32(i) / 32
				if a.colorOf(x) != b.colorOf(x) {
					t.Fatalf("cols %d: one key, two colors at %v: %v and %v", cols, x, a.colorOf(x), b.colorOf(x))
				}
			}
			// Unread fields moved: same key. Read fields moved: a new one.
			c := a
			c.phase, c.sft = a.phase+0.25, a.sft+0.25
			if moved := a.colorOf(0.3) != c.colorOf(0.3) || a.colorOf(0.7) != c.colorOf(0.7); moved == (a.keyFor() == c.keyFor()) {
				t.Fatalf("cols %d: colors moved %v, key moved %v", cols, moved, a.keyFor() != c.keyFor())
			}
		}
	}
}
