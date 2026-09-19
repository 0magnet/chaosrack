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
	saved := panelScale
	t.Cleanup(func() { panelScale = saved })
	panelScale = 1

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
	panelScale = 2
	if d := unitFrameWidthPx() - 2*got; math.Abs(d) > 0.01 {
		t.Errorf("at scale 2 the frame is %v, want twice %v", unitFrameWidthPx(), got)
	}
}

// A bay is a whole number of slots wide. Left to the window's width the row
// ended wherever it ended, and the leftover between the last module and the
// right ear was not a slot — which is not something a rack can contain.
func TestEveryCatalogCategoryGetsItsOwnDetent(t *testing.T) {
	seen := map[string]string{}
	for _, g := range Catalog() {
		cat := optgroupCategory(g.Label)
		if cat == "" {
			t.Errorf("category %q maps to nothing", g.Label)
			continue
		}
		if other, dup := seen[cat]; dup {
			t.Errorf("%q and %q both map to the knob position %q — one of them is hidden inside the other",
				other, g.Label, cat)
			continue
		}
		seen[cat] = g.Label
		if short := catShortLabel(g.Label); short == g.Label {
			t.Errorf("category %q has no short tag for the knob ring; it would print in full and overlap its neighbors",
				g.Label)
		}
	}
	if len(seen) != len(Catalog()) {
		t.Errorf("%d knob positions for %d categories", len(seen), len(Catalog()))
	}
}

// And the reverse: a mode in no category is a mode with no way in at all.
func TestEveryModeIsReachableFromSomeCategory(t *testing.T) {
	inGroup := map[string]bool{}
	for _, g := range Catalog() {
		for _, m := range g.Models {
			inGroup[m.Key] = true
		}
	}
	for _, k := range CatalogKeys() {
		if !inGroup[k] {
			t.Errorf("mode %q is in no category, so the selector cannot reach it", k)
		}
	}
}
