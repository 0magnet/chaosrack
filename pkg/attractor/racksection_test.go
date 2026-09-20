package attractor

import "testing"

// A bay holds one section. That is the whole point of the grouping: the
// label on a unit has to be true of everything in it, and a bay holding the
// tail of one section and the head of the next has a label that lies.
func TestABayHoldsOneSection(t *testing.T) {
	items := []packItem{
		{2, secInput}, {1, secInput},
		{2, secAnalyze}, {2, secAnalyze}, {1, secAnalyze},
		{3, secOutput},
	}
	units := packBySection(items, 12)
	if len(units) != 3 {
		t.Fatalf("got %d units for 3 sections, want 3: %v", len(units), units)
	}
	for u, idx := range units {
		want := items[idx[0]].Section
		for _, i := range idx {
			if items[i].Section != want {
				t.Errorf("unit %d mixes %q and %q", u, want, items[i].Section)
			}
		}
	}
}

// It breaks between sections even with room to spare — five slots of
// modules in three sections is three units, not one.
func TestASectionAlwaysStartsANewBay(t *testing.T) {
	items := []packItem{{1, secInput}, {1, secAnalyze}, {1, secOutput}}
	if units := packBySection(items, 12); len(units) != 3 {
		t.Errorf("got %d units, want one per section: %v", len(units), units)
	}
}

// Within a section it is still fill-and-overflow, so a section wider than a
// bay continues into the next one rather than being squeezed or dropped.
func TestASectionWiderThanABayContinuesIntoTheNext(t *testing.T) {
	var items []packItem
	for i := 0; i < 14; i++ {
		items = append(items, packItem{1, secDisplay})
	}
	units := packBySection(items, 12)
	if len(units) != 2 {
		t.Fatalf("got %d units for 14 slots in a 12-slot bay, want 2", len(units))
	}
	if len(units[0]) != 12 || len(units[1]) != 2 {
		t.Errorf("split %d/%d, want 12/2", len(units[0]), len(units[1]))
	}
	for _, idx := range units {
		if got := unitSection(items, idx); got != secDisplay {
			t.Errorf("a continued section reported bay %q", got)
		}
	}
}

// Nothing is lost and nothing is reordered, whatever the sections and
// widths — the same guarantee the unsectioned packer gives.
func TestSectionedPackingLosesNothingAndKeepsTheOrder(t *testing.T) {
	items := []packItem{
		{1, secConsole}, {5, secModel}, {2, secModel}, {0, secModel},
		{2, secDisplay}, {1, secDisplay}, {20, secDisplay}, {1, secOutput},
	}
	var flat []int
	for _, u := range packBySection(items, 12) {
		flat = append(flat, u...)
	}
	if len(flat) != len(items) {
		t.Fatalf("%d modules came out of %d", len(flat), len(items))
	}
	for i, v := range flat {
		if v != i {
			t.Errorf("position %d holds module %d — packing reordered the rack", i, v)
		}
	}
}

// A module too wide for any bay gets its own and overhangs, and must not
// drag the next section's modules in with it.
func TestAnOversizedModuleDoesNotSwallowTheNextSection(t *testing.T) {
	items := []packItem{{20, secModel}, {1, secDisplay}}
	units := packBySection(items, 12)
	if len(units) != 2 {
		t.Fatalf("got %d units, want the oversized one alone then the next section", len(units))
	}
	if len(units[0]) != 1 || units[0][0] != 0 {
		t.Errorf("first unit is %v, want just the oversized module", units[0])
	}
	if got := unitSection(items, units[1]); got != secDisplay {
		t.Errorf("second unit is bay %q, want %q", got, secDisplay)
	}
}

// Every section that modules are assigned to has a place in the stack and a
// name to silkscreen on it. A section with no rank sorts to the end
// silently; one with no title draws a blank label.
func TestEverySectionIsOrderedAndNamed(t *testing.T) {
	seen := map[string]bool{}
	for key, s := range moduleSections {
		seen[s] = true
		if sectionRank(s) >= len(sectionOrder) {
			t.Errorf("module %q is in section %q, which has no place in the stack", key, s)
		}
	}
	for _, s := range sectionOrder {
		if sectionTitle[s] == "" {
			t.Errorf("section %q has no title to silkscreen", s)
		}
	}
	// And every declared section is actually used, or it is a bay that will
	// never appear and a name nobody will see.
	for _, s := range sectionOrder {
		if !seen[s] {
			t.Errorf("section %q is declared and ordered but holds no modules", s)
		}
	}
}

// An unplaced module goes to the last bay, not the first: it is a mistake
// to notice, and it must not land in the middle of the signal path.
func TestAnUnplacedModuleGoesToTheEnd(t *testing.T) {
	got := moduleSection("something nobody has placed")
	if got != secUtility {
		t.Errorf("an unplaced module went to %q, want %q", got, secUtility)
	}
	if sectionRank(got) != len(sectionOrder)-1 {
		t.Errorf("%q is not the last bay in the stack", got)
	}
}

// The signal order is the reading order: in, measured, routed, generated,
// displayed, out. If this list is rearranged the rack stops telling you
// which way the signal goes.
func TestTheBaysAreStackedInSignalOrder(t *testing.T) {
	want := []string{secConsole, secInput, secAnalyze, secMod, secModel, secDisplay, secOutput, secUtility}
	if len(sectionOrder) != len(want) {
		t.Fatalf("got %d sections, want %d", len(sectionOrder), len(want))
	}
	for i := range want {
		if sectionOrder[i] != want[i] {
			t.Errorf("bay %d is %q, want %q", i, sectionOrder[i], want[i])
		}
	}
}

// A section appears in exactly one run, so it gets exactly one bay however
// the modules were declared or dragged. Grouping is what makes the label on
// a bay reliable: without it a module built at runtime lands wherever it
// was created and the rack grows a second bay with the same name.
func TestGroupingGivesEachSectionOneRun(t *testing.T) {
	// Deliberately interleaved, as the live rack was when the patchbay was
	// built after the parameters.
	in := []packItem{
		{1, secMod}, {2, secModel}, {1, secMod}, {1, secDisplay}, {2, secModel},
	}
	got := groupSectionsOnly(in)
	seenRun := map[string]int{}
	last := ""
	for _, it := range got {
		if it.Section != last {
			seenRun[it.Section]++
			last = it.Section
		}
	}
	for sec, runs := range seenRun {
		if runs != 1 {
			t.Errorf("section %q appears in %d runs, want 1", sec, runs)
		}
	}
	// And in signal order.
	for i := 1; i < len(got); i++ {
		if sectionRank(got[i-1].Section) > sectionRank(got[i].Section) {
			t.Errorf("out of signal order at %d: %q before %q",
				i, got[i-1].Section, got[i].Section)
		}
	}
	// Nothing lost.
	if len(got) != len(in) {
		t.Errorf("%d items in, %d out", len(in), len(got))
	}
}

// groupSectionsOnly is groupBySection's ordering, without the DOM half, so
// the rule can be tested on the host.
func groupSectionsOnly(items []packItem) []packItem {
	out := make([]packItem, 0, len(items))
	for _, sec := range sectionOrder {
		for _, it := range items {
			if it.Section == sec {
				out = append(out, it)
			}
		}
	}
	for _, it := range items {
		if sectionRank(it.Section) >= len(sectionOrder) {
			out = append(out, it)
		}
	}
	return out
}

// Capacity is a parameter because a bay's size is a property of the rack,
// not a constant of the packer — a narrower subrack is a real thing to
// build. A smaller bay must give more bays, not a fuller one.
func TestASmallerBayGivesMoreBays(t *testing.T) {
	var items []packItem
	for i := 0; i < 8; i++ {
		items = append(items, packItem{1, secDisplay})
	}
	wide := packBySection(items, 8)
	narrow := packBySection(items, 4)
	if len(wide) != 1 {
		t.Errorf("eight slots in an eight-slot bay took %d bays, want 1", len(wide))
	}
	if len(narrow) != 2 {
		t.Errorf("eight slots in a four-slot bay took %d bays, want 2", len(narrow))
	}
	for _, idx := range narrow {
		if len(idx) > 4 {
			t.Errorf("a four-slot bay holds %d one-slot modules", len(idx))
		}
	}
	// A degenerate capacity must not loop or lose anything.
	if got := packBySection(items, 0); len(got) != len(items) {
		t.Errorf("capacity 0 gave %d bays for %d modules, want one each", len(got), len(items))
	}
}
