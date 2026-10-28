//go:build js && wasm

package attractor

import (
	"math"
	"testing"
)

// TestDiverged keeps the inlinable form saying what the explicit one did: a
// NaN in any coordinate, or one past ±1e4, is diverged; the bound itself is
// not.
func TestDiverged(t *testing.T) {
	nan := float32(math.NaN())
	inf := float32(math.Inf(1))
	for _, c := range []struct {
		x, y, z float32
		want    bool
	}{
		{0, 0, 0, false},
		{1e4, -1e4, 1e4, false},
		{1e4 + 1, 0, 0, true},
		{0, -1e4 - 1, 0, true},
		{0, 0, inf, true},
		{nan, 0, 0, true},
		{0, nan, 0, true},
		{0, 0, nan, true},
	} {
		if got := diverged(c.x, c.y, c.z); got != c.want {
			t.Errorf("diverged(%v, %v, %v) = %v, want %v", c.x, c.y, c.z, got, c.want)
		}
	}
}
