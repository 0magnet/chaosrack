//go:build js && wasm

package attractor

import (
	"math"
	"testing"
)

// FROM at 0 is the model's own trajectory; above it ε rises by powers of
// ten to 10⁻¹ at TO = 1, so a grid of two from 0 to 1 is the model and a
// copy 0.1 away.
func TestStartEpsRunsFromTheModelOutward(t *testing.T) {
	saved := grid
	defer func() { grid = saved }()
	grid.sweepIDs = []string{"", "#src", "#map", "#start"}
	grid.sweepParamF, grid.sweep2ParamF = 3, 0
	grid.sweepLo, grid.sweepHi = 0, 1

	if e := startEps(0, 2); e != 0 {
		t.Errorf("FROM 0: ε = %g, want 0, the model itself", e)
	}
	if e := startEps(1, 2); math.Abs(e-1e-1) > 1e-12 {
		t.Errorf("TO 1: ε = %g, want 0.1", e)
	}
	prev := -1.0
	for i := range 9 {
		e := startEps(i, 9)
		if e <= prev {
			t.Errorf("cell %d: ε %g does not rise from %g", i, e, prev)
		}
		prev = e
	}
}
