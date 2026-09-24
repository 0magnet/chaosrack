//go:build js && wasm

package attractor

import (
	"math"
	"testing"
)

// A pair of identical channels is perfectly correlated; inverting one is
// the out-of-phase case the goniometer is kept for.
func TestCorrelationEnds(t *testing.T) {
	n := 400
	l := make([]float32, n)
	r := make([]float32, n)
	for i := range l {
		v := float32(math.Sin(float64(i) * 0.1))
		l[i], r[i] = v, v
	}
	out := make([]float32, 4)
	shortTimeCorrelation(l, r, out)
	for i, v := range out {
		if v < 0.95 {
			t.Errorf("in phase slot %d = %v, want near 1", i, v)
		}
	}
	for i := range r {
		r[i] = -l[i]
	}
	shortTimeCorrelation(l, r, out)
	for i, v := range out {
		if v > 0.05 {
			t.Errorf("out of phase slot %d = %v, want near 0", i, v)
		}
	}
	// Silence is neither: the middle, not the bottom.
	shortTimeCorrelation(make([]float32, n), make([]float32, n), out)
	for i, v := range out {
		if v != 0.5 {
			t.Errorf("silent slot %d = %v, want 0.5", i, v)
		}
	}
}

// Balance and position both read the stereo field; centered must land in
// the middle of the ramp and the two sides at opposite ends.
func TestBalanceAndPositionEnds(t *testing.T) {
	n := 200
	out := make([]float32, 2)
	mk := func(lv, rv float32) ([]float32, []float32) {
		l, r := make([]float32, n), make([]float32, n)
		for i := range l {
			s := float32(1)
			if i%2 == 1 {
				s = -1
			}
			l[i], r[i] = lv*s, rv*s
		}
		return l, r
	}

	l, r := mk(1, 1)
	shortTimeBalance(l, r, out)
	if out[0] < 0.45 || out[0] > 0.55 {
		t.Errorf("centered balance = %v, want 0.5", out[0])
	}
	shortTimePosition(l, r, out)
	if out[0] < 0.45 || out[0] > 0.55 {
		t.Errorf("centered position = %v, want 0.5", out[0])
	}

	l, r = mk(1, 0)
	shortTimeBalance(l, r, out)
	if out[0] < 0.95 {
		t.Errorf("hard left balance = %v, want near 1", out[0])
	}
	l, r = mk(0, 1)
	shortTimeBalance(l, r, out)
	if out[0] > 0.05 {
		t.Errorf("hard right balance = %v, want near 0", out[0])
	}
}

// dB is absolute: a known RMS has to land at a known place on the ramp,
// which is the whole reason for having it beside the linear level.
func TestDBIsAbsolute(t *testing.T) {
	n := 400
	w := make([]float32, n)
	// A square wave at 0.1 is RMS 0.1, which is −20 dB.
	for i := range w {
		if i%2 == 0 {
			w[i] = 0.1
		} else {
			w[i] = -0.1
		}
	}
	out := make([]float32, 2)
	shortTimeDB(w, out)
	want := float32((-20.0 - dbFloor) / -dbFloor) // ≈ 0.667
	if math.Abs(float64(out[0]-want)) > 0.02 {
		t.Errorf("−20 dB landed at %v, want about %v", out[0], want)
	}
	shortTimeDB(make([]float32, n), out)
	if out[0] != 0 {
		t.Errorf("silence = %v, want 0", out[0])
	}
}

// The classification drives whether a source is auto-ranged, so it has to
// agree with the sources that actually have a fixed scale.
func TestSourceClassification(t *testing.T) {
	for _, s := range []int{gradientSourceCorr, gradientSourceBalance,
		gradientSourcePosition, gradientSourceDB, gradientSourcePitch} {
		if !gradientSourceIsAbsolute(s) {
			t.Errorf("source %d should be absolute", s)
		}
	}
	for _, s := range []int{gradientSourceAudio, gradientSourceLevel,
		gradientSourceSide, gradientSourceFlux} {
		if gradientSourceIsAbsolute(s) {
			t.Errorf("source %d should be auto-ranged", s)
		}
	}
	// OFF sits inside the numeric range and is not a source.
	if gradientSourceIsAudio(GradientSourceOff) {
		t.Error("OFF was treated as an audio source")
	}
	for _, s := range []int{0, 1, 2, 3} {
		if gradientSourceIsAudio(s) {
			t.Errorf("coordinate source %d was treated as audio", s)
		}
	}
	for s := gradientSourceAudio; s <= gradientSourceMax; s++ {
		if s == GradientSourceOff {
			continue
		}
		if !gradientSourceIsAudio(s) {
			t.Errorf("source %d should be audio-fed", s)
		}
	}
}

// Held means held: the same input must map the same way twice, even after
// the window's own extremes move.
func TestRangeLockFreezesTheScale(t *testing.T) {
	saved := colorRangeLock
	savedLo, savedHi := acolor.lo, acolor.hi
	defer func() {
		colorRangeLock = saved
		acolor.lo, acolor.hi = savedLo, savedHi
	}()

	colorRangeLock = false
	quiet := []float32{0, 0.1, 0.2}
	acolor.stretchAudioColorLUT(quiet)
	lo, hi := acolor.lo, acolor.hi

	colorRangeLock = true
	loud := []float32{0, 5, 10}
	acolor.stretchAudioColorLUT(loud)
	if acolor.lo != lo || acolor.hi != hi {
		t.Errorf("a locked range moved: %v..%v became %v..%v", lo, hi, acolor.lo, acolor.hi)
	}
	// And it clamps rather than rescaling.
	for i, v := range loud {
		if v < 0 || v > 1 {
			t.Errorf("slot %d = %v, outside 0..1", i, v)
		}
	}
}

// The src dial binds a label to an option BY INDEX, so the ring must have
// one label per option in option order. It was left at six while seven
// sources were added to the select, and the new ones were then unreachable
// from the knob — permalink only.
//
// The option list lives in the panel HTML and the ring in main.go, so this
// counts what each should hold and pins them together.
func TestSrcRingCoversEverySource(t *testing.T) {
	// Every source the code knows about, which is what the select carries:
	// the four coordinate sources, OFF, and every audio-fed one.
	want := 5 // X, Y, Z, trail, off
	for s := gradientSourceAudio; s <= gradientSourceMax; s++ {
		if s == GradientSourceOff {
			continue
		}
		want++
	}
	if got := len(gradSrcRingLabels); got != want {
		t.Errorf("the src ring has %d labels for %d sources; a label per option "+
			"in option order is what index binding requires", got, want)
	}
	for i, l := range gradSrcRingLabels {
		if l == "" {
			t.Errorf("ring label %d is empty", i)
		}
		if len(l) > 3 {
			t.Errorf("ring label %q is %d characters; the ring fits three", l, len(l))
		}
	}
}
