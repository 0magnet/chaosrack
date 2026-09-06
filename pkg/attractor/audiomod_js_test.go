//go:build js && wasm

package attractor

import (
	"math"
	"math/rand"
	"testing"
)

// These drive applyAudioModulation itself, with the real eqModValue and the
// real paramDefs, and touch neither the DOM nor GL — the modulation path is
// arithmetic over package globals, and the only reason the file is tagged is
// that paramDef and paramMods are. Node is enough to run them (make test-wasm).
//
// Each one leaves the globals it moved as it found them: they are the same
// variables the rest of the suite and the running program read.

// The route every test in this file uses: a steady mono signal near full scale
// at a depth that swings sphere latitude by about a fifth of its range.
// Constants rather than arguments because all of them want the same route —
// what they differ in is what they then ask of the result.
const (
	testModLevel  = 0.2
	testModEnergy = 0.8
)

// routeMono points a parameter at that signal and returns the undo. bands stays
// nil so eqModValue takes its "loudest band" path, which is what an untouched
// EQ strip means.
func routeMono(t *testing.T, id string) func() {
	t.Helper()
	prevMods, hadMod := paramMods[id]
	prevBand := afBand["mono"]
	prevOn := audioMod
	prevHold, hadHold := modHold[id]

	band := make([]float32, numEQBands)
	for i := range band {
		band[i] = testModEnergy
	}
	afBand["mono"] = band
	paramMods[id] = paramMod{channel: "mono", level: testModLevel}
	audioMod = true
	delete(modHold, id)
	modAppliedPrev = modAppliedPrev[:0]

	return func() {
		audioMod = prevOn
		afBand["mono"] = prevBand
		if hadMod {
			paramMods[id] = prevMods
		} else {
			delete(paramMods, id)
		}
		if hadHold {
			modHold[id] = prevHold
		} else {
			delete(modHold, id)
		}
		modAppliedPrev = modAppliedPrev[:0]
	}
}

// paramByID finds a mode's paramDef so a test can read its range without
// restating it — the ranges move, and a test that hard-codes them tests the
// copy rather than the parameter.
func paramByID(t *testing.T, mode, id string) paramDef {
	t.Helper()
	for _, pd := range attractorParams[mode] {
		if pd.ID == id {
			return pd
		}
	}
	t.Fatalf("no parameter %q in mode %q", id, mode)
	return paramDef{}
}

// The feature this branch exists for. A latitude count routed from audio has to
// MOVE — the old code skipped integer parameters outright, so the answer was
// always "the value it already had" — and it has to land on a whole number,
// because half a latitude line cannot be drawn.
func TestCountParameterIsModulatedInWholeSteps(t *testing.T) {
	const mode, id = "sphere", "sphere-stacks"
	pd := paramByID(t, mode, id)
	defer routeMono(t, id)()

	base := *pd.Value
	saved := applyAudioModulation(mode)
	got := *pd.Value
	restoreAudioModulation(saved)

	if got == base {
		t.Fatalf("a routed count did not move: still %v", got)
	}
	if float32(math.Trunc(float64(got))) != got {
		t.Errorf("modulated latitude count came out as %v; a fractional line cannot be drawn", got)
	}
	if got < pd.Min || got > pd.Max {
		t.Errorf("modulated count %v is outside the parameter's own range %v..%v", got, pd.Min, pd.Max)
	}
	if *pd.Value != base {
		t.Errorf("after restore the base is %v, want %v — the slider must not drift", *pd.Value, base)
	}
}

// Restoration is the same requirement the float path has, and it is worth its
// own test because the quantized value is a DIFFERENT number from the base
// rather than a nudge away from it: a count left written back would ratchet the
// slider onto the grid and stay there, which reads as a knob that moves itself
// whenever the music plays. Run over many frames because one restore that works
// proves nothing about the accumulation.
func TestModulatingACountNeverDriftsTheSlider(t *testing.T) {
	const mode, id = "sphere", "sphere-stacks"
	pd := paramByID(t, mode, id)
	defer routeMono(t, id)()

	base := *pd.Value
	for i := 0; i < 240; i++ {
		restoreAudioModulation(applyAudioModulation(mode))
	}
	if *pd.Value != base {
		t.Errorf("after 240 modulated frames the base is %v, want %v", *pd.Value, base)
	}
}

// The rebuild cost, which is the real hazard in letting audio reach a line
// count: a static model only rebuilds its mesh when staticGeomDirty is set, and
// setting it every frame is what staticGeomCached's comment records as 45% of
// all allocation and a 66-100ms collector pause every 400ms. Quantizing means
// most frames produce the same integer, and a frame that produces the same
// integer must not ask for the mesh again.
func TestGeometryRebuildsOnlyWhenTheCountActuallyChanges(t *testing.T) {
	const mode, id = "sphere", "sphere-stacks"
	defer routeMono(t, id)()

	// First frame: the count moves off its base, so the mesh is stale.
	staticGeomDirty = false
	restoreAudioModulation(applyAudioModulation(mode))
	if !staticGeomDirty {
		t.Fatal("the frame that first modulated the count left the mesh unmarked")
	}
	// Every frame after it, with the signal held constant, lands on the same
	// integer and must leave the flag alone.
	for i := 0; i < 120; i++ {
		staticGeomDirty = false
		restoreAudioModulation(applyAudioModulation(mode))
		if staticGeomDirty {
			t.Fatalf("frame %d rebuilt the mesh for an unchanged count", i)
		}
	}
	// Switching the route off is itself a change: the mesh on the GPU is the
	// modulated one and nothing else will mark it stale.
	staticGeomDirty = false
	audioMod = false
	restoreAudioModulation(applyAudioModulation(mode))
	if !staticGeomDirty {
		t.Error("switching modulation off left the modulated mesh on the GPU unmarked")
	}
}

// An attractor regenerates its trail every frame and never consults the flag,
// so nothing in the count path may set it there — a mode that does not use
// staticGeomDirty setting it would make the NEXT static mode upload on a frame
// it did not need to.
func TestAttractorModesDoNotTouchTheGeometryFlag(t *testing.T) {
	const mode, id = "stereo", "stereo-tau"
	if _, ok := attractorParams[mode]; !ok {
		t.Skipf("mode %q is not in this build", mode)
	}
	if isAttractorMode(mode) {
		defer routeMono(t, id)()
		staticGeomDirty = false
		restoreAudioModulation(applyAudioModulation(mode))
		if staticGeomDirty {
			t.Errorf("modulating %s in attractor mode %q dirtied the static geometry", id, mode)
		}
	}
}

// The measurement the deadband was chosen from, kept in the tree so the choice
// can be re-examined rather than believed. It runs the real signal path —
// afSmooth's 0.6 attack / 0.12 release, the same arithmetic
// applyAudioModulation does — for ten seconds at 60 fps and counts how often
// the quantized value moves, which is exactly how often the mesh is rebuilt.
//
// The interesting configuration is a STEADY tone with room jitter: nothing
// musical is happening, so a count that keeps changing is pure chatter. The
// numbers on the machine this was written on, sphere latitude (4..100 by 1) at
// depth 0.05:
//
//	no quantizing at all   600 rebuilds / 600 frames
//	nearest rounding       127
//	nearest + deadband       2
//
// The per-frame strobe that was the obvious worry — 10, 11, 10, 11 on
// successive frames — barely appears at all: immediate reversals ran 0..7 per
// 600 frames whether or not there was a deadband, because afSmooth has already
// low-passed the feature. The chatter is at the rate of the room, not the
// frame, and that is what the deadband is for.
func TestQuantizedCountsDoNotChatterOnASteadyTone(t *testing.T) {
	const (
		frames                 = 600 // ten seconds at 60 fps
		min, max, step float32 = 4, 100, 1
		base, level    float32 = 30, 0.05
	)
	// A FIXED seed: a measurement that changed run to run could not be compared
	// against the numbers in the comment above, and nothing here is a secret.
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // deterministic test signal, not cryptography
	var f, held float32
	var nearest, deadband, prevNearest, prevDeadband float32
	var nearestChanges, deadbandChanges int
	for i := 0; i < frames; i++ {
		// A steady tone at half scale, plus the jitter any real room puts on a
		// normalized band energy.
		raw := clamp01(float32(0.5 + rng.NormFloat64()*0.03))
		f = afSmooth(f, raw)
		v := clampF(base+level*f*(max-min), min, max)

		nearest = snapToStep(v, min, max, step)
		held = quantizeHeld(v, held, i > 0, min, max, step)
		deadband = held

		if i > 0 {
			if nearest != prevNearest {
				nearestChanges++
			}
			if deadband != prevDeadband {
				deadbandChanges++
			}
		}
		prevNearest, prevDeadband = nearest, deadband
	}
	t.Logf("steady tone, %d frames: nearest rounding rebuilt %d times, the deadband %d",
		frames, nearestChanges, deadbandChanges)
	// Nearest rounding really does chatter here — if it stopped doing so the
	// deadband would be dead weight and this test should be the thing that
	// says so, rather than passing quietly on a signal that no longer probes
	// the boundary.
	if nearestChanges < 20 {
		t.Fatalf("nearest rounding changed the count only %d times in %d frames; "+
			"the case this measures no longer straddles a boundary and needs rebuilding",
			nearestChanges, frames)
	}
	if deadbandChanges*8 > nearestChanges {
		t.Errorf("the deadband cut chatter from %d changes to %d in %d frames; "+
			"it was chosen for roughly a sixtyfold cut and anything under eightfold is not worth its state",
			nearestChanges, deadbandChanges, frames)
	}
}

// The other half of the same choice: a deadband that also swallowed real
// modulation would be a knob that has stopped working rather than one that has
// stopped flickering. A depth that genuinely sweeps the count across most of
// its range must still move it on the great majority of the frames plain
// rounding would.
func TestTheDeadbandKeepsModulationThatIsReallyThere(t *testing.T) {
	const (
		frames                 = 600
		min, max, step float32 = 4, 100, 1
		base, level    float32 = 30, 0.5
	)
	// A FIXED seed: a measurement that changed run to run could not be compared
	// against the numbers in the comment above, and nothing here is a secret.
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // deterministic test signal, not cryptography
	var f, held float32
	var prevNearest, prevDeadband float32
	var nearestChanges, deadbandChanges int
	for i := 0; i < frames; i++ {
		// A 2 Hz beat: the signal genuinely traverses many whole counts.
		t := float64(i) / 60
		raw := clamp01(float32(0.5 + 0.45*math.Sin(2*math.Pi*2*t) + rng.NormFloat64()*0.03))
		f = afSmooth(f, raw)
		v := clampF(base+level*f*(max-min), min, max)

		nearest := snapToStep(v, min, max, step)
		held = quantizeHeld(v, held, i > 0, min, max, step)
		if i > 0 {
			if nearest != prevNearest {
				nearestChanges++
			}
			if held != prevDeadband {
				deadbandChanges++
			}
		}
		prevNearest, prevDeadband = nearest, held
	}
	t.Logf("2 Hz beat at full depth, %d frames: nearest rounding moved the count %d times, the deadband %d",
		frames, nearestChanges, deadbandChanges)
	if deadbandChanges*10 < nearestChanges*8 {
		t.Errorf("under real modulation the deadband kept only %d of %d changes; "+
			"below 80%% it is damping the music rather than the boundary",
			deadbandChanges, nearestChanges)
	}
}
