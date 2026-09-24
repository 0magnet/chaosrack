//go:build js && wasm

package attractor

import (
	"math"
	"testing"

	"github.com/0magnet/chaosrack/pkg/rackspec"
)

// The frame is a real 19-inch panel: an 84 HP opening with the rest of
// the 482.6 mm as the two ears the unit is bolted to the rails through.
// Derived from rackspec rather than chosen, so the frame drawn on
// screen and the one the spec describes cannot come apart.
func TestTheFrameIsAWholeNineteenInchPanel(t *testing.T) {
	saved := layout.scale
	t.Cleanup(func() { layout.scale = saved })
	layout.scale = 1

	opening := rackspec.RowHP * rackspec.HP * rackspec.PxPerMM
	got := unitFrameWidthPx()
	if want := rackspec.PanelWidth19 * rackspec.PxPerMM; math.Abs(got-want) > 0.01 {
		t.Errorf("the frame is %v px, want a whole 19-inch panel at %v", got, want)
	}
	// Two ears plus the opening is the panel, exactly. A rounding error
	// here is a gap at the end of every row.
	if sum := 2*unitEarWidthPx() + opening; math.Abs(sum-got) > 0.01 {
		t.Errorf("two ears plus an 84 HP opening is %v, want the panel's %v", sum, got)
	}
	// And it scales with the interface, or the frame stops matching the
	// modules in it the moment the Size ring moves.
	layout.scale = 2
	if d := unitFrameWidthPx() - 2*got; math.Abs(d) > 0.01 {
		t.Errorf("at scale 2 the frame is %v, want twice %v", unitFrameWidthPx(), got)
	}
}
