package attractor

import (
	"math"
	"testing"
)

// The beam's sound has the circuit as its period: a unit square drawn at a
// given speed repeats every perimeter/speed seconds, and X and Y keep the
// square's proportions (a fixed scale, not one per channel).
func TestTheBeamSoundsItsCircuit(t *testing.T) {
	saved, savedSpd, savedLvl := beam.path, modelOut.speed, modelOut.level
	defer func() { beam.path, modelOut.speed, modelOut.level = saved, savedSpd, savedLvl }()
	beam.path = beamPath{}
	beam.path.fromStrip([]float32{-0.5, -0.5, 0, 0, 0.5, -0.5, 0, 0, 0.5, 0.5, 0, 0, -0.5, 0.5, 0, 0, -0.5, -0.5, 0, 0}, 0, 5)
	modelOut.speed, modelOut.level = 0, 100
	const sr = 48000.0
	period := 4 / beamSpeed() * sr // samples round a perimeter of 4
	var v modelVoice
	var out [3][]float32
	n := int(math.Ceil(period * 3))
	for c := range out {
		out[c] = make([]float32, n)
	}
	v.beamOut(&out, n, sr)
	lo, hi := float32(1), float32(-1)
	for i := range n {
		lo, hi = min(lo, out[0][i], out[1][i]), max(hi, out[0][i], out[1][i])
		if j := i + int(math.Round(period)); j < n && math.Abs(float64(out[0][j]-out[0][i])) > 0.05 {
			t.Fatalf("x at %d is %v and one circuit later %v: period %v samples", i, out[0][i], out[0][j], period)
		}
	}
	if math.Abs(float64(lo)+0.5) > 0.01 || math.Abs(float64(hi)-0.5) > 0.01 {
		t.Errorf("swing %v..%v, want the square's ±0.5", lo, hi)
	}
}
