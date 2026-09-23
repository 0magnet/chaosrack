// Package racksurface lays a rack out: which modules share a bay, and where
// every one of them sits on a surface of a FIXED size.
//
// It knows nothing about chaosrack. An Item is a width in slots, the bay it
// belongs to and whether it has to lead one; a Metrics says how many cells a
// slot and a bay are worth. What a "console" is, which module is in it and
// what the bays are called all stay with the caller — this package is the
// arithmetic, and it is the only copy of it.
//
// That matters because there used to be two. The page packed modules into
// bays with the rule below, and the terminal panel laid the same modules out
// by wrapping them to the terminal's width, which is a second layout that
// could — and did — disagree with the first: in the terminal the modules were
// not in bays at all. A surface fixes that by not depending on the viewer.
// The rack is the size the rack is; a window looks at part of it.
//
// The packer half was lifted out of pkg/attractor unchanged. If a second
// program ever wants racks of this shape, this package (not that one) is what
// it should take, and rack-go — which delegates layout to CSS and has no pure
// pass of its own — is where it would go.
package racksurface

// Item is one module as the packer sees it: how many slots it needs, which
// bay it belongs in, and whether it has to be the first thing in one.
type Item struct {
	// Key and Title are carried through untouched, for the caller to find
	// its own module again in the result.
	Key   string
	Title string

	Slots   int
	Section string
	// Lead is a module that must START a bay: the head panel of a model
	// row, which carries that row's monitor. See Pack.
	Lead bool

	// Rows is how tall this module needs to be. Zero takes the metrics'
	// default.
	//
	// Height is the one dimension the RENDERER gets to decide, and the
	// asymmetry is deliberate. A module's width is the rack's business — it
	// is a whole number of slots and the frame is a fixed number of slots
	// across — but how many rows its controls need depends on how they are
	// drawn, which is a different answer in a browser and in a terminal. A
	// bay then takes the height of the tallest module in it, exactly as a
	// shelf of different-height chassis would.
	Rows int
}

// SectionOf is the bay a packed unit belongs to, which is the section of
// whatever is in it. Empty for an empty unit.
func SectionOf(items []Item, idx []int) string {
	for _, i := range idx {
		if i >= 0 && i < len(items) {
			return items[i].Section
		}
	}
	return ""
}

// Pack assigns modules to bays, fitting as many sections into a bay as will
// go.
//
// It used to break at every section, which made the labels honest and the
// rack empty: ten bays, twenty-six modules and 69 blank slots of 120 — 58%
// of the rack was blank panel. Most sections are nowhere near 84 HP wide,
// and a bay per section spends a whole row on a section holding one module.
//
// So a bay carries several sections now, each marked over its own span.
// That is what a real 84 HP row looks like — several functional groups
// sharing it — and it is Woodson & Conover's own answer for identifying a
// group WITHIN a row rather than by giving it a row: "adequate spacing of
// display or control groups... marked outlines around each group... area
// color patterning" (§2-133). Only the break rule changed; a section still
// never interleaves with another, because the caller has already made each
// one contiguous.
//
// The one thing that does force a break is a Lead item — a bay head, which
// carries a monitor. A BAY BEGINS WITH A SCREEN: whatever else shares the
// row, the leftmost panel in it is a display, so a reader scanning down the
// left edge of the frame finds one per row. See the break rule below.
// A BAY IS A CHASSIS, NOT A SHELF.
//
// monitor maps a section to how many slots its built-in monitor takes. A
// section that is not in it has none yet and packs exactly as before, so the
// rack can grow monitors one bay at a time rather than in one change.
func Pack(items []Item, capacity int, monitor map[string]int) [][]int {
	if capacity < 1 {
		capacity = 1
	}
	// A monitor wider than the bay it leads would leave no room for the
	// modules it monitors, which is not a rack, it is a screen with a
	// caption. Clamped rather than rejected: the rack still draws.
	monitorFor := func(section string) int {
		w := monitor[section]
		if w < 0 {
			w = 0
		}
		if w >= capacity {
			w = capacity - 1
		}
		return w
	}
	var units [][]int
	var cur []int
	used := 0
	led := false // this bay begins with a head
	flush := func() {
		if len(cur) > 0 {
			units = append(units, cur)
			cur, used, led = nil, 0, false
		}
	}
	for i, it := range items {
		w := it.Slots
		if w < 0 {
			w = 0
		}
		// Where a head may go, in two parts.
		//
		// It may not follow something that is not part of a head's run,
		// because then the bay would begin with that instead and the row
		// would have no display at its left edge. And its whole run has to
		// fit in what is left, because a head exists to introduce the
		// generators beside it, and a break inside one of those groups puts
		// a monitor in a different row from the models it is monitoring.
		//
		// Otherwise a head is free to share: two or three small rows in one
		// bay, each opening with its own screen, is what an 84 HP row of a
		// real rack looks like. Breaking at EVERY head instead reads the
		// same and costs four bays of blank panel — measured, 21 bays where
		// 17 hold it.
		//
		// A head that is switched OUT takes no slots and breaks nothing: a
		// bay boundary drawn for a module that is not in the rack is a blank
		// row.
		if it.Lead && w > 0 && used > 0 && (!led || used+LeadRun(items, i) > capacity) {
			flush()
		}
		if w > capacity {
			// Too big for any bay. Its own, overhanging — visible, which is
			// the right outcome for a thing that genuinely does not fit.
			flush()
			units = append(units, []int{i})
			continue
		}
		// The bay's own monitor, charged as the bay is opened. It belongs to
		// whichever section opens the bay: a bay carries one screen at its
		// left, and the sections that come to share the row behind it are
		// plugged into that one rather than bringing their own.
		open := len(cur) == 0
		mw := 0
		if open {
			mw = monitorFor(it.Section)
		}
		if used+mw+w > capacity && len(cur) > 0 {
			flush()
			open, mw = true, monitorFor(it.Section)
		}
		if open {
			used += mw
			led = mw > 0 || (it.Lead && w > 0)
		}
		cur = append(cur, i)
		used += w
	}
	flush()
	return units
}

// LeadRun is how much room the head at i wants: itself, and the modules that
// follow it until the next head or the end of its own section.
//
// That run is one group — a monitor, the selector that picks among the
// generators beside it, and those generators. It is the thing that has to
// stay in one row; the modules after it belong to some other head.
func LeadRun(items []Item, i int) int {
	n := items[i].Slots
	for j := i + 1; j < len(items); j++ {
		if items[j].Lead || items[j].Section != items[i].Section {
			break
		}
		n += items[j].Slots
	}
	return n
}

// Run is one section's stretch inside a bay: where it starts among the bay's
// modules, and how many of them it covers.
type Run struct {
	Section string
	From    int // index into the unit's own module list
	Count   int
}

// Runs splits a bay into the sections it carries, in order.
//
// A bay with three groups in it needs three labels, each over the modules it
// names — one label on a bay holding three sections would be a label that is
// two-thirds wrong.
func Runs(items []Item, idx []int) []Run {
	var out []Run
	for n, i := range idx {
		sec := ""
		if i >= 0 && i < len(items) {
			sec = items[i].Section
		}
		if len(out) > 0 && out[len(out)-1].Section == sec {
			out[len(out)-1].Count++
			continue
		}
		out = append(out, Run{Section: sec, From: n, Count: 1})
	}
	return out
}
