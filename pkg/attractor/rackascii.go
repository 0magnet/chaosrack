package attractor

import (
	"fmt"
	"strings"
)

// Drawing the rack as text.
//
// Not as documentation of what it looks like — as the thing itself, at the
// one resolution where a rack of 84 HP rows IS a table: a row is a fixed
// number of slots, a module occupies a whole number of them, and everything
// else is blank panel. Drawn from the packer's own output rather than from a
// description of it, so it cannot disagree with the rack: change packBySection
// and the picture changes with it, or a test fails.
//
// It is also the smallest possible renderer for the layout, which is the
// point beyond a diagram. The rack's model already lives in untagged Go —
// racksection.go packs it, rackspec.go dimensions it, racklayout.go persists
// it — and only the DOM half is behind //go:build js. A text renderer is a
// second backend over the same model, and the first evidence that the seam
// between "what the rack IS" and "how it is drawn" is real rather than
// asserted.

// rackDrawing is what the renderer needs to know about one module.
type rackModule struct {
	Key   string // what is silkscreened on it
	Slots int
}

// rackDrawWidth is how many characters a slot gets. Two is enough to tell a
// one-slot module from a two-slot one at a glance, leaves room for most of a
// module.s name, and keeps an 84 HP row
// inside a terminal.
const rackDrawWidth = 4

// drawRack renders the bays as text.
//
// mods is every module in packing order, units is packBySection's output over
// the same indices, and monitors says which sections carry a chassis monitor
// and how wide it is — the same map the packer was given, so the picture
// shows the monitor exactly where the packer reserved its room.
func drawRack(mods []rackModule, items []packItem, units [][]int, capacity int, monitors map[string]int) string {
	if capacity < 1 {
		capacity = 1
	}
	total := capacity * rackDrawWidth
	var b strings.Builder
	fmt.Fprintf(&b, "┌%s┐\n", strings.Repeat("─", total))
	blankTotal := 0
	for n, u := range units {
		sec := unitSection(items, u)
		used := 0
		var row strings.Builder
		// The chassis monitor, at the left, where the packer charged for it.
		// Drawn as the frame rather than as a panel: it is not plugged in.
		if mw := monitors[sec]; mw > 0 {
			if mw >= capacity {
				mw = capacity - 1
			}
			row.WriteString(strings.Repeat("▚", mw*rackDrawWidth))
			used += mw
		}
		for _, i := range u {
			if i < 0 || i >= len(mods) || mods[i].Slots <= 0 {
				continue // switched out: no slots, no panel
			}
			row.WriteString(panelCell(mods[i].Key, mods[i].Slots))
			used += mods[i].Slots
		}
		blank := max(capacity-used, 0)
		blankTotal += blank
		row.WriteString(strings.Repeat("·", blank*rackDrawWidth))
		fmt.Fprintf(&b, "│%s│ bay %-2d %s\n", padRunes(row.String(), total), n+1, bayLabel(items, u))
	}
	fmt.Fprintf(&b, "└%s┘\n", strings.Repeat("─", total))
	slots := len(units) * capacity
	if slots > 0 {
		fmt.Fprintf(&b, "%d bays · %d slots · %d blank (%d%%)\n",
			len(units), slots, blankTotal, 100*blankTotal/slots)
	}
	return b.String()
}

// panelCell is one module's panel: a separator and its name, trimmed to the
// room the module actually has. Exactly slots*rackDrawWidth runes wide, so a
// row's width is the sum of what is in it and nothing has to be measured.
func panelCell(name string, slots int) string {
	w := max(slots*rackDrawWidth-1, 1)
	r := []rune(name)
	if len(r) > w {
		r = r[:w]
	}
	return "│" + string(r) + strings.Repeat(" ", w-len(r))
}

// bayLabel names the sections a bay carries, in order.
func bayLabel(items []packItem, idx []int) string {
	var names []string
	for _, r := range sectionRuns(items, idx) {
		t := sectionTitleOf(r.Section)
		if t == "" {
			t = strings.ToUpper(r.Section)
		}
		names = append(names, t)
	}
	return strings.Join(names, " + ")
}

func padRunes(s string, w int) string {
	n := len([]rune(s))
	if n >= w {
		return string([]rune(s)[:w])
	}
	return s + strings.Repeat(" ", w-n)
}

// DrawRackFrom packs and draws a rack from the barest description of it: what
// each module is called and how many slots it takes, in the order the frame
// holds them.
//
// The sections come from the same rule sectionOfModule uses — a model card
// belongs to its category's row and everything else to the table the rack
// itself uses, so a drawing cannot put a module in a bay the rack would not —
// and the packing is packBySection, so it cannot disagree about the bays
// either. What a caller supplies is only what has to be MEASURED: a module's
// width is decided by its contents at the interface scale in use, which is a
// question only a laid-out panel can answer.
//
// Exported for cmd/uitool, and for whatever renders the rack next: this is
// the whole input a second front end needs.
func DrawRackFrom(keys, cats []string, slots []int, capacity int, monitors map[string]int) string {
	n := min(len(slots), len(keys))
	mods := make([]rackModule, 0, n)
	items := make([]packItem, 0, n)
	for i := range n {
		mods = append(mods, rackModule{Key: keys[i], Slots: slots[i]})
		items = append(items, packItem{
			Slots:   slots[i],
			Section: drawSectionOf(keys[i], cats, i),
			Lead:    drawLead(keys[i], cats, i),
		})
	}
	items, order := groupDrawBySection(items)
	om := make([]rackModule, len(order))
	for i, j := range order {
		om[i] = mods[j]
	}
	return drawRack(om, items, packBySection(items, capacity, monitors), capacity, monitors)
}

// bayScreenKeys is which modules carry a screen of their own, mirroring
// bayScreens in the js half — the pure side needs it to draw the same bays.
var bayScreenKeys = map[string]bool{"desk": true, "record": true}

// drawLead is whether a measured module must open a bay, by the same rule
// the page applies: it carries a screen, or it is a model row's head. A head
// is a card whose name is its own category's, "Attractors" or "Attractors 2";
// the Analysis meter shares a name with a category but is not a card, so it
// is not taken for one.
func drawLead(key string, cats []string, i int) bool {
	if bayScreenKeys[key] {
		return true
	}
	if i >= len(cats) || cats[i] == "" {
		return false
	}
	rest, ok := strings.CutPrefix(key, strings.ToLower(cats[i]))
	if !ok {
		return false
	}
	if rest == "" {
		return true
	}
	n, ok := strings.CutPrefix(rest, " ")
	return ok && n != "" && strings.Trim(n, "0123456789") == ""
}

// groupDrawBySection makes each section contiguous, as the rack does before
// packing, and reports where each item came from so the names follow it.
func groupDrawBySection(items []packItem) ([]packItem, []int) {
	order := make([]int, 0, len(items))
	for _, s := range sectionOrder {
		for i, it := range items {
			if it.Section == s {
				order = append(order, i)
			}
		}
	}
	// Anything whose section is not in the order at all still gets drawn,
	// at the end, which is where an unplaced module belongs.
	seen := make(map[int]bool, len(order))
	for _, i := range order {
		seen[i] = true
	}
	for i := range items {
		if !seen[i] {
			order = append(order, i)
		}
	}
	out := make([]packItem, len(order))
	for i, j := range order {
		out[i] = items[j]
	}
	return out, order
}

// drawSectionOf is sectionOfModule's rule, for a caller that has read the
// module's category off the panel rather than holding the element: a model
// card belongs to its category's row, and everything else to the table.
func drawSectionOf(key string, cats []string, i int) string {
	if i < len(cats) && cats[i] != "" {
		return categorySection(cats[i])
	}
	return moduleSection(key)
}

// DrawSectionOf is drawSectionOf for a caller outside this package.
func DrawSectionOf(key string, cats []string, i int) string { return drawSectionOf(key, cats, i) }
