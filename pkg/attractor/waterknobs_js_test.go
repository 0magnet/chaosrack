//go:build js && wasm

package attractor

import (
	"syscall/js"
	"testing"
)

// The waterfall's two surfaces want LINE, STEP and FFT an order of magnitude
// apart, so the SRC switch carries them — and the whole difficulty is knowing
// when NOT to.

// resetWfallKnobs puts the three back where the package starts them.
func resetWfallKnobs(t *testing.T) {
	t.Helper()
	l, s, f, a, u := wfallLinesF, wfallStepF, wfallFFTF, wfallAutoSet, wfallUserSet
	t.Cleanup(func() {
		wfallLinesF, wfallStepF, wfallFFTF, wfallAutoSet, wfallUserSet = l, s, f, a, u
	})
	wfallLinesF, wfallStepF, wfallFFTF = 16, 5, 1
	wfallAutoSet = wfallDecayDefaults
	wfallUserSet = struct{ lines, step, fft bool }{}
	// No DOM: setWfallKnob writes the variable and then gives up on the element,
	// which is the half this test is about.
	withFakeDoc(t, map[string]js.Value{})
}

func TestWaterfallDefaultsFollowTheSource(t *testing.T) {
	resetWfallKnobs(t)
	wfallApplyDefaults(wfallLiveDefaults)
	if wfallLinesF != 32 || wfallStepF != 40 || wfallFFTF != 2 {
		t.Fatalf("live: line=%v step=%v fft=%v, want 32/40/2", wfallLinesF, wfallStepF, wfallFFTF)
	}
	wfallApplyDefaults(wfallDecayDefaults)
	if wfallLinesF != 16 || wfallStepF != 5 || wfallFFTF != 1 {
		t.Fatalf("decay: line=%v step=%v fft=%v, want 16/5/1", wfallLinesF, wfallStepF, wfallFFTF)
	}
}

// TestWaterfallDefaultsDeferForGood is the regression.
//
// A hand-set line count survived one switch and was reclaimed on the way back,
// because deferring recorded the value it had deferred TO as though the switch
// had written it — which makes a chosen value indistinguishable from an
// automatic one the next time round. Turning a knob has to take it out of the
// automation permanently, not for one switch.
func TestWaterfallDefaultsDeferForGood(t *testing.T) {
	resetWfallKnobs(t)
	wfallApplyDefaults(wfallLiveDefaults)
	wfallLinesF = 48 // as if somebody turned it
	for i, d := range []wfallDefaults{wfallDecayDefaults, wfallLiveDefaults, wfallDecayDefaults} {
		wfallApplyDefaults(d)
		if wfallLinesF != 48 {
			t.Fatalf("switch %d: line=%v, want the chosen 48", i, wfallLinesF)
		}
		// The two nobody touched still follow.
		if wfallStepF != d.step || wfallFFTF != d.fft {
			t.Errorf("switch %d: step=%v fft=%v, want %v/%v", i, wfallStepF, wfallFFTF, d.step, d.fft)
		}
	}
}

// TestWaterfallKnobsClamp checks the accessors, because every one of these is
// an audio-modulation target and a modulator drives a knob past its own ends:
// a line count of zero or a transform index off the end of the table is a
// crash rather than a wrong picture.
func TestWaterfallKnobsClamp(t *testing.T) {
	resetWfallKnobs(t)
	for _, v := range []float32{-1e9, -1, 0, 3, 16, 64, 1e9} {
		wfallLinesF = v
		if n := wfallLines(); n < 4 || n > 64 {
			t.Errorf("LINE %v gave %d slices", v, n)
		}
		wfallStepF = v
		if ms := wfallStepMS(); ms < 1 || ms > 100 {
			t.Errorf("STEP %v gave %v ms", v, ms)
		}
		wfallFFTF = v
		n := wfallFFTLen()
		if n&(n-1) != 0 || n < 1024 || n > 8192 {
			t.Errorf("FFT %v gave a %d-point transform", v, n)
		}
		wfallChanF = v
		if c := wfallChan(); int(c) < 0 || int(c) >= len(tapChanNames) {
			t.Errorf("CHAN %v gave channel %d", v, c)
		}
	}
}
