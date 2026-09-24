package attractor

import "github.com/0magnet/chaosrack/pkg/racksurface"

// The rack as a SURFACE, for a front end that draws it rather than printing it.
//
// DrawRackFrom, in rackascii.go, answers the same question as text: which
// module is in which bay. This answers it as geometry — every module's
// rectangle on a canvas the size of the rack — because a front end that wants
// to put controls on the panels needs to know where the panels ARE, and a
// drawing it would have to parse back is not an answer.
//
// Both build their items the same way and group them with the same function,
// so the picture and the geometry cannot disagree about the bays. That was the
// failure this fixes: the terminal panel used to lay the modules out by
// wrapping them to the terminal's width, which is a second layout, and in it
// the modules were not in bays at all.

// RackItemsFrom turns the barest description of a rack — what each module is
// called, how many slots it takes, and the category a model card belongs to —
// into the items the packer takes, grouped into sections as the rack groups
// them.
//
// rows, when given, is how tall each module wants to be in the caller's own
// units: a front end that draws bigger controls asks for taller modules, and a
// bay ends up as tall as the tallest module in it. It may be nil, and short
// entries take the metrics' default.
//
// The items come back in SECTION order, not the caller's. An index into the
// result is an index into the surface; the Key on each item is how a caller
// finds its own module again.
func RackItemsFrom(keys, cats []string, slots, rows []int) []racksurface.Item {
	n := min(len(slots), len(keys))
	items := make([]packItem, 0, n)
	for i := range n {
		it := packItem{
			Key:     keys[i],
			Title:   keys[i],
			Slots:   slots[i],
			Section: drawSectionOf(keys[i], cats, i),
			Lead:    bayScreenKeys[keys[i]],
		}
		if i < len(rows) {
			it.Rows = rows[i]
		}
		items = append(items, it)
	}
	items, _ = groupDrawBySection(items)
	return items
}

// RackSurfaceFrom lays a rack out at the given scale.
func RackSurfaceFrom(keys, cats []string, slots, rows []int, capacity int, monitors map[string]int, m racksurface.Metrics) racksurface.Surface {
	return racksurface.Build(RackItemsFrom(keys, cats, slots, rows), capacity, monitors, m)
}
