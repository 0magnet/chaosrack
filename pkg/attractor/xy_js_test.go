//go:build js && wasm

package attractor

import (
	"math"
	"testing"
)

// setCanvas sets the globals xyDeflection reads and puts them back afterwards,
// so a size used for one case cannot leak into a later test in the package.
func setCanvas(t *testing.T, w, h int) {
	t.Helper()
	ow, oh := gpu.width, gpu.height
	t.Cleanup(func() { gpu.width, gpu.height = ow, oh })
	gpu.width, gpu.height = w, h
}

// The goniometer has to be SQUARE. The vertex shader writes gl_Position
// straight from the sample pair, so without a correction one unit of L spans
// the canvas width and one unit of R spans its height — and on any ordinary
// window those are not the same number of pixels. Every reading the display
// exists to give is an angle or an eccentricity, and both are wrong under an
// anisotropic scale.
func TestXYDeflectionIsSquareOnEveryAspect(t *testing.T) {
	cases := []struct{ w, h int }{
		{1916, 998},  // the window this was found in
		{998, 1916},  // portrait
		{1000, 1000}, // square
		{3840, 1080}, // an ultrawide, where the error is worst
	}
	for _, c := range cases {
		setCanvas(t, c.w, c.h)
		sx, sy := xyDeflection()
		// Equal PIXELS per unit on both axes is the property: sx is a fraction
		// of the half-width and sy a fraction of the half-height.
		px := float64(sx) * float64(c.w) / 2
		py := float64(sy) * float64(c.h) / 2
		if math.Abs(px-py) > 0.5 {
			t.Errorf("%dx%d: %.1f px per unit across, %.1f up — the scope is not square",
				c.w, c.h, px, py)
		}
	}
}

// Squaring must not crop: the full ±1 range has to stay on screen in both
// directions, so the shorter side is the one that keeps xyScale.
func TestXYDeflectionNeverLeavesTheViewport(t *testing.T) {
	for _, c := range []struct{ w, h int }{{1916, 998}, {998, 1916}, {1000, 1000}} {
		setCanvas(t, c.w, c.h)
		sx, sy := xyDeflection()
		if sx > xyScale || sy > xyScale {
			t.Errorf("%dx%d: deflection (%.4f, %.4f) exceeds xyScale %.4f", c.w, c.h, sx, sy, xyScale)
		}
		if max(sx, sy) != xyScale {
			t.Errorf("%dx%d: neither axis uses the full %.4f — the figure is smaller than it need be",
				c.w, c.h, xyScale)
		}
	}
}

// A zero-sized canvas happens for a frame during startup and on a hidden tab.
// Dividing by it would put a NaN in every vertex, which GL reports to nobody.
func TestXYDeflectionSurvivesAnUnmeasuredCanvas(t *testing.T) {
	for _, c := range []struct{ w, h int }{{0, 0}, {0, 998}, {1916, 0}, {-1, -1}} {
		setCanvas(t, c.w, c.h)
		sx, sy := xyDeflection()
		if math.IsNaN(float64(sx)) || math.IsNaN(float64(sy)) || sx <= 0 || sy <= 0 {
			t.Errorf("%dx%d: deflection (%v, %v)", c.w, c.h, sx, sy)
		}
	}
}

// The CRT beam shortens the drawn arc by shrinking the vertex budget, which is
// a length on an integrated model and a RESOLUTION on the three audio
// embeddings. Applied there it silently decimates the window instead of
// shortening it, so those modes have to be out.
func TestCRTBeamSkipsTheAudioEmbeddings(t *testing.T) {
	for _, m := range []string{"takens", "stereo", "polar"} {
		if !isAudioEmbedding(m) {
			t.Errorf("%s should count as an audio embedding", m)
		}
		if !isAttractorMode(m) {
			t.Errorf("%s stopped being an attractor mode — this test is then checking nothing", m)
		}
	}
	for _, m := range []string{"lorenz", "rossler", "henon", "turtle", "lissajou"} {
		if isAudioEmbedding(m) {
			t.Errorf("%s is not an audio embedding", m)
		}
	}
}

// The camera fit on the audio embeddings is keyed to GAIN, because the bound it
// frames is computed from GAIN. That has to survive the audio modulator, which
// writes its value straight into the parameter for the duration of a step: a
// GAIN with a modulator on it "changes" almost every frame, and re-fitting to
// that would put the camera back under the music — the one thing takens_js.go's
// comments exist to prevent.
func TestModulatedParametersDoNotCountAsKnobTurns(t *testing.T) {
	oldMod, oldMods := audioMod, paramMods
	t.Cleanup(func() { audioMod, paramMods = oldMod, oldMods })

	paramMods = map[string]paramMod{
		"takens-gain": {channel: "mono", level: 0.5},
		"stereo-gain": {channel: "L", level: 0},  // routed, but at zero depth
		"polar-gain":  {channel: "", level: 0.5}, // depth, but no channel
	}

	audioMod = false
	for _, id := range []string{"takens-gain", "stereo-gain", "polar-gain", "nonesuch"} {
		if paramIsModulated(id) {
			t.Errorf("%s reads as modulated with Audio mod switched off", id)
		}
	}

	audioMod = true
	if !paramIsModulated("takens-gain") {
		t.Error("a parameter with a channel and a level is modulated")
	}
	for _, id := range []string{"stereo-gain", "polar-gain", "nonesuch"} {
		if paramIsModulated(id) {
			t.Errorf("%s has nothing driving it and must not read as modulated", id)
		}
	}
}

// ── The controls this mode did not have ──────────────────────────────────

// The window is a SNAPSHOT: TimeDomainStereo fills it from the source's ring,
// and asking for more than the ring holds returns wrapped, already-overwritten
// samples rather than failing — a plausible buffer of the wrong audio, which
// nothing downstream can detect. The knob's ceiling and this clamp are what
// keep that unreachable at every sample rate.
func TestXYWindowNeverOutrunsTheSnapshotRing(t *testing.T) {
	for _, sr := range []int{8000, 24000, 44100, 48000, 96000, 192000} {
		got := xyWindowSamples(xyWinMax, sr)
		if got > xySpanMax {
			t.Errorf("at %d Hz the longest window is %d samples, past the %d the ring holds",
				sr, got, xySpanMax)
		}
		if got < 64 {
			t.Errorf("at %d Hz the longest window came back as %d samples", sr, got)
		}
	}
}

// A window shorter than a couple of cycles of anything is not a display, and a
// zero-length one is a buffer nobody can index. The floor holds whatever the
// knob and the rate conspire to ask for.
func TestXYWindowHasAFloor(t *testing.T) {
	for _, c := range []struct {
		ms float32
		sr int
	}{{0, 48000}, {-5, 48000}, {0.001, 48000}, {43, 0}, {43, -1}} {
		if got := xyWindowSamples(c.ms, c.sr); got < 64 {
			t.Errorf("xyWindowSamples(%v, %d) = %d", c.ms, c.sr, got)
		}
	}
}

// WIN and LAG are durations, so one setting is one amount of time on every
// source — takens.TauSamples' argument, applied to the scope's own two.
func TestXYWindowAndLagAreDurations(t *testing.T) {
	for _, ms := range []float32{5, 43, 100} {
		for _, sr := range []int{24000, 48000} {
			if got := float32(xyWindowSamples(ms, sr)) / float32(sr) * 1000; got < ms*0.98 || got > ms*1.02 {
				t.Errorf("a %v ms window is %.2f ms at %d Hz", ms, got, sr)
			}
			if got := float32(xyLagSamples(ms, sr)) / float32(sr) * 1000; got < ms*0.98 || got > ms*1.02 {
				t.Errorf("a %v ms lag is %.2f ms at %d Hz", ms, got, sr)
			}
		}
	}
}

// A zero lag is the raw mono diagonal the lag exists to avoid, so it can never
// be the answer however the knob is driven.
func TestXYLagIsNeverZero(t *testing.T) {
	for _, c := range []struct {
		ms float32
		sr int
	}{{0, 48000}, {-1, 48000}, {0.0001, 48000}, {2.67, 0}} {
		if got := xyLagSamples(c.ms, c.sr); got < 1 {
			t.Errorf("xyLagSamples(%v, %d) = %d", c.ms, c.sr, got)
		}
	}
}

// Every knob is a routable modulation destination, so each selector has to
// survive an out-of-range float or a NaN — the value arriving is not
// necessarily one the dial can be at. (stereoAxisSel's argument, and its trap:
// the range is checked BEFORE the conversion, because a float-to-int
// conversion whose value does not fit is implementation-defined in Go.)
func TestXYSelectorsClampWhateverModulationDoes(t *testing.T) {
	oldS, oldB, oldP := xySmoothF, xyBasisF, xyPersist
	t.Cleanup(func() { xySmoothF, xyBasisF, xyPersist = oldS, oldB, oldP })

	inf := float32(1)
	for i := 0; i < 40; i++ {
		inf *= 1e10
	}
	nan := inf - inf

	for _, v := range []float32{-1e9, -1, 0, 0.5, 1, 4, 16, 1e9, inf, -inf, nan} {
		xySmoothF = v
		if got := xySmoothSel(); got < 1 || got > 16 {
			t.Errorf("xySmoothSel() = %d for %v", got, v)
		}
		xyPersist = v
		if got := xyPersistK(); got < 0 || got > 0.98 {
			t.Errorf("xyPersistK() = %v for %v", got, v)
		}
		xyBasisF = v
		_ = xyIsMidSide() // a bool cannot be out of range; this is here to catch a panic
	}
}

// The retention may never reach 1. At exactly 1 the frame never decays, so the
// display fills in and stays filled — a trace that cannot be erased is not a
// long persistence, it is a stuck picture with no way out but leaving the mode.
func TestXYPersistAlwaysDecays(t *testing.T) {
	old := xyPersist
	t.Cleanup(func() { xyPersist = old })
	for _, v := range []float32{0.98, 1, 2, 1e9} {
		xyPersist = v
		if got := xyPersistK(); got >= 1 {
			t.Errorf("a persist knob at %v gives a retention of %v, which never fades", v, got)
		}
	}
	xyPersist = 0
	if xyPersistK() != 0 {
		t.Error("persist at zero must mean a plain clear, as the mode always did")
	}
}

// The basis knob's names and its dial ring have to be the same length: a ring
// that does not match its options is discarded whole by buildParamUnit, which
// then falls back to the full names and draws the knob over the top of them.
func TestXYBasisRingMatchesItsNames(t *testing.T) {
	if len(xyBasisNames) != len(xyBasisRing) {
		t.Fatalf("%d names, %d ring labels", len(xyBasisNames), len(xyBasisRing))
	}
	var p paramDef
	for _, d := range attractorParams["xy"] {
		if d.ID == "xy-basis" {
			p = d
		}
	}
	if p.ID == "" {
		t.Fatal("the xy mode has no basis row")
	}
	if int(p.Max)+1 != len(xyBasisNames) {
		t.Errorf("the basis knob runs 0..%v but there are %d names", p.Max, len(xyBasisNames))
	}
}

// The mode had no parameters at all, and the Parameters module did not appear
// for it. That it has them now is the feature, so it is worth a test that says
// which — a knob quietly lost in a refactor is a control that stops existing.
func TestXYHasItsControls(t *testing.T) {
	want := []string{"xy-basis", "xy-gain", "xy-win", "xy-persist", "xy-lag", "xy-smooth"}
	got := map[string]bool{}
	for _, p := range attractorParams["xy"] {
		got[p.ID] = true
	}
	for _, id := range want {
		if !got[id] {
			t.Errorf("the xy scope has lost its %s knob", id)
		}
	}
}
