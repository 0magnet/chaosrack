package attractor

import "testing"

// A source is a 0..1 signal by contract, and the depth knob is what scales
// it. A coordinate that escaped that range would become a modulation spike
// larger than the parameter's whole span — which is exactly the runaway a
// feedback path must not have for free.
func TestAModelSourceStaysInTheZeroToOneASourceMeans(t *testing.T) {
	const ext = 20
	for _, v := range []float32{-1000, -ext, -1, 0, 1, ext, 1000} {
		got := modelModNorm(v, ext)
		if got < 0 || got > 1 {
			t.Errorf("coordinate %v normalized to %v, outside 0..1", v, got)
		}
	}
	// The center of the range is the middle of the signal, so a model sitting
	// at the origin modulates nothing off its knob's own setting.
	if got := modelModNorm(0, ext); got != 0.5 {
		t.Errorf("the origin normalized to %v, want 0.5", got)
	}
	// And the ends are the ends.
	if got := modelModNorm(-ext, ext); got != 0 {
		t.Errorf("the negative extent normalized to %v, want 0", got)
	}
	if got := modelModNorm(ext, ext); got != 1 {
		t.Errorf("the positive extent normalized to %v, want 1", got)
	}
}

// Before there is a fit there is no scale to normalize against, and guessing
// one would drive every routed parameter from a number that means nothing.
// The middle modulates nothing.
func TestWithNoCameraFitAModelSourceSitsStill(t *testing.T) {
	for _, ext := range []float32{0, -1} {
		if got := modelModNorm(5, ext); got != 0.5 {
			t.Errorf("extent %v gave %v, want the inert 0.5", ext, got)
		}
	}
}

// Normalizing against the CAMERA'S extent rather than a fixed number is what
// lets one source serve a Lorenz running to 50 and a polyhedron running to 1.
// The same relative position must give the same signal on both.
func TestTheSameRelativePositionReadsTheSameOnAnyModel(t *testing.T) {
	big := modelModNorm(25, 50)   // halfway out on a large attractor
	small := modelModNorm(0.5, 1) // halfway out on a small one
	if big != small {
		t.Errorf("halfway out reads %v on a big model and %v on a small one", big, small)
	}
}

// The smoothing is what stops the loop oscillating at the frame rate. It has
// to converge, and it has to take long enough to be a drift rather than a
// buzz.
func TestTheLoopSmoothingConvergesButNotWithinAFrame(t *testing.T) {
	v := float32(0)
	const target = 1
	if after := modelModSmooth(v, target); after >= 0.5 {
		t.Errorf("one frame moved the source to %v — that is a frame-rate oscillator, not a loop", after)
	}
	for range 500 {
		v = modelModSmooth(v, target)
	}
	if v < 0.99 {
		t.Errorf("after 500 frames the source is at %v, want it converged on %v", v, target)
	}
	// It must converge DOWNWARD too, or a source that has been high stays high.
	for range 500 {
		v = modelModSmooth(v, 0)
	}
	if v > 0.01 {
		t.Errorf("after 500 frames falling, the source is at %v, want ~0", v)
	}
}

// The model sources have to be recognized as model sources, and the audio
// channels must not be — "mono" starts with an m too, and routing it through
// the model tap would silently replace the audio with a coordinate.
func TestModelSourcesAreToldApartFromTheAudioChannels(t *testing.T) {
	for _, s := range []string{modSrcModelX, modSrcModelY, modSrcModelZ, modSrcModelR} {
		if !isModelModSource(s) {
			t.Errorf("%q is a model source and was not recognized as one", s)
		}
	}
	for _, s := range []string{"", "mono", "L", "R"} {
		if isModelModSource(s) {
			t.Errorf("%q is an audio channel and was taken for a model source", s)
		}
	}
}
