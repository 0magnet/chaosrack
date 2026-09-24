package gentile

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// shape renders a packed module as the grid it will be built as, so a test
// can state the layout it wants rather than a list of coordinates.
//
//	aa
//	aa
//	bc
//
// is Chua's four constants over Thomas and Halvorsen.
func shape(m Module) string {
	cell := make([]byte, m.Cells())
	for i := range cell {
		cell[i] = '.'
	}
	for i, t := range m.Tiles {
		for p := t.Start; p < t.Start+t.N && p < len(cell); p++ {
			cell[p] = byte('a' + i)
		}
	}
	var rows []string
	for r := range modRows {
		rows = append(rows, string(cell[r*m.Cols:(r+1)*m.Cols]))
	}
	return strings.Join(rows, "\n")
}

func modeSet(m Module) string {
	var s []string
	for _, t := range m.Tiles {
		s = append(s, fmt.Sprintf("%s:%d", t.Mode, t.N))
	}
	return strings.Join(s, " ")
}

// The layout the user described: Chua's four constants arranged as a square,
// with Thomas and Halvorsen taking the two positions along the bottom.
func TestChuaThomasAndHalvorsenFillOnePanel(t *testing.T) {
	gens := []Spec{
		{Mode: "chua", Constants: 4},
		{Mode: "thomas", Constants: 1},
		{Mode: "halvorsen", Constants: 1},
	}
	mods := Pack(gens, 9)
	if len(mods) != 1 {
		t.Fatalf("want one module, got %d: %v", len(mods), mods)
	}
	want := "aa\naa\nbc"
	if got := shape(mods[0]); got != want {
		t.Errorf("layout:\n%s\nwant:\n%s", got, want)
	}
	if mods[0].Cells() != mods[0].Used() {
		t.Errorf("panel not full: %d of %d", mods[0].Used(), mods[0].Cells())
	}
}

// The combinations the rule is stated in terms of. Each fills its panel
// exactly, which is the whole reason for putting them together.
func TestStatedCombinationsFillTheirPanel(t *testing.T) {
	cases := []struct {
		counts []int
		want   string
	}{
		{[]int{4, 2}, "aa\naa\nbb"},
		{[]int{1, 2}, "a\nb\nb"},
		{[]int{1, 1, 1}, "a\nb\nc"},
		{[]int{5, 1}, "aa\naa\nab"},
		{[]int{2, 2, 2}, "aa\nbb\ncc"},
	}
	for _, c := range cases {
		var gens []Spec
		for i, n := range c.counts {
			gens = append(gens, Spec{Mode: fmt.Sprintf("m%d", i), Constants: n})
		}
		mods := Pack(gens, 9)
		if len(mods) != 1 {
			t.Errorf("%v: want one module, got %d", c.counts, len(mods))
			continue
		}
		if got := shape(mods[0]); got != c.want {
			t.Errorf("%v layout:\n%s\nwant:\n%s", c.counts, got, c.want)
		}
	}
}

// A generator that already fills its own columns has no room to share, so
// sharing a panel would only put two unrelated things on one unit.
func TestAGeneratorThatFillsItsColumnsStandsAlone(t *testing.T) {
	gens := []Spec{
		{Mode: "rossler", Constants: 3},
		{Mode: "lorenz", Constants: 3},
		{Mode: "aizawa", Constants: 6},
		{Mode: "chua", Constants: 4},
		{Mode: "thomas", Constants: 1},
		{Mode: "halvorsen", Constants: 1},
	}
	mods := Pack(gens, 9)
	alone := map[string]bool{}
	for _, m := range mods {
		if len(m.Tiles) == 1 {
			alone[m.Tiles[0].Mode] = true
		}
		for _, tl := range m.Tiles {
			if tl.N%modRows == 0 && len(m.Tiles) > 1 {
				t.Errorf("%s fills %d columns and should not share: %s",
					tl.Mode, tl.N/modRows, modeSet(m))
			}
		}
	}
	for _, want := range []string{"rossler", "lorenz", "aizawa"} {
		if !alone[want] {
			t.Errorf("%s should have a panel of its own", want)
		}
	}
}

// Nothing is lost, nothing is duplicated, and the catalog order survives.
func TestPackingKeepsEveryGeneratorOnce(t *testing.T) {
	gens := []Spec{
		{Mode: "a", Constants: 2}, {Mode: "b", Constants: 5}, {Mode: "c", Constants: 0},
		{Mode: "d", Constants: 3}, {Mode: "e", Constants: 1}, {Mode: "f", Constants: 4},
		{Mode: "g", Constants: 2}, {Mode: "h", Constants: 7},
	}
	seen := map[string]int{}
	for _, m := range Pack(gens, 9) {
		for _, tl := range m.Tiles {
			seen[tl.Mode]++
		}
	}
	for _, g := range gens {
		want := 1
		if g.Constants == 0 {
			want = 0 // nothing to mount
		}
		if seen[g.Mode] != want {
			t.Errorf("%s appears %d times, want %d", g.Mode, seen[g.Mode], want)
		}
	}
	// Within a panel the generators are in catalog order, and the panels
	// themselves are ordered by the first generator on each. Across panels
	// the order can step back — grouping is allowed to reach past a
	// neighbor to fill a panel, which is the whole point of it.
	for _, m := range Pack(gens, 9) {
		var on []string
		for _, tl := range m.Tiles {
			on = append(on, tl.Mode)
		}
		sorted := append([]string(nil), on...)
		sort.Strings(sorted)
		if strings.Join(on, "") != strings.Join(sorted, "") {
			t.Errorf("panel out of catalog order: %v", on)
		}
	}
}

// Runs stay inside the panel, do not overlap, and carry exactly as many
// positions as the generator has constants.
func TestRunsAreContiguousAndDoNotOverlap(t *testing.T) {
	gens := []Spec{
		{Mode: "a", Constants: 5}, {Mode: "b", Constants: 1}, {Mode: "c", Constants: 2},
		{Mode: "d", Constants: 4}, {Mode: "e", Constants: 2}, {Mode: "f", Constants: 8},
		{Mode: "g", Constants: 1}, {Mode: "h", Constants: 2}, {Mode: "i", Constants: 7},
	}
	for _, m := range Pack(gens, 9) {
		if len(m.Tiles) > maxGensPerModule {
			t.Errorf("%d generators on one panel: %s", len(m.Tiles), modeSet(m))
		}
		taken := map[int]string{}
		for _, tl := range m.Tiles {
			if tl.N <= 0 {
				t.Errorf("%s has no positions", tl.Mode)
			}
			for p := tl.Start; p < tl.Start+tl.N; p++ {
				if p < 0 || p >= m.Cells() {
					t.Errorf("%s reaches position %d of a %d-position panel", tl.Mode, p, m.Cells())
					continue
				}
				if other, dup := taken[p]; dup {
					t.Errorf("%s and %s both claim position %d", other, tl.Mode, p)
				}
				taken[p] = tl.Mode
			}
		}
	}
}

// Blank panel is the thing this is for. Whatever the counts, a shared panel
// wastes no more than the arithmetic forces: the total that cannot be made
// up into whole columns.
func TestWasteIsTheLeastTheCountsAllow(t *testing.T) {
	cases := [][]int{
		{4, 2, 1, 1, 5, 2, 2, 2, 4}, // the attractors, less the ones that stand alone
		{2, 1, 4, 4, 4, 1},          // the maps
		{7, 7, 4},                   // the audio displays
		{1, 1},
		{2},
	}
	for _, counts := range cases {
		var gens []Spec
		total := 0
		for i, n := range counts {
			gens = append(gens, Spec{Mode: fmt.Sprintf("m%d", i), Constants: n})
			total += n
		}
		waste := 0
		for _, m := range Pack(gens, 9) {
			waste += m.Cells() - m.Used()
		}
		if want := (modRows - total%modRows) % modRows; waste != want {
			t.Errorf("%v: %d positions blank, %d is the least the counts allow", counts, waste, want)
		}
	}
}

// A panel cannot be wider than the bay it is mounted in — unless one
// generator on its own is, in which case it overhangs, which is visible and
// therefore the right thing for something that genuinely does not fit.
func TestAPanelFitsItsBay(t *testing.T) {
	gens := []Spec{
		{Mode: "small", Constants: 2}, {Mode: "huge", Constants: 40}, {Mode: "mid", Constants: 4},
	}
	for _, m := range Pack(gens, 5) {
		if m.Cols > 5 && len(m.Tiles) > 1 {
			t.Errorf("%d columns will not go in a 5-column bay: %s", m.Cols, modeSet(m))
		}
	}
}

// A category of one-knob models is the case that started this: nineteen of
// Sprott's twenty have a step and nothing else, and on a panel each they
// would be nineteen units a knob wide.
func TestSingleKnobGeneratorsShareAColumn(t *testing.T) {
	var gens []Spec
	for i := range 5 {
		gens = append(gens, Spec{Mode: fmt.Sprintf("seed%d", i), Constants: 1})
	}
	mods := Pack(gens, 9)
	if len(mods) != 2 {
		t.Fatalf("five one-knob generators want two panels, got %d", len(mods))
	}
	// The blank position lands on the LAST panel, not the first.
	if got := shape(mods[0]); got != "a\nb\nc" {
		t.Errorf("first panel should be full:\n%s", got)
	}
	if got := shape(mods[1]); got != "a\nb\n." {
		t.Errorf("last panel:\n%s", got)
	}
}
