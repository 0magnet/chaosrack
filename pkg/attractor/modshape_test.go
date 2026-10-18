package attractor

import "testing"

// beats is an envelope as the features make it: a hit every half second at
// 60 frames, rising fast and falling slowly (FeatureSmooth), over a floor.
func beats(frames int) []float32 {
	out := make([]float32, frames)
	var cur float32
	for i := range out {
		target := float32(0.2)
		if i%30 < 3 {
			target = 1
		}
		cur = FeatureSmooth(cur, target)
		out[i] = cur
	}
	return out
}

// signals runs env through a fresh route of shape from the rack's mono.
func signals(shape string, env []float32) []float32 {
	var s ModShaper
	out := make([]float32, len(env))
	for i, f := range env {
		out[i] = s.Signal(shape, "mono", f)
	}
	return out
}

// After the first few seconds, the extremes and the average.
func spread(sig []float32) (lo, hi, mean float32) {
	sig = sig[len(sig)/2:]
	lo, hi = sig[0], sig[0]
	for _, v := range sig {
		lo, hi, mean = min(lo, v), max(hi, v), mean+v
	}
	return lo, hi, mean / float32(len(sig))
}

func TestPushMovesOneWay(t *testing.T) {
	lo, _, _ := spread(signals(ModShapePush, beats(600)))
	if lo < 0 {
		t.Errorf("push went below its setting: %v", lo)
	}
}

func TestSwingMovesBothWaysAroundTheSetting(t *testing.T) {
	lo, hi, mean := spread(signals(ModShapeSwing, beats(600)))
	if lo > -0.1 || hi < 0.5 {
		t.Errorf("swing reached only %v..%v", lo, hi)
	}
	if mean < -0.1 || mean > 0.1 {
		t.Errorf("swing sits at %v on average, not at the setting", mean)
	}
}

func TestFlipAlternatesWithTheBeat(t *testing.T) {
	sig := signals(ModShapeFlip, beats(600))
	// Each beat's peak, in turn, the way it went.
	var peaks []float32
	for b := 4; b < 20; b++ {
		var p float32
		for _, v := range sig[b*30 : b*30+15] {
			if v*v > p*p {
				p = v
			}
		}
		peaks = append(peaks, p)
	}
	for i := 1; i < len(peaks); i++ {
		if peaks[i]*peaks[i-1] >= 0 || peaks[i]*peaks[i] < 0.25 {
			t.Fatalf("beats went %v", peaks)
		}
	}
}

func TestASteadySoundSwingsAndFlipsNothing(t *testing.T) {
	steady := make([]float32, 600)
	for i := range steady {
		steady[i] = 0.7
	}
	for _, shape := range []string{ModShapeSwing, ModShapeFlip} {
		if lo, hi, _ := spread(signals(shape, steady)); lo < -0.01 || hi > 0.01 {
			t.Errorf("%q under a steady tone moved %v..%v", shape, lo, hi)
		}
	}
}

// A coordinate's middle is the model at its center, which moves nothing.
func TestAPushedCoordinateIsCentered(t *testing.T) {
	var s ModShaper
	if v := s.Signal(ModShapePush, modSrcModelX, 0.5); v != 0 {
		t.Errorf("head x at the center pushes %v", v)
	}
	if v := s.Signal(ModShapePush, modSrcModelX, 0); v != -1 {
		t.Errorf("head x at the far left pushes %v", v)
	}
	if v := s.Signal(ModShapePush, modSrcModelR, 0.5); v != 0.5 {
		t.Errorf("head r is a distance, but pushes %v", v)
	}
}

func TestEveryShapeIsStorable(t *testing.T) {
	for _, m := range ModShapes {
		if !IsModShape(m.Value) || m.Label == "" {
			t.Errorf("shape %+v", m)
		}
	}
	if IsModShape("wobble") {
		t.Error("an unknown shape is storable")
	}
}
