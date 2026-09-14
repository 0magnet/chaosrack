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
	ow, oh := width, height
	t.Cleanup(func() { width, height = ow, oh })
	width, height = w, h
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
