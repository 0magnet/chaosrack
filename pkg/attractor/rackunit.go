package attractor

// The level that was missing: the rack UNIT.
//
// There are three containers in a rack and chaosrack had one. A 19-inch
// frame holds units; a unit bolts to the rails and occupies some number of
// U; a unit that is a subrack has an OPENING, and plug-in modules go in the
// opening on HP pitch. The panel had only the third of those — a flow of
// modules — and drew a frame behind whatever the browser's flex-wrap
// happened to do, which made "which bay is this module in" a function of
// the window's width. There was therefore nowhere to put a thing that is
// not a plug-in card, which is why the rack-mount scope ended up
// implemented as one.
//
// The important part of the model is that a subrack and an instrument are
// not two kinds of unit. A unit has a PANEL, and optionally an OPENING. A
// plain card cage is nearly all opening; the scope is all panel; a Tek
// 7000-series or HP 180-series scope is a panel WITH an opening, and that
// falls out of this rather than needing the level invented again.
//
// This file is the arithmetic: how many slots a unit holds and which
// modules land in which one. Pure, because the bugs in packing are all
// off-by-ones at the capacity boundary and none of them need a browser.

import "github.com/0magnet/chaosrack/pkg/rackspec"

// unitCapacitySlots is how many module slots one subrack's opening holds.
//
// A real number, not a window measurement: 84 HP of opening at 7 HP a
// module is twelve. That is the whole point of declaring it — a module's
// home becomes a function of the order it is in and how wide it is, the way
// it is in a rack you could touch, rather than of how wide the browser
// happens to be.
func unitCapacitySlots() int { return rackspec.SlotsPerRow() }

// packUnits assigns modules to units. slots[i] is how many slots module i
// takes; the result is the module indices in each unit, in order.
//
// Fill and overflow, which is what putting cards in a rack is: they go in
// left to right until the next one does not fit, and then it starts the
// next unit. No attempt to pack tightly by reordering — a rack does not
// rearrange your modules to save a slot, and one that did would move things
// out from under the person who put them there.
func packUnits(slots []int, capacity int) [][]int {
	if capacity < 1 {
		capacity = 1
	}
	var units [][]int
	var cur []int
	used := 0
	for i, w := range slots {
		// Zero is a module that is switched OUT: it keeps its place in the
		// order — so that putting it back does not move it to the front —
		// and takes no room in the opening, because a module that is not
		// in the rack must not reserve space in it.
		if w < 0 {
			w = 0
		}
		// A module wider than a whole unit cannot be made to fit, and
		// refusing to place it would lose it. It gets a unit of its own and
		// overhangs — visible, which is the right outcome for a thing that
		// is genuinely too big, rather than silently dropped.
		if w > capacity {
			if len(cur) > 0 {
				units = append(units, cur)
				cur, used = nil, 0
			}
			units = append(units, []int{i})
			continue
		}
		if used+w > capacity && len(cur) > 0 {
			units = append(units, cur)
			cur, used = nil, 0
		}
		cur = append(cur, i)
		used += w
	}
	if len(cur) > 0 {
		units = append(units, cur)
	}
	return units
}

// unitUsedSlots is how much of a unit's opening its modules take.
func unitUsedSlots(slots []int, idx []int) int {
	used := 0
	for _, i := range idx {
		if i < 0 || i >= len(slots) {
			continue
		}
		w := slots[i]
		if w < 0 {
			w = 0
		}
		used += w
	}
	return used
}

// unitBlankSlots is what is left at the end of a unit, which a rack fills
// with blank panels rather than leaving as a ragged gap.
//
// Never negative: a unit holding one oversized module is already over its
// capacity and has no room for blanks, and a negative count would be asked
// to draw that many of them.
func unitBlankSlots(slots []int, idx []int, capacity int) int {
	n := capacity - unitUsedSlots(slots, idx)
	if n < 0 {
		return 0
	}
	return n
}
