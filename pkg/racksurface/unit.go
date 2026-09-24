package racksurface

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
// This file is the arithmetic of one unit: how many slots its opening holds
// and how many are left over. Which modules land in which unit is Pack's.

import "github.com/0magnet/chaosrack/pkg/rackspec"

// UnitCapacity is how many module slots one subrack's opening holds.
//
// A real number, not a window measurement: 84 HP of opening at 7 HP a
// module is twelve. That is the whole point of declaring it — a module's
// home becomes a function of the order it is in and how wide it is, the way
// it is in a rack you could touch, rather than of how wide the browser
// happens to be.
func UnitCapacity() int { return rackspec.SlotsPerRow() }

// usedSlots is how much of a unit's opening its modules take.
func usedSlots(slots []int, idx []int) int {
	used := 0
	for _, i := range idx {
		if i < 0 || i >= len(slots) {
			continue
		}
		w := max(slots[i], 0)
		used += w
	}
	return used
}

// BlankSlots is what is left at the end of a unit, which a rack fills
// with blank panels rather than leaving as a ragged gap.
//
// Never negative: a unit holding one oversized module is already over its
// capacity and has no room for blanks, and a negative count would be asked
// to draw that many of them.
func BlankSlots(slots []int, idx []int, capacity int) int {
	n := capacity - usedSlots(slots, idx)
	if n < 0 {
		return 0
	}
	return n
}
