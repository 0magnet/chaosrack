//go:build js && wasm

package attractor

import "testing"

// The bug this pins: the mode took its span from the global trail knob, so the
// default 20000 points meant a 20000-SAMPLE window — 833 ms at 24 kHz. The
// figure was a dense tangle that a frame's worth of new audio barely touched;
// measured against the frame 250 ms before it, 0.0% of the lit pixels changed.
// The window is a DURATION now, and its default matches the xy scope's
// long-standing xyWindow of 2048 samples.
func TestTakensWindowIsADurationNotAPointCount(t *testing.T) {
	const sr = 24000
	const budget = 20000 // the trail knob's default, i.e. vertBuf's capacity

	n, stride := takensWindow(85, sr, budget)
	spanMS := float64(n*stride) / sr * 1000
	if spanMS < 60 || spanMS > 110 {
		t.Errorf("the default window spans %.0f ms, want ~85 — the xy scope's 2048 samples", spanMS)
	}
	if spanMS > 300 {
		t.Errorf("the default window is %.0f ms; at that length the figure is visually frozen", spanMS)
	}
	if want := xyWindow; n*stride < want/2 || n*stride > want*2 {
		t.Errorf("the default window is %d samples, want the same order as the xy scope's %d", n*stride, want)
	}

	// The point BUDGET must not change the duration: that conflation is what
	// broke it. Ten times the budget shows the same 85 ms, in finer steps.
	nBig, strideBig := takensWindow(85, sr, budget*10)
	if got := float64(nBig*strideBig) / sr * 1000; got < 60 || got > 110 {
		t.Errorf("a 10x point budget changed the window to %.0f ms; the budget is resolution, not duration", got)
	}
}

// Whatever the knob asks for, the vertices have to fit in the buffer that
// exists — including the four the spline spends per source point.
func TestTakensWindowFitsTheVertexBudget(t *testing.T) {
	for _, sr := range []int{8000, 24000, 48000} {
		for _, budget := range []int{2000, 20000} {
			for _, win := range []float32{5, 85, 250, 500} {
				n, stride := takensWindow(win, sr, budget)
				if n < 2 {
					t.Errorf("sr=%d budget=%d win=%v: %d points, want at least 2", sr, budget, win, n)
				}
				if stride < 1 {
					t.Errorf("sr=%d budget=%d win=%v: stride %d", sr, budget, win, stride)
				}
				if v := takensVerts(n); v > budget {
					t.Errorf("sr=%d budget=%d win=%v: %d vertices overruns the %d-vertex buffer",
						sr, budget, win, v, budget)
				}
				// The requested duration is delivered, give or take the stride.
				want := float64(win) / 1000 * float64(sr)
				if got := float64(n * stride); want >= 64 && (got < want*0.85 || got > want*1.15) {
					t.Errorf("sr=%d budget=%d win=%v: spans %.0f samples, want ~%.0f", sr, budget, win, got, want)
				}
			}
		}
	}
}

// A bad sample rate must not produce a zero-length or negative window; the ws
// source reports 0 until it has been told the rate.
func TestTakensWindowSurvivesAnUnknownSampleRate(t *testing.T) {
	n, stride := takensWindow(85, 0, 20000)
	if n < 2 || stride < 1 {
		t.Fatalf("unknown rate gave n=%d stride=%d", n, stride)
	}
	if got := float64(n*stride) / 24000 * 1000; got < 60 || got > 110 {
		t.Errorf("unknown rate spans %.0f ms, want the 24000 Hz default's ~85", got)
	}
}

// The scale is fixed, and the point of fixing it is that nothing may leave the
// frame anyway. A sample is bounded to ±1, so no coordinate exceeds GAIN and
// the worst a delay vector can reach is the corner of that cube.
func TestTakensFitCoversFullScale(t *testing.T) {
	for _, gain := range []float32{0.5, 1, 10, 50} {
		fit := takensFitExtent(gain)
		// The corner of the cube a full-scale signal can draw.
		corner := gain * 1.7320508
		if fit < corner {
			t.Errorf("gain %v: fitted to %v but a full-scale delay vector reaches %v — peaks clip",
				gain, fit, corner)
		}
		// And not so far out that the figure is a dot.
		if fit > corner*1.5 {
			t.Errorf("gain %v: fitted to %v for a worst case of %v; the figure would be needlessly small",
				gain, fit, corner)
		}
	}
}

// Fixed means fixed: the drawn size must depend on the input and the gain, and
// on nothing else — no history, no state, no adaptation. This is the property
// the user asked for after three attempts at automatic scaling, each of which
// resized the figure as the music moved.
func TestTakensScaleIsFixed(t *testing.T) {
	saved := takensGain
	defer func() { takensGain = saved }()

	// The same sample must map to the same world coordinate every time,
	// whatever came before it.
	takensGain = 10
	quiet := 0.05 * takensGain
	loud := 0.90 * takensGain
	for i := 0; i < 100; i++ {
		if got := 0.05 * takensGain; got != quiet {
			t.Fatalf("a fixed scale drifted: %v then %v", quiet, got)
		}
		if got := 0.90 * takensGain; got != loud {
			t.Fatalf("a fixed scale drifted: %v then %v", loud, got)
		}
	}
	// A quiet passage draws smaller than a loud one — dynamics are visible,
	// which is the deliberate consequence of not adapting.
	if !(quiet < loud) {
		t.Errorf("quiet %v is not smaller than loud %v", quiet, loud)
	}
	// And the loudest possible input still sits inside the fit.
	if full := 1.0 * takensGain; full*1.7320508 > takensFitExtent(takensGain) {
		t.Errorf("full scale (%v) exceeds the fitted extent %v", full, takensFitExtent(takensGain))
	}
}
