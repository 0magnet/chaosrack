package attractor

import "testing"

// The color range closes in the same TIME whatever the frame rate: a second
// at 15 fps (four 60 Hz frames per fill) lands where a second at 60 fps does.
// It closed by a fixed step per fill, so a throttled tab held a transient's
// range four times as long.
func TestTheColorRangeClosesInTimeNotFrames(t *testing.T) {
	closeFor := func(fps int) float32 {
		a := AudioColor{lo: 0.4, hi: 0.9, k: float32(60 / fps)}
		for range fps { // one second
			a.stretchAudioColorLUT([]float32{0.4, 0.5})
		}
		return a.hi
	}
	at60, at15 := closeFor(60), closeFor(15)
	if d := at60 - at15; d > 1e-4 || d < -1e-4 {
		t.Fatalf("after a second the upper bound is %v at 60 fps and %v at 15", at60, at15)
	}
	if at60 > 0.85 {
		t.Fatalf("after a second the upper bound is still %v; it should have eased back from 0.9", at60)
	}
}
