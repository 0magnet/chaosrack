package racksurface

import "testing"

// items builds a run of modules of the given widths, all in one section.
func items(section string, widths ...int) []Item {
	out := make([]Item, len(widths))
	for i, w := range widths {
		out[i] = Item{Key: section, Slots: w, Section: section}
	}
	return out
}

func TestEveryModulePlacedOnce(t *testing.T) {
	it := append(items("a", 2, 2, 1, 2), items("b", 3, 1)...)
	s := Build(it, 6, nil, DefaultMetrics)
	if len(s.Panels) != len(it) {
		t.Fatalf("got %d panels for %d modules", len(s.Panels), len(it))
	}
	for i := range it {
		if s.PanelOf[i] < 0 {
			t.Errorf("module %d got no panel", i)
		}
	}
}

// A module's x must come from the slots before it, not from its position in
// the bay: the modules are different widths, and using the index was the bug
// that would put a 1-slot module and a 3-slot one at the same offset.
func TestPanelsSitAtTheirSlotNotTheirIndex(t *testing.T) {
	s := Build(items("a", 1, 3, 2), 6, nil, DefaultMetrics)
	want := []int{0, 1, 4} // slots
	for i, w := range want {
		p := s.Panels[s.PanelOf[i]]
		if p.Slot != w {
			t.Errorf("module %d at slot %d, want %d", i, p.Slot, w)
		}
		if p.X != w*s.Metrics.SlotCols {
			t.Errorf("module %d at x %d, want %d", i, p.X, w*s.Metrics.SlotCols)
		}
	}
}

func TestPanelsInABayDoNotOverlap(t *testing.T) {
	s := Build(items("a", 2, 1, 2, 1, 2, 1, 3, 2), 6, nil, DefaultMetrics)
	for _, b := range s.Bays {
		end := 0
		for _, pi := range b.Panels {
			p := s.Panels[pi]
			if p.X < end {
				t.Errorf("bay %d: panel at x %d overlaps the one ending at %d", b.Index, p.X, end)
			}
			end = p.X + p.W
		}
		if end > s.Cols {
			t.Errorf("bay %d runs to %d, past the surface at %d", b.Index, end, s.Cols)
		}
	}
}

// The width is the RACK's, not the content's: a half-empty bay is still a
// full-width bay, because the frame is that wide.
func TestSurfaceWidthIsTheRackNotTheContent(t *testing.T) {
	s := Build(items("a", 1), 14, nil, DefaultMetrics)
	if want := 14 * DefaultMetrics.SlotCols; s.Cols != want {
		t.Errorf("Cols = %d, want %d", s.Cols, want)
	}
}

func TestBaysStackWithoutOverlapping(t *testing.T) {
	s := Build(items("a", 4, 4, 4, 4, 4), 6, nil, DefaultMetrics)
	if len(s.Bays) < 3 {
		t.Fatalf("expected several bays, got %d", len(s.Bays))
	}
	for i, b := range s.Bays {
		if i > 0 {
			prev := s.Bays[i-1]
			if b.Y < prev.Y+prev.H {
				t.Errorf("bay %d starts at %d, inside bay %d ending at %d", i, b.Y, i-1, prev.Y+prev.H)
			}
		}
		if b.Y+b.H > s.Rows {
			t.Errorf("bay %d ends at %d, past the surface at %d", i, b.Y+b.H, s.Rows)
		}
	}
}

// A bay's monitor takes slots that no module owns, so the first panel has to
// start after them. Getting this wrong draws the first module over the screen.
func TestAMonitorPushesTheFirstPanelAlong(t *testing.T) {
	m := map[string]int{"a": 3}
	s := Build(items("a", 2, 2), 8, m, DefaultMetrics)
	p := s.Panels[s.PanelOf[0]]
	if p.Slot != 3 {
		t.Errorf("first panel at slot %d, want 3 (behind a 3-slot monitor)", p.Slot)
	}
}

// The section labels have to cover exactly the modules they name, or a bay
// holding three groups gets a label that is two-thirds wrong.
func TestRunSpansCoverTheirOwnModules(t *testing.T) {
	it := append(items("a", 2, 2), items("b", 2)...)
	s := Build(it, 6, nil, DefaultMetrics)
	b := s.Bays[0]
	if len(b.Runs) != 2 {
		t.Fatalf("expected 2 runs in the bay, got %d", len(b.Runs))
	}
	for i, r := range b.Runs {
		want := 0
		for n := r.From; n < r.From+r.Count; n++ {
			want += s.Panels[b.Panels[n]].W
		}
		if b.SpanW[i] != want {
			t.Errorf("run %q spans %d cells, want %d", r.Section, b.SpanW[i], want)
		}
		if b.SpanX[i] != s.Panels[b.Panels[r.From]].X {
			t.Errorf("run %q starts at %d, want %d", r.Section, b.SpanX[i], s.Panels[b.Panels[r.From]].X)
		}
	}
}

func TestBlankCountsTheEmptySlots(t *testing.T) {
	s := Build(items("a", 2, 2), 6, nil, DefaultMetrics) // one bay, 4 of 6 used
	slots, share := s.Blank()
	if slots != 2 {
		t.Errorf("blank slots = %d, want 2", slots)
	}
	if share < 0.33 || share > 0.34 {
		t.Errorf("blank share = %v, want about 1/3", share)
	}
}

func TestEmptySurfaceIsEmptyNotNegative(t *testing.T) {
	s := Build(nil, 6, nil, DefaultMetrics)
	if s.Rows != 0 || len(s.Bays) != 0 {
		t.Errorf("empty rack: Rows=%d bays=%d, want 0 and 0", s.Rows, len(s.Bays))
	}
	if _, share := s.Blank(); share != 0 {
		t.Errorf("empty rack blank share = %v, want 0", share)
	}
}

// A zero-slot module is one that has been switched out. It must not get a
// rectangle, and must not push its neighbors along.
func TestASwitchedOutModuleTakesNoRoom(t *testing.T) {
	s := Build(items("a", 2, 0, 2), 6, nil, DefaultMetrics)
	if got := s.Panels[s.PanelOf[2]].Slot; got != 2 {
		t.Errorf("the module after a switched-out one is at slot %d, want 2", got)
	}
	if got := s.Panels[s.PanelOf[1]].W; got != 0 {
		t.Errorf("a switched-out module is %d cells wide, want 0", got)
	}
}

// A bay takes the height of the tallest module in it: a shelf of chassis of
// different heights is as deep as the deepest one.
func TestABayIsAsTallAsItsTallestModule(t *testing.T) {
	it := items("a", 2, 2)
	it[1].Rows = 40
	s := Build(it, 6, nil, DefaultMetrics)
	if got := s.Bays[0].H; got != DefaultMetrics.HeadRows+40 {
		t.Errorf("bay height %d, want %d", got, DefaultMetrics.HeadRows+40)
	}
	for _, pi := range s.Bays[0].Panels {
		if s.Panels[pi].H != 40 {
			t.Errorf("panel height %d, want 40 — every panel in a bay is the bay's height", s.Panels[pi].H)
		}
	}
}

// And a module shorter than the metrics' default does not shrink the bay,
// which would let one small module pull a whole row out of alignment.
func TestAShortModuleDoesNotShrinkTheBay(t *testing.T) {
	it := items("a", 2)
	it[0].Rows = 3
	s := Build(it, 6, nil, DefaultMetrics)
	if got := s.Bays[0].H; got != DefaultMetrics.HeadRows+DefaultMetrics.PanelRows {
		t.Errorf("bay height %d, want the default %d", got, DefaultMetrics.HeadRows+DefaultMetrics.PanelRows)
	}
}
