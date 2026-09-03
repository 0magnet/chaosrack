//go:build js && wasm

package attractor

import (
	"math"
	"testing"

	"github.com/0magnet/chaosrack/pkg/rackspec"
)

// A bay is a whole number of slots wide. Left to the window's width the row
// ended wherever it ended, and the leftover between the last module and the
// right ear was not a slot — which is not something a rack can contain.
func TestBaySlotsIsWholeAndCapped(t *testing.T) {
	const scale = 1.0
	slot := moduleSlot * scale
	pitch := slot + moduleGap*scale
	ear := earWidthMM * rackspec.PxPerMM * scale

	// Exactly n slots plus the ears must yield exactly n.
	for n := 1; n <= rackspec.SlotsPerRow(); n++ {
		natural := float64(n)*pitch - moduleGap*scale + 2*ear
		if got := baySlots(natural, pitch, slot, ear); got != n {
			t.Errorf("a bay of exactly %d slots (%.1f px) measured %d", n, natural, got)
		}
		// A hair under n slots is n-1: a slot that does not fit is not a slot.
		if n > 1 {
			if got := baySlots(natural-slot/2, pitch, slot, ear); got != n-1 {
				t.Errorf("a bay half a slot short of %d measured %d, want %d", n, got, n-1)
			}
		}
	}

	// However wide the window, a 19-inch frame has 84 HP of row: twelve
	// 7 HP slots and no more.
	if got := baySlots(100000, pitch, slot, ear); got != rackspec.SlotsPerRow() {
		t.Errorf("an enormous window gave %d slots, want the row's %d", got, rackspec.SlotsPerRow())
	}
	if rackspec.SlotsPerRow() != 12 {
		t.Errorf("an 84 HP row of %d HP slots is %d, not 12", rackspec.ModuleHP, rackspec.SlotsPerRow())
	}
	// And never zero, however narrow.
	if got := baySlots(10, pitch, slot, ear); got < 1 {
		t.Errorf("a tiny window gave %d slots", got)
	}
}

// Whatever the modules leave at the end of a row is filled with blanks, and
// what is left after THEM is nothing — the bay ends where the last blank does.
func TestBlankOffsetsFillTheRow(t *testing.T) {
	const scale = 1.0
	slot := moduleSlot * scale
	pitch := slot + moduleGap*scale
	ear := earWidthMM * rackspec.PxPerMM * scale
	const n = 12
	inner := float64(n)*pitch - moduleGap*scale

	for used := 0; used < n; used++ {
		rowRight := ear
		if used > 0 {
			rowRight = ear + float64(used)*pitch - moduleGap*scale
		}
		got := blankOffsets(rowRight, inner, ear, slot, pitch)
		if len(got) != n-used {
			t.Errorf("%d slots used: %d blanks, want %d", used, len(got), n-used)
			continue
		}
		for i, x := range got {
			want := ear + float64(used+i)*pitch
			if math.Abs(x-want) > 1 {
				t.Errorf("%d used, blank %d at %.1f, want %.1f", used, i, x, want)
			}
		}
		if end := got[len(got)-1] + slot; math.Abs(end-(ear+inner)) > 1 {
			t.Errorf("%d used: blanks end at %.1f, bay ends at %.1f", used, end, ear+inner)
		}
	}

	// A full row leaves none.
	if got := blankOffsets(ear+inner, inner, ear, slot, pitch); len(got) != 0 {
		t.Errorf("a full row produced %d blanks", len(got))
	}
}

// The row grouping decides how many bays are drawn and where. It reads the
// browser's wrapping back rather than predicting it, so a fake DOM with known
// geometry is exactly the right test.
func TestMeasureBayRowsGroupsByTop(t *testing.T) {
	const h = 514.0
	host := newFakeHost(1900,
		newFakeEl("sect").at(0, 0, 140, h),
		newFakeEl("sect").at(142, 0, 282, h),
		newFakeEl("sect").at(426, 0, 140, h),
		// Second row.
		newFakeEl("sect").at(0, 530, 140, h),
		newFakeEl("sect").at(142, 530, 140, h),
		// A module that is switched off has no box and belongs to no row.
		newFakeEl("sect").at(0, 0, 0, 0),
		// The bay's own furniture must not be counted as a module, or every
		// pass would measure the one before it.
		newFakeEl("rack-bay").at(0, 0, 1900, h),
		newFakeEl("rack-blank").at(600, 0, 140, h),
	)

	rows := measureBayRows(host)
	if len(rows) != 2 {
		t.Fatalf("%d rows, want 2", len(rows))
	}
	if rows[0].top != 0 || rows[0].bottom != h {
		t.Errorf("row 0 spans %.0f..%.0f, want 0..%.0f", rows[0].top, rows[0].bottom, h)
	}
	if rows[0].right != 566 {
		t.Errorf("row 0 ends at %.0f, want 566 (the third module's right edge)", rows[0].right)
	}
	if rows[1].top != 530 {
		t.Errorf("row 1 starts at %.0f, want 530", rows[1].top)
	}
	if rows[1].right != 282 {
		t.Errorf("row 1 ends at %.0f, want 282", rows[1].right)
	}
}

// Modules of unequal height still make one row: the tolerance is for a border
// a pixel different, not for a second row.
func TestMeasureBayRowsToleratesAWobble(t *testing.T) {
	host := newFakeHost(1900,
		newFakeEl("sect").at(0, 0, 140, 514),
		newFakeEl("sect").at(142, 2, 140, 512),
	)
	rows := measureBayRows(host)
	if len(rows) != 1 {
		t.Fatalf("%d rows, want 1 — two pixels is not a new row", len(rows))
	}
	if rows[0].bottom != 514 {
		t.Errorf("row bottom %.0f, want the taller module's 514", rows[0].bottom)
	}
}

// Every category of the catalog must get its own position on the selector
// knob, or the modes in it are reachable only by scrolling past the ones that
// swallowed them.
//
// This is the regression: optgroupCategory used to fold everything outside
// four named groups into "Attractors", so the Turtle Path, the Bifurcation
// Explorer and the STL loader landed at the bottom of the Attractors model
// knob behind twenty Sprott systems. Reachability belongs to the registry,
// not to a switch statement in the selector.
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
