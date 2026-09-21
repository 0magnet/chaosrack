package attractor

import "testing"

// The worked example: Chua's four constants are a square, and the two
// one-constant systems take the positions along the bottom beneath it. One
// module, three generators, six control positions, no blank panel.
func TestChuaThomasAndHalvorsenShareOnePanel(t *testing.T) {
	mods := packGenerators([]genSpec{
		{"chua", 4}, {"thomas", 1}, {"halvorsen", 1},
	}, 12)
	if len(mods) != 1 {
		t.Fatalf("three generators took %d modules, want 1: %+v", len(mods), mods)
	}
	want := []genTile{
		{"chua", 0, 0, 2, 2},
		{"thomas", 0, 2, 1, 1},
		{"halvorsen", 1, 2, 1, 1},
	}
	for i, w := range want {
		if mods[0][i] != w {
			t.Errorf("tile %d is %+v, want %+v", i, mods[0][i], w)
		}
	}
	if got := moduleCols(mods[0]); got != 2 {
		t.Errorf("the module is %d columns, want 2", got)
	}
}

// A module carries three generators and no more, however small they are.
// The limit is a statement about the hardware: a unit with eight generators
// on it is not a module.
func TestAModuleCarriesAtMostThreeGenerators(t *testing.T) {
	var gens []genSpec
	for i := 0; i < 9; i++ {
		gens = append(gens, genSpec{"g", 1})
	}
	mods := packGenerators(gens, 12)
	if len(mods) != 3 {
		t.Fatalf("nine one-knob generators took %d modules, want 3", len(mods))
	}
	for i, m := range mods {
		if len(m) != maxGensPerModule {
			t.Errorf("module %d carries %d generators, want %d", i, len(m), maxGensPerModule)
		}
	}
}

// Nothing overlaps, nothing escapes the panel, and nothing is lost. The
// invariant that matters: two generators must never be given the same
// control position.
func TestTilesNeverOverlapOrEscape(t *testing.T) {
	gens := []genSpec{
		{"a", 3}, {"b", 1}, {"c", 2}, {"d", 4}, {"e", 6}, {"f", 1},
		{"g", 5}, {"h", 2}, {"i", 1}, {"j", 7}, {"k", 3}, {"l", 1},
	}
	mods := packGenerators(gens, 12)
	seen := 0
	for mi, m := range mods {
		used := map[[2]int]string{}
		for _, tile := range m {
			seen++
			if tile.Row < 0 || tile.Row+tile.H > modRows {
				t.Errorf("module %d: %s spans rows %d..%d, panel has %d",
					mi, tile.Mode, tile.Row, tile.Row+tile.H-1, modRows)
			}
			for dc := 0; dc < tile.W; dc++ {
				for dr := 0; dr < tile.H; dr++ {
					p := [2]int{tile.Col + dc, tile.Row + dr}
					if prev, dup := used[p]; dup {
						t.Errorf("module %d: %s and %s both hold position %v",
							mi, prev, tile.Mode, p)
					}
					used[p] = tile.Mode
				}
			}
		}
	}
	if seen != len(gens) {
		t.Errorf("%d generators went in, %d came out", len(gens), seen)
	}
}

// Order is preserved: the rack reads in the order the catalog lists, so a
// model cannot move because of how its neighbors happened to pack.
func TestPackingKeepsCatalogOrder(t *testing.T) {
	gens := []genSpec{{"a", 4}, {"b", 1}, {"c", 1}, {"d", 3}, {"e", 2}, {"f", 6}}
	var got []string
	for _, m := range packGenerators(gens, 12) {
		for _, tile := range m {
			got = append(got, tile.Mode)
		}
	}
	for i, g := range gens {
		if got[i] != g.Mode {
			t.Errorf("position %d is %q, want %q — packing reordered the rack", i, got[i], g.Mode)
		}
	}
}

// A generator with no constants of its own does not take a panel position.
// Its step size and its selector live on the bay's head.
func TestAGeneratorWithNoConstantsTakesNoSpace(t *testing.T) {
	mods := packGenerators([]genSpec{{"a", 0}, {"b", 2}, {"c", 0}}, 12)
	if len(mods) != 1 || len(mods[0]) != 1 || mods[0][0].Mode != "b" {
		t.Fatalf("got %+v, want just b", mods)
	}
}

// The shapes are as square as the count allows, because a block of
// constants reads at once and a line of four does not.
func TestTheShapeOfAGeneratorIsAsSquareAsItCanBe(t *testing.T) {
	for _, c := range []struct{ n, w, h int }{
		{1, 1, 1}, {2, 1, 2}, {3, 1, 3}, {4, 2, 2}, {5, 2, 3}, {6, 2, 3},
		{7, 3, 3}, {9, 3, 3}, {10, 4, 3},
	} {
		w, h := tileShape(c.n)
		if w != c.w || h != c.h {
			t.Errorf("%d constants shape %dx%d, want %dx%d", c.n, w, h, c.w, c.h)
		}
		if w*h < c.n {
			t.Errorf("%d constants do not fit in %dx%d", c.n, w, h)
		}
		if h > modRows {
			t.Errorf("%d constants want %d rows, a panel has %d", c.n, h, modRows)
		}
	}
}

// A module never grows wider than a bay, because a module wider than a bay
// cannot be put in one.
func TestAModuleNeverOutgrowsABay(t *testing.T) {
	var gens []genSpec
	for i := 0; i < 12; i++ {
		gens = append(gens, genSpec{"g", 9})
	}
	for _, m := range packGenerators(gens, 4) {
		if got := moduleCols(m); got > 4 {
			t.Errorf("a module came out %d columns wide, bay holds 4", got)
		}
	}
}
