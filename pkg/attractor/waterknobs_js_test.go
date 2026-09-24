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
	l, s, f, a, u := wfall.linesF, wfall.stepF, wfall.fftF, wfall.autoSet, wfall.userSet
	t.Cleanup(func() {
		wfall.linesF, wfall.stepF, wfall.fftF, wfall.autoSet, wfall.userSet = l, s, f, a, u
	})
	wfall.linesF, wfall.stepF, wfall.fftF = 16, 5, 1
	wfall.autoSet = wfallDecayDefaults
	wfall.userSet = struct{ lines, step, fft bool }{}
	// No DOM: setWfallKnob writes the variable and then gives up on the element,
	// which is the half this test is about.
	withFakeDoc(t, map[string]js.Value{})
}

func TestWaterfallDefaultsFollowTheSource(t *testing.T) {
	resetWfallKnobs(t)
	wfall.applyDefaults(wfallLiveDefaults)
	if wfall.linesF != 32 || wfall.stepF != 40 || wfall.fftF != 2 {
		t.Fatalf("live: line=%v step=%v fft=%v, want 32/40/2", wfall.linesF, wfall.stepF, wfall.fftF)
	}
	wfall.applyDefaults(wfallDecayDefaults)
	if wfall.linesF != 16 || wfall.stepF != 5 || wfall.fftF != 1 {
		t.Fatalf("decay: line=%v step=%v fft=%v, want 16/5/1", wfall.linesF, wfall.stepF, wfall.fftF)
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
	wfall.applyDefaults(wfallLiveDefaults)
	wfall.linesF = 48 // as if somebody turned it
	for i, d := range []wfallDefaults{wfallDecayDefaults, wfallLiveDefaults, wfallDecayDefaults} {
		wfall.applyDefaults(d)
		if wfall.linesF != 48 {
			t.Fatalf("switch %d: line=%v, want the chosen 48", i, wfall.linesF)
		}
		// The two nobody touched still follow.
		if wfall.stepF != d.step || wfall.fftF != d.fft {
			t.Errorf("switch %d: step=%v fft=%v, want %v/%v", i, wfall.stepF, wfall.fftF, d.step, d.fft)
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
		wfall.linesF = v
		if n := wfall.lines(); n < 4 || n > 64 {
			t.Errorf("LINE %v gave %d slices", v, n)
		}
		wfall.stepF = v
		if ms := wfall.stepMS(); ms < 1 || ms > 100 {
			t.Errorf("STEP %v gave %v ms", v, ms)
		}
		wfall.fftF = v
		n := wfall.fftLen()
		if n&(n-1) != 0 || n < 1024 || n > 8192 {
			t.Errorf("FFT %v gave a %d-point transform", v, n)
		}
		wfall.chanF = v
		if c := wfall.channel(); int(c) < 0 || int(c) >= len(tapChanNames) {
			t.Errorf("CHAN %v gave channel %d", v, c)
		}
	}
}
