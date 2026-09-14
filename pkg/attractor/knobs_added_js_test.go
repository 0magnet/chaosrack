//go:build js && wasm

package attractor

import "testing"

// nanF and infF build the values a modulator riding a feature that has gone to
// zero or to infinity can deliver, without importing math into a test file that
// otherwise needs none.
func infF() float32 {
	v := float32(1)
	for i := 0; i < 40; i++ {
		v *= 1e10
	}
	return v
}
func nanF() float32 { return infF() - infF() }

// ── The tap's channel fold ───────────────────────────────────────────────

// mid carries the ½ and so does side, so that a full-scale input stays inside
// ±1 on every position. Every fixed-scale camera fit in this package is fitted
// to that bound, so a fold that could exceed it would put the figure off the
// screen on one dial position and nowhere else.
func TestTapFoldStaysInsideTheBound(t *testing.T) {
	for _, c := range []tapChan{tapMix, tapLeft, tapRight, tapMid, tapSide} {
		for _, p := range [][2]float32{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}, {1, 0}, {0, -1}} {
			if got := tapFold(c, p[0], p[1]); got > 1 || got < -1 {
				t.Errorf("fold %d of (%v, %v) = %v, outside ±1", c, p[0], p[1], got)
			}
		}
	}
}

// A mono source writes the same samples into both rings, so "side" of it is
// silence — and that is the true answer rather than a special case to dodge.
func TestTapSideOfAMonoSourceIsSilence(t *testing.T) {
	for _, v := range []float32{-1, -0.3, 0, 0.5, 1} {
		if got := tapFold(tapSide, v, v); got != 0 {
			t.Errorf("side of a mono sample %v = %v, want silence", v, got)
		}
	}
}

// mid and the mix are the same arithmetic under two names — the mix is what a
// display of "what is playing" wants and mid is what a stereo pair is called —
// so they must not drift apart.
func TestTapMidAndMixAgree(t *testing.T) {
	for _, p := range [][2]float32{{1, 1}, {0.3, -0.7}, {-1, 0.25}} {
		if a, b := tapFold(tapMix, p[0], p[1]), tapFold(tapMid, p[0], p[1]); a != b {
			t.Errorf("mix %v and mid %v disagree on (%v, %v)", a, b, p[0], p[1])
		}
	}
}

// Every knob is a routable modulation destination, so the selector has to
// survive whatever arrives — the range is checked BEFORE the conversion,
// because a float-to-int conversion whose value does not fit is
// implementation-defined in Go.
func TestTapChanSelClampsWhateverModulationDoes(t *testing.T) {
	last := tapChan(len(tapChanNames) - 1)
	for _, v := range []float32{-1e9, -1, 0, 0.4, 1, 2, 4, 4.6, 1e9, infF(), -infF(), nanF()} {
		if got := tapChanSel(v); got > last {
			t.Errorf("tapChanSel(%v) = %d, past the last position %d", v, got, last)
		}
	}
}

// The dial's names and the ring around it have to be the same length: a ring
// that does not match its options is discarded whole by buildParamUnit, which
// falls back to the full names and draws the knob over the top of them.
func TestTapChanRingMatchesItsNames(t *testing.T) {
	if len(tapChanNames) != len(tapChanRing) {
		t.Fatalf("%d names, %d ring labels", len(tapChanNames), len(tapChanRing))
	}
	for _, c := range []struct{ mode, id string }{
		{"takens", "takens-chan"}, {"polar", "polar-chan"},
	} {
		var p paramDef
		for _, d := range attractorParams[c.mode] {
			if d.ID == c.id {
				p = d
			}
		}
		if p.ID == "" {
			t.Errorf("%s has no %s row", c.mode, c.id)
			continue
		}
		if int(p.Max)+1 != len(tapChanNames) {
			t.Errorf("%s runs 0..%v but there are %d channels", c.id, p.Max, len(tapChanNames))
		}
	}
}

// ── The Stereo Embedding's alignment and width ───────────────────────────

// ALIGN is a duration, so the offset it dials out is the same amount of time on
// every source — the point of dialling it out at all is to READ how far apart
// the channels were.
func TestStereoAlignIsADuration(t *testing.T) {
	for _, knob := range []float32{-stereoAlignMax, -48, 24, stereoAlignMax} {
		wantMS := float64(tauMS(knob))
		for _, sr := range []int{24000, 44100, 48000, 96000} {
			got := float64(stereoAlignSamples(knob, sr)) / float64(sr) * 1000
			if tol := 1000.0 / float64(sr); got < wantMS-tol || got > wantMS+tol {
				t.Errorf("align %v at %d Hz is %.4f ms, want %.4f", knob, sr, got, wantMS)
			}
		}
	}
}

// Zero has to stay zero. tauSamples floors at one sample because a zero DELAY
// collapses an embedding; a zero OFFSET is the normal setting and must not
// quietly become a one-sample skew.
func TestStereoAlignZeroIsZero(t *testing.T) {
	for _, sr := range []int{24000, 48000} {
		if got := stereoAlignSamples(0, sr); got != 0 {
			t.Errorf("a zero offset became %d samples at %d Hz", got, sr)
		}
	}
}

// The snapshot bound is computed from the knob's reach, so a value past it —
// from a hand-edited permalink or a modulator — must not produce an index the
// buffer does not have.
func TestStereoAlignClampsToItsReach(t *testing.T) {
	for _, v := range []float32{-1e9, 1e9, infF(), -infF(), nanF()} {
		got := stereoAlignSamples(v, 48000)
		if got > stereoAlignMax || got < -stereoAlignMax {
			t.Errorf("stereoAlignSamples(%v) = %d, outside ±%d", v, got, stereoAlignMax)
		}
	}
}

// WIDTH at 1 is the identity — the figure is what was recorded — and at 0 the
// side content is gone, which is the mono sum and the thing the knob exists to
// let you hear the shape of.
func TestStereoWidthEndsBehaveAsMarked(t *testing.T) {
	for _, p := range [][2]float32{{0.8, -0.2}, {1, 1}, {-0.4, 0.9}} {
		l, r := stereoWiden(p[0], p[1], 1)
		if l != p[0] || r != p[1] {
			t.Errorf("width 1 changed (%v, %v) into (%v, %v)", p[0], p[1], l, r)
		}
		l, r = stereoWiden(p[0], p[1], 0)
		mid := (p[0] + p[1]) * 0.5
		if l != mid || r != mid {
			t.Errorf("width 0 of (%v, %v) = (%v, %v), want both at the mid %v", p[0], p[1], l, r, mid)
		}
	}
}

// Widening must leave MID alone: it scales the difference, and a version that
// also moved the sum would be a level control wearing the wrong label.
func TestStereoWidthLeavesMidAlone(t *testing.T) {
	for _, w := range []float32{0, 0.5, 1, 2, 3} {
		l, r := stereoWiden(0.6, -0.2, w)
		before := float32(0.6+-0.2) * 0.5
		const tol = float32(1e-6)
		if after := (l + r) * 0.5; after < before-tol || after > before+tol {
			t.Errorf("width %v moved mid from %v to %v", w, before, after)
		}
	}
}

// A NaN arriving from a modulator must leave the pair alone rather than erase
// the figure: a NaN coordinate in the vertex buffer is a hole GL reports to
// nobody.
func TestStereoWidthSurvivesRubbish(t *testing.T) {
	l, r := stereoWiden(0.5, -0.5, nanF())
	if l != 0.5 || r != -0.5 {
		t.Errorf("a NaN width gave (%v, %v)", l, r)
	}
	if l, r = stereoWiden(0.5, -0.5, -3); l != r {
		t.Errorf("a negative width gave (%v, %v); want it clamped to the mono sum", l, r)
	}
}

// ── The Polar mode's logarithmic radius ──────────────────────────────────

// THE WHOLE MODE RESTS ON THIS, and a new map is a new way to break it:
// polarFitExtent fits the camera to the sphere exactly, so a radius past 1
// draws outside the frame the fit reserved.
func TestPolarLogMapStaysInsideTheSphere(t *testing.T) {
	for _, drive := range []float32{0.2, 1, 2, 10, 1e6, infF(), nanF()} {
		for _, r := range []float32{0, 1e-9, 0.1, 0.5, 1, 1.7320508, 1e6, infF(), nanF()} {
			got := polarRadius(polarMapLog, r, drive)
			if got < 0 || got > 1 {
				t.Errorf("log map: r=%v drive=%v gave %v, outside [0,1]", r, drive, got)
			}
		}
	}
}

// A full-scale vector lands exactly on the surface, which is what makes the
// map a normalized decibel radius rather than an arbitrary curve.
func TestPolarLogMapReachesTheSurfaceAtFullScale(t *testing.T) {
	for _, drive := range []float32{0.5, 2, 10} {
		if got := polarRadius(polarMapLog, 1, drive); got < 0.999 || got > 1.001 {
			t.Errorf("drive %v: a full-scale vector draws at %v, not the surface", drive, got)
		}
	}
}

// It is the map for quiet material, so inside its window it has to draw quiet
// material LARGER than the linear-near-zero curves it sits beside — otherwise it
// is a fourth position that does nothing anybody wanted. Below the window it is
// the origin by design, which is what 10^-drive marks.
func TestPolarLogMapLiftsTheQuietEnd(t *testing.T) {
	const drive = 2 // two decades: a 40 dB window
	for _, r := range []float32{0.02, 0.05, 0.1, 0.3} {
		lg := polarRadius(polarMapLog, r, drive)
		th := polarRadius(polarMapTanh, r, drive)
		if lg <= th {
			t.Errorf("at r=%v the log map draws %v and tanh %v — it is not lifting the quiet end",
				r, lg, th)
		}
	}
}

// Every map's names and ring have to stay the same length as the map count, or
// the dial names a curve it does not draw.
func TestPolarMapTablesCoverTheNewMap(t *testing.T) {
	if len(polarMapNames) != polarMapCount || len(polarMapRing) != polarMapCount {
		t.Fatalf("%d maps, %d names, %d ring labels", polarMapCount, len(polarMapNames), len(polarMapRing))
	}
	for _, d := range attractorParams["polar"] {
		if d.ID == "polar-map" && int(d.Max)+1 != polarMapCount {
			t.Errorf("the map knob runs 0..%v but there are %d maps", d.Max, polarMapCount)
		}
	}
}

// DRIVE is a WINDOW in decades, so the bottom of it is the origin and every
// decade above it is an equal step outward. That equal-step property is the
// whole difference between this map and the two beside it.
func TestPolarLogMapIsADecibelScale(t *testing.T) {
	for _, drive := range []float32{1, 2, 4} {
		// A decade is exactly 1/drive of the radius, wherever it falls.
		want := 1 / drive
		for _, r := range []float32{1, 0.1, 0.01} {
			hi := polarRadius(polarMapLog, r, drive)
			lo := polarRadius(polarMapLog, r/10, drive)
			if lo <= 0 || hi <= 0 {
				continue // under the window, where the map is the origin by design
			}
			if d := hi - lo; d < want*0.99 || d > want*1.01 {
				t.Errorf("drive %v: a decade from %v steps %v of the radius, want %v", drive, r, d, want)
			}
		}
		// The bottom of the window is the origin.
		var floor float32 = 1
		for i := float32(0); i < drive; i++ {
			floor /= 10
		}
		if got := polarRadius(polarMapLog, floor, drive); got > 0.001 {
			t.Errorf("drive %v: the bottom of the window draws at %v, not the origin", drive, got)
		}
	}
}
