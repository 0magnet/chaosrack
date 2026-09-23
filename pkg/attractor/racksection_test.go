package attractor

import "testing"

// Within a section it is still fill-and-overflow, so a section wider than a
// bay continues into the next one rather than being squeezed or dropped.
func TestASectionWiderThanABayContinuesIntoTheNext(t *testing.T) {
	var items []packItem
	for i := 0; i < 14; i++ {
		items = append(items, packItem{Slots: 1, Section: secDisplay})
	}
	units := packBySection(items, 12, nil)
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
		{Slots: 1, Section: secConsole}, {Slots: 5, Section: secModel}, {Slots: 2, Section: secModel}, {Slots: 0, Section: secModel},
		{Slots: 2, Section: secDisplay}, {Slots: 1, Section: secDisplay}, {Slots: 20, Section: secDisplay}, {Slots: 1, Section: secOutput},
	}
	var flat []int
	for _, u := range packBySection(items, 12, nil) {
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
	items := []packItem{{Slots: 20, Section: secModel}, {Slots: 1, Section: secDisplay}}
	units := packBySection(items, 12, nil)
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
		if sectionTitleOf(s) == "" {
			t.Errorf("section %q has no title to silkscreen", s)
		}
	}
	// And every declared section is actually used, or it is a bay that will
	// never appear and a name nobody will see. A model row is used by its
	// own category module, which is generated rather than listed in
	// moduleSections, so it is matched by name instead.
	for _, s := range sectionOrder {
		if seen[s] || isCategorySection(s) {
			continue
		}
		t.Errorf("section %q is declared and ordered but holds no modules", s)
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
	// The fixed sections keep their order, with the model rows spliced in
	// where the models go — so a category row is never above the input bay
	// or below the output one.
	var fixed []string
	for _, s := range sectionOrder {
		if !isCategorySection(s) {
			fixed = append(fixed, s)
		}
	}
	want := []string{secConsole, secInput, secAnalyze, secMod, secModel, secDisplay, secOutput, secUtility}
	if len(fixed) != len(want) {
		t.Fatalf("got %d fixed sections, want %d: %v", len(fixed), len(want), fixed)
	}
	for i := range want {
		if fixed[i] != want[i] {
			t.Errorf("fixed bay %d is %q, want %q", i, fixed[i], want[i])
		}
	}
}

// The model rows sit together, in the selector's own order, between the
// modulation bay and the models' own section.
func TestTheModelRowsAreSplicedInSelectorOrder(t *testing.T) {
	var cats []string
	first, lastIdx := -1, -1
	for i, s := range sectionOrder {
		if !isCategorySection(s) {
			continue
		}
		if first < 0 {
			first = i
		}
		lastIdx = i
		cats = append(cats, s)
	}
	if len(cats) != len(modeGroups) {
		t.Fatalf("%d model rows for %d categories", len(cats), len(modeGroups))
	}
	if lastIdx-first != len(cats)-1 {
		t.Errorf("the model rows are not contiguous: first %d last %d for %d rows", first, lastIdx, len(cats))
	}
	for i, c := range modelCategories() {
		if cats[i] != categorySection(c) {
			t.Errorf("model row %d is %q, want %q", i, cats[i], categorySection(c))
		}
	}
	if sectionRank(secMod) > first {
		t.Error("a model row sits above the modulation bay")
	}
	if sectionRank(secDisplay) < lastIdx {
		t.Error("a model row sits below the display bay")
	}
}

// A bay opens with its head, so the monitor it carries is the leftmost
// thing in the row it is the monitor for — here, even though the row would
// have fitted in the tail of the bay before it.
//
// It used to start one only when the row would not otherwise have fitted,
// which is the ordinary keep-together rule and reads perfectly well — but it
// put the monitor wherever the row happened to begin. Measured on the rack,
// two of eleven monitors led their bay and nine sat somewhere in the middle
// of the row above their own.
func TestABayOpensWithItsHead(t *testing.T) {
	cat := categorySection("Attractors")
	// Three slots of another section, then a row of six. It would fit in
	// what is left of the bay — that is the point.
	items := []packItem{
		{Slots: 3, Section: secMod},
		{Slots: 1, Section: cat, Lead: true}, {Slots: 2, Section: cat}, {Slots: 3, Section: cat},
	}
	units := packBySection(items, 12, nil)
	if len(units) != 2 {
		t.Fatalf("nine slots took %d bays, want the head to have opened a second: %v", len(units), units)
	}
	if units[1][0] != 1 {
		t.Errorf("the bay does not begin with its head: %v", units)
	}
}

// Only a head breaks. A category module that is not one — a card, or a row
// continuing into a bay it does not head — packs like anything else, or
// every card in the rack would be a row of its own.
func TestAModuleThatIsNotAHeadDoesNotBreak(t *testing.T) {
	cat := categorySection("Attractors")
	items := []packItem{
		{Slots: 3, Section: secMod},
		{Slots: 1, Section: cat}, {Slots: 2, Section: cat}, {Slots: 3, Section: cat},
	}
	if got := packBySection(items, 12, nil); len(got) != 1 {
		t.Errorf("nine slots with no head took %d bays, want 1: %v", len(got), got)
	}
}

// A head that is switched OUT breaks nothing. It takes no slots, so a bay
// opened for it would be a row of twelve blank panels.
func TestAHeadThatIsSwitchedOutBreaksNothing(t *testing.T) {
	cat := categorySection("Attractors")
	items := []packItem{
		{Slots: 3, Section: secMod},
		{Slots: 0, Section: cat, Lead: true}, {Slots: 2, Section: cat},
	}
	if got := packBySection(items, 12, nil); len(got) != 1 {
		t.Errorf("a head that is not in the rack opened %d bays: %v", len(got), got)
	}
}

// The row is not split across bays, which is what the head is dividing the
// category into bay-sized groups FOR: each head is followed by exactly the
// generators that fit beside it.
func TestTheModelRowIsNeverSplitAcrossBays(t *testing.T) {
	cat := categorySection("Attractors")
	// Nine slots of another section first, so the row could not have fitted
	// in what is left of the bay whatever the rule.
	items := []packItem{
		{Slots: 5, Section: secMod}, {Slots: 4, Section: secMod},
		{Slots: 1, Section: cat, Lead: true}, {Slots: 2, Section: cat}, {Slots: 3, Section: cat},
		{Slots: 1, Section: secDisplay},
	}
	units := packBySection(items, 12, nil)
	bays, total := 0, 0
	for _, u := range units {
		n := 0
		for _, i := range u {
			if items[i].Section == cat {
				n++
			}
		}
		if n > 0 {
			bays++
			total += n
		}
	}
	if bays != 1 {
		t.Errorf("the model row is spread over %d bays: %v", bays, units)
	}
	if total != 3 {
		t.Errorf("%d of the row's 3 modules were placed: %v", total, units)
	}
}

// Category sections cost nothing when they hold nothing. Every category has
// a place in the stack whether or not the model running is in it, and an
// empty one must not open a bay, a break, or a label.
func TestEmptyCategorySectionsCostNothing(t *testing.T) {
	plain := []packItem{
		{Slots: 3, Section: secMod}, {Slots: 3, Section: secDisplay}, {Slots: 3, Section: secOutput},
	}
	base := packBySection(plain, 12, nil)
	if len(base) != 1 {
		t.Fatalf("nine slots took %d bays: %v", len(base), base)
	}
	// The same rack with every category section declared but unfilled packs
	// identically — there is nothing to declare, since an empty section
	// contributes no items at all.
	for _, c := range modelCategories() {
		if sectionRank(categorySection(c)) >= len(sectionOrder) {
			t.Errorf("category %q has no place in the stack", c)
		}
	}
	if got := packBySection(plain, 12, nil); len(got) != len(base) {
		t.Errorf("packing is not stable: %d bays then %d", len(base), len(got))
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
		{Slots: 1, Section: secMod}, {Slots: 2, Section: secModel}, {Slots: 1, Section: secMod}, {Slots: 1, Section: secDisplay}, {Slots: 2, Section: secModel},
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
		items = append(items, packItem{Slots: 1, Section: secDisplay})
	}
	wide := packBySection(items, 8, nil)
	narrow := packBySection(items, 4, nil)
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
	if got := packBySection(items, 0, nil); len(got) != len(items) {
		t.Errorf("capacity 0 gave %d bays for %d modules, want one each", len(got), len(items))
	}
}

// A bay carries as many sections as fit. Breaking at every section made
// the labels easy and the rack 58% blank panel; several groups sharing an
// 84 HP row is what a real one looks like.
func TestABayCarriesSeveralSections(t *testing.T) {
	items := []packItem{
		{Slots: 1, Section: secInput},
		{Slots: 2, Section: secAnalyze}, {Slots: 2, Section: secAnalyze},
		{Slots: 1, Section: secOutput},
	}
	units := packBySection(items, 12, nil)
	if len(units) != 1 {
		t.Fatalf("six slots in three sections took %d bays, want 1: %v", len(units), units)
	}
	runs := sectionRuns(items, units[0])
	if len(runs) != 3 {
		t.Fatalf("got %d runs in the bay, want 3: %+v", len(runs), runs)
	}
	for i, want := range []sectionRun{
		{Section: secInput, From: 0, Count: 1}, {Section: secAnalyze, From: 1, Count: 2}, {Section: secOutput, From: 3, Count: 1},
	} {
		if runs[i] != want {
			t.Errorf("run %d is %+v, want %+v", i, runs[i], want)
		}
	}
}

// The runs cover every module in the bay exactly once, or a label is
// missing from part of the row.
func TestTheRunsCoverTheWholeBay(t *testing.T) {
	items := []packItem{
		{Slots: 1, Section: secInput}, {Slots: 1, Section: secAnalyze}, {Slots: 1, Section: secAnalyze}, {Slots: 1, Section: secMod}, {Slots: 1, Section: secOutput},
	}
	units := packBySection(items, 12, nil)
	for _, idx := range units {
		covered := 0
		for _, r := range sectionRuns(items, idx) {
			if r.From != covered {
				t.Errorf("a run starts at %d, want %d — there is a gap or an overlap", r.From, covered)
			}
			covered += r.Count
		}
		if covered != len(idx) {
			t.Errorf("runs cover %d modules of %d in the bay", covered, len(idx))
		}
	}
}

// A section still never interleaves: grouping has made it contiguous, so
// each one appears in exactly one run across the whole rack.
func TestASectionStillAppearsOnce(t *testing.T) {
	items := groupSectionsOnly([]packItem{
		{Slots: 1, Section: secMod}, {Slots: 2, Section: secModel}, {Slots: 1, Section: secMod}, {Slots: 1, Section: secDisplay}, {Slots: 2, Section: secModel},
	})
	seen := map[string]int{}
	for _, idx := range packBySection(items, 12, nil) {
		for _, r := range sectionRuns(items, idx) {
			seen[r.Section]++
		}
	}
	for sec, n := range seen {
		if n != 1 {
			t.Errorf("section %q appears in %d runs, want 1", sec, n)
		}
	}
}

// The model's own panels follow the model into its category's row. That is
// the whole point of the rows: the knobs that tune a model sit in the same
// 84 HP as the knob that chose it, instead of in a GENERATOR bay somewhere
// else in the rack.
func TestTheModelsPanelsFollowTheModel(t *testing.T) {
	prev := activeCategory
	t.Cleanup(func() { activeCategory = prev })

	setActiveCategory("lorenz")
	if activeCategory == "" {
		t.Fatal("lorenz is in no category, so the rows cannot hold anything")
	}
	want := categorySection(activeCategory)
	for _, key := range []string{"parameters", "patch", "banner", "animation"} {
		if got := moduleSection(key); got != want {
			t.Errorf("%q is in %q, want the running model's row %q", key, got, want)
		}
	}
	// A model nobody filed leaves the panels where they were: a bay of their
	// own is a worse place than the right row, and a far better one than
	// UTILITY beside the presets.
	setActiveCategory("a model that is in no category")
	if got := moduleSection("parameters"); got != secModel {
		t.Errorf("with no active category Parameters went to %q, want %q", got, secModel)
	}
	// And nothing else moves with them.
	if got := moduleSection("patchbay"); got != secMod {
		t.Errorf("the patchbay followed the model to %q; it is rack wiring, not a model panel", got)
	}
}

// What a leading head costs, stated: the bay before it ends where the head
// begins, however much room was left in it.
//
// This is a deliberate trade and the old rule made the opposite one — it
// let a row share the tail of the bay before it, and paid for that with a
// monitor sitting in the middle of somebody else's row. A rack is read down
// its left edge; one display per row there is worth a few blank slots.
func TestALeadingHeadEndsTheBayBeforeIt(t *testing.T) {
	small := categorySection("Solids")
	big := categorySection("Audio")
	items := []packItem{
		{Slots: 2, Section: small, Lead: true}, {Slots: 1, Section: small}, // monitor and rotary, no parameters
		{Slots: 6, Section: big, Lead: true}, {Slots: 6, Section: big}, {Slots: 6, Section: big}, // eighteen, wider than any bay
	}
	units := packBySection(items, 12, nil)
	if len(units) != 3 {
		t.Fatalf("got %d bays, want the two heads to open two of them: %v", len(units), units)
	}
	if len(units[0]) != 2 {
		t.Errorf("the small row did not get the bay to itself: %v", units)
	}
	// A row wider than a bay still continues into the next one, and the
	// continuation is not a head, so it does not open a third break of its
	// own beyond the one overflow forces.
	if units[1][0] != 2 || len(units[1]) != 2 {
		t.Errorf("the big row's head did not open a bay and fill it: %v", units)
	}
}

// Two small rows share a bay, each opening with its own screen. That is the
// whole point of letting a bay carry several sections, and the reason the
// break rule is "a bay begins with a head" rather than "a head begins a
// bay": the second costs four rows of blank panel to say the same thing.
func TestSmallRowsShareABayEachBehindItsOwnHead(t *testing.T) {
	a := categorySection("Polyhedra")
	b := categorySection("Geometry")
	items := []packItem{
		{Slots: 3, Section: a, Lead: true},
		{Slots: 2, Section: b, Lead: true}, {Slots: 4, Section: b}, {Slots: 1, Section: b},
	}
	units := packBySection(items, 12, nil)
	if len(units) != 1 {
		t.Fatalf("ten slots took %d bays, want 1: %v", len(units), units)
	}
	if len(units[0]) != 4 {
		t.Errorf("the two rows did not share: %v", units)
	}
}

// Every bay that holds a head opens with one. The invariant the rule is
// for, checked against an arrangement rather than a hand-made case.
func TestABayHoldingAHeadOpensWithOne(t *testing.T) {
	cats := modelCategories()
	if len(cats) < 4 {
		t.Skip("not enough categories to arrange")
	}
	var items []packItem
	items = append(items, packItem{Slots: 4, Section: secAnalyze})
	for i, c := range cats {
		sec := categorySection(c)
		items = append(items, packItem{Slots: 2, Section: sec, Lead: true})
		for n := 0; n < i%5; n++ {
			items = append(items, packItem{Slots: 1 + n%3, Section: sec})
		}
	}
	items = append(items, packItem{Slots: 3, Section: secOutput})
	for _, u := range packBySection(items, 12, nil) {
		held := false
		for _, i := range u {
			if items[i].Lead {
				held = true
			}
		}
		if held && !items[u[0]].Lead {
			t.Errorf("a bay holding a head opens with %+v instead: %v", items[u[0]], u)
		}
	}
}

// ── A bay is a chassis: it owns a monitor, and the monitor is at its left ──

// baysOf is the sections each bay carries, in order, for the assertions below.
func baysOf(items []packItem, units [][]int) []string {
	out := make([]string, 0, len(units))
	for _, u := range units {
		out = append(out, unitSection(items, u))
	}
	return out
}

func TestABaysMonitorIsChargedAgainstItsWidth(t *testing.T) {
	// Four 3-slot modules and a 12-slot bay fit in one row. Give the section
	// a 3-slot monitor and one of them has to move: the monitor is part of
	// the bay, not something the bay finds room for afterwards.
	items := []packItem{
		{Slots: 3, Section: "a"}, {Slots: 3, Section: "a"},
		{Slots: 3, Section: "a"}, {Slots: 3, Section: "a"},
	}
	if got := len(packBySection(items, 12, nil)); got != 1 {
		t.Fatalf("with no monitor the four fit in %d bays, want 1", got)
	}
	units := packBySection(items, 12, map[string]int{"a": 3})
	if len(units) != 2 {
		t.Fatalf("with a 3-slot monitor they took %d bays, want 2", len(units))
	}
	if len(units[0]) != 3 {
		t.Fatalf("first bay holds %d modules, want 3 — the monitor takes the fourth's room", len(units[0]))
	}
}

func TestEveryBayOfASectionGetsItsOwnMonitor(t *testing.T) {
	// A section wide enough for three bays is three chassis, each with a
	// screen at its left — which is what the category rows already do and the
	// reason they read as instruments rather than as a shelf.
	var items []packItem
	for i := 0; i < 9; i++ {
		items = append(items, packItem{Slots: 3, Section: "a"})
	}
	units := packBySection(items, 12, map[string]int{"a": 3})
	if len(units) != 3 {
		t.Fatalf("nine 3-slot modules behind a 3-slot monitor took %d bays, want 3", len(units))
	}
	for i, u := range units {
		if len(u) != 3 {
			t.Fatalf("bay %d holds %d modules, want 3 — every bay pays for its own monitor", i, len(u))
		}
	}
}

func TestTheMonitorBelongsToWhicheverSectionOpensTheBay(t *testing.T) {
	// A bay carries one screen. Sections that come to share the row behind it
	// plug into that one rather than each bringing another, or a row of three
	// small sections would be three screens and no room.
	items := []packItem{
		{Slots: 2, Section: "a"},
		{Slots: 2, Section: "b"},
		{Slots: 2, Section: "c"},
	}
	units := packBySection(items, 12, map[string]int{"a": 3, "b": 3, "c": 3})
	if len(units) != 1 {
		t.Fatalf("three small sections took %d bays, want 1 — only the section that opens the bay is charged", len(units))
	}
	if got := baysOf(items, units); got[0] != "a" {
		t.Fatalf("the bay reads as section %q, want the one that opened it", got[0])
	}
}

func TestASectionWithNoMonitorPacksExactlyAsBefore(t *testing.T) {
	// The table is filled one section at a time, so a section that has no
	// monitor built yet must be untouched by the change.
	items := []packItem{
		{Slots: 4, Section: "a"}, {Slots: 4, Section: "a"}, {Slots: 4, Section: "a"},
		{Slots: 4, Section: "b"}, {Slots: 4, Section: "b"},
	}
	want := packBySection(items, 12, nil)
	got := packBySection(items, 12, map[string]int{"z": 6}) // a section not present
	if len(got) != len(want) {
		t.Fatalf("an unrelated monitor changed the packing: %d bays, want %d", len(got), len(want))
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("bay %d holds %d, want %d", i, len(got[i]), len(want[i]))
		}
	}
}

func TestAMonitorWiderThanTheBayStillLeavesRoomToPlugInto(t *testing.T) {
	// A screen with a caption is not a rack. Clamped rather than refused, so
	// a mis-declared width degrades to a cramped row and not to an empty one.
	items := []packItem{{Slots: 1, Section: "a"}, {Slots: 1, Section: "a"}}
	units := packBySection(items, 6, map[string]int{"a": 99})
	if len(units) == 0 {
		t.Fatal("an over-wide monitor packed nothing at all")
	}
	n := 0
	for _, u := range units {
		n += len(u)
	}
	if n != len(items) {
		t.Fatalf("packed %d of %d modules — an over-wide monitor lost some", n, len(items))
	}
}

func TestEveryDeclaredBayMonitorNamesARealSection(t *testing.T) {
	// The table is keyed by section, and a key that is not one is a monitor
	// nobody gets: the packer would charge nothing, the bay would open with a
	// knob at its left, and nothing would say so. Same guard the module table
	// has, for the same reason.
	for section, slots := range bayMonitorSlots {
		if slots <= 0 {
			t.Errorf("section %q declares a monitor of %d slots", section, slots)
		}
		found := false
		for _, s := range sectionOrder {
			if s == section {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("bayMonitorSlots names %q, which is not a section — see sectionOrder", section)
		}
	}
}
