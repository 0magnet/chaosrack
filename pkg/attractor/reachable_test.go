package attractor

import "testing"

// Every model has to be reachable by turning one knob on one row, and nothing
// else. That is a chain, and each link is a separate way to lose a model:
//
//	modeInfo  →  modeGroups  →  a category, which is a row
//	                         →  a position on that row's rotary
//
// TestEveryModeIsInAGroup already guards the first link. These guard the rest.
//
// They used to guard a different control: two concentric knobs on the Console,
// the outer ring picking a category from a table of four-character tags. The
// tags went with the knob — a dial ring has room for four characters and a row
// header has room for the name — so what is left to check is the row, the
// rotary, and the one line that says what the category is for.

func TestEveryCategoryRowHasABayAndATitle(t *testing.T) {
	// A category with no row is a category whose models can only be reached
	// by a permalink, because the rotary that offers them IS the row.
	for _, g := range Catalog() {
		sec := categorySection(g.Label)
		if !isCategorySection(sec) {
			t.Errorf("category %q produced %q, which is not a model row", g.Label, sec)
			continue
		}
		if sectionRank(sec) >= len(sectionOrder) {
			t.Errorf("category %q has no bay in the rack", g.Label)
		}
		if sectionTitleOf(sec) == "" {
			t.Errorf("category %q has no title to silkscreen on its row", g.Label)
		}
	}
}

func TestEveryCategoryHasAHeaderTooltip(t *testing.T) {
	// The row header is the only thing on a row that says what the category
	// is FOR — the rotary names models, and a list of model names does not
	// add up to the idea. With no entry the header falls back to repeating
	// its own label, which explains nothing.
	for _, g := range Catalog() {
		if catTooltips[g.Label] == "" {
			t.Errorf("category %q has no header tooltip; its row would only repeat its own name", g.Label)
		}
	}
}

func TestHeaderTooltipsAreDistinct(t *testing.T) {
	// Two rows with the same sentence say nothing about either.
	seen := map[string]string{}
	for cat, tip := range catTooltips {
		if prev, dup := seen[tip]; dup {
			t.Errorf("categories %q and %q share the header tooltip %q", prev, cat, tip)
		}
		seen[tip] = cat
	}
}

func TestNoHeaderTooltipIsOrphaned(t *testing.T) {
	// The other direction: a tooltip for a category that does not exist any
	// more is a description nobody can reach, while the live row it was
	// renamed to falls back to its own name.
	live := map[string]bool{}
	for _, g := range Catalog() {
		live[g.Label] = true
	}
	for cat := range catTooltips {
		if !live[cat] {
			t.Errorf("header tooltip for %q, which is not a category any more", cat)
		}
	}
}

func TestEveryModelIsReachableFromItsRow(t *testing.T) {
	// The whole requirement, stated once: every registered mode is offered by
	// some row's rotary.
	reach := map[string]string{}
	for _, g := range Catalog() {
		for _, m := range g.Models {
			if m.Label == "" {
				t.Errorf("%s/%s has no label; the rotary would read out a blank", g.Label, m.Key)
			}
			if _, dup := reach[m.Key]; !dup {
				reach[m.Key] = g.Label
			}
		}
	}
	for key, info := range modeInfo {
		if _, ok := reach[key]; !ok {
			t.Errorf("mode %q (%s) is on no row's rotary and cannot be reached", key, info.Label)
		}
	}
	if len(reach) != len(modeInfo) {
		t.Errorf("%d models reachable, %d registered", len(reach), len(modeInfo))
	}
}
