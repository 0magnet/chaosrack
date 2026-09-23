package racktui

import (
	"testing"

	"github.com/0magnet/chaosrack/pkg/controlspec"
	"github.com/0magnet/chaosrack/pkg/racksurface"
)

// buf is a Painter that remembers what was drawn, so the layout can be tested
// without a terminal. That is the whole reason DrawSurface takes a Painter.
type buf struct {
	w, h  int
	cells map[[2]int]cellAt
	out   []int // out-of-bounds writes, which must not happen
}

func newBuf(w, h int) *buf { return &buf{w: w, h: h, cells: map[[2]int]cellAt{}} }

func (b *buf) Set(x, y int, c cellAt) {
	if x < 0 || y < 0 || x >= b.w || y >= b.h {
		b.out = append(b.out, x, y)
		return
	}
	b.cells[[2]int{x, y}] = c
}

func (b *buf) art() int {
	n := 0
	for _, c := range b.cells {
		if c.Art {
			n++
		}
	}
	return n
}

func demoSurface(nmods, slots int) (racksurface.Surface, []moduleCtls) {
	var items []racksurface.Item
	var ctls []Control
	for i := 0; i < nmods; i++ {
		k := string(rune('a' + i))
		items = append(items, racksurface.Item{
			Key: k, Title: k, Slots: slots, Section: "s",
			Rows: ModuleRows(3, slots, racksurface.DefaultMetrics.SlotCols),
		})
		for j := 0; j < 3; j++ {
			ctls = append(ctls, Control{
				ControlInfo: controlspec.ControlInfo{
					ID: k + string(rune('0'+j)), Label: "L", Module: k, Min: 0, Max: 10,
				},
				Value: "5",
			})
		}
	}
	s := racksurface.Build(items, 6, nil, racksurface.DefaultMetrics)
	return s, attach(items, ctls)
}

// Nothing may be painted outside the window. A renderer that writes past the
// edge corrupts whatever the host drew there — and in a shell that is the
// scrollback.
func TestDrawingStaysInsideTheWindow(t *testing.T) {
	s, mods := demoSurface(8, 2)
	b := newBuf(40, 12)
	DrawSurface(b, s, racksurface.View{X: 0, Y: 0, W: 40, H: 12}, mods, ctlAt{Module: -1})
	if len(b.out) > 0 {
		t.Errorf("%d writes landed outside the window: %v", len(b.out)/2, b.out[:mini(len(b.out), 12)])
	}
}

// And nothing may be painted outside it when the window is part-way down a
// surface much bigger than it, which is the case the offsets get wrong.
func TestDrawingStaysInsideAScrolledWindow(t *testing.T) {
	s, mods := demoSurface(12, 2)
	b := newBuf(37, 11)
	DrawSurface(b, s, racksurface.View{X: 9, Y: 17, W: 37, H: 11}, mods, ctlAt{Module: -1})
	if len(b.out) > 0 {
		t.Errorf("%d writes landed outside the scrolled window", len(b.out)/2)
	}
}

// The knobs have to actually be drawn: half-block art cells, not text.
func TestControlsAreDrawnAsArt(t *testing.T) {
	s, mods := demoSurface(2, 2)
	b := newBuf(80, 40)
	DrawSurface(b, s, racksurface.View{X: 0, Y: 0, W: 80, H: 40}, mods, ctlAt{Module: -1})
	if b.art() == 0 {
		t.Error("no half-block cells were drawn: the dials are missing")
	}
}

// A surface far larger than the window must cost only what is on screen.
func TestOffscreenBaysAreNotDrawn(t *testing.T) {
	s, mods := demoSurface(60, 2)
	small := newBuf(40, 10)
	DrawSurface(small, s, racksurface.View{X: 0, Y: 0, W: 40, H: 10}, mods, ctlAt{Module: -1})
	if len(small.cells) > 40*10 {
		t.Errorf("%d cells written into a 400-cell window", len(small.cells))
	}
	if s.Rows < 200 {
		t.Fatalf("expected a tall surface to test against, got %d rows", s.Rows)
	}
}

// The window is a window: what it shows must depend on where it is.
func TestPanningChangesWhatIsDrawn(t *testing.T) {
	s, mods := demoSurface(20, 2)
	a := newBuf(40, 12)
	DrawSurface(a, s, racksurface.View{X: 0, Y: 0, W: 40, H: 12}, mods, ctlAt{Module: -1})
	c := newBuf(40, 12)
	DrawSurface(c, s, racksurface.View{X: 0, Y: 40, W: 40, H: 12}, mods, ctlAt{Module: -1})
	same := 0
	for k, v := range a.cells {
		if c.cells[k] == v {
			same++
		}
	}
	if same == len(a.cells) {
		t.Error("panning down showed exactly the same thing")
	}
}

// The module is as tall as its controls need, and its controls sit inside it.
func TestModuleRowsHoldsItsControls(t *testing.T) {
	const slotCols = 13
	for _, c := range []struct{ ctls, slots int }{{1, 1}, {3, 1}, {6, 2}, {12, 3}} {
		rows := ModuleRows(c.ctls, c.slots, slotCols)
		across := ctlsAcross(c.slots, slotCols)
		down := (c.ctls + across - 1) / across
		if want := panelPad*2 + 1 + down*ctlBlockRows(); rows != want {
			t.Errorf("%d controls in %d slots: %d rows, want %d", c.ctls, c.slots, rows, want)
		}
		// The last control's block has to end inside the module.
		if end := 1 + panelPad + down*ctlBlockRows(); end > rows {
			t.Errorf("%d controls in %d slots overflow: block ends at %d, module is %d",
				c.ctls, c.slots, end, rows)
		}
	}
}

// The narrowest module still has to work: one slot is where a dial and its
// frame only just fit, so it is where an off-by-one in the padding shows.
func TestAOneSlotModuleStillDrawsItsControls(t *testing.T) {
	s, mods := demoSurface(4, 1)
	b := newBuf(60, 30)
	DrawSurface(b, s, racksurface.View{X: 0, Y: 0, W: 60, H: 30}, mods, ctlAt{Module: -1})
	if len(b.out) > 0 {
		t.Errorf("%d writes escaped a 1-slot module", len(b.out)/2)
	}
	if b.art() == 0 {
		t.Error("a 1-slot module drew no dial")
	}
}

// A wide module puts its controls side by side rather than in one tall column,
// which is both what the panel does and the only way a 3-slot module is not a
// tall thin strip.
func TestAWideModuleLaysControlsAcross(t *testing.T) {
	one := ctlsAcross(1, racksurface.DefaultMetrics.SlotCols)
	three := ctlsAcross(3, racksurface.DefaultMetrics.SlotCols)
	if three <= one {
		t.Errorf("a 3-slot module fits %d controls across, a 1-slot one %d", three, one)
	}
}

// A control whose module is not on the surface must still be reachable: it is
// a disagreement to see, not one to hide.
func TestAnOrphanControlIsNotDropped(t *testing.T) {
	items := []racksurface.Item{{Key: "a", Title: "a", Slots: 2, Section: "s"}}
	ctls := []Control{
		{ControlInfo: controlspec.ControlInfo{ID: "a1", Module: "a"}},
		{ControlInfo: controlspec.ControlInfo{ID: "x1", Module: "nowhere"}},
	}
	mods := attach(items, ctls)
	if _, ok := cursorOf(mods, "x1"); !ok {
		t.Error("a control in an unknown module vanished")
	}
}

// Module names are matched case-insensitively, because the registry records
// the name as the panel spells it and the frame as the layout spells it.
func TestModulesMatchRegardlessOfCase(t *testing.T) {
	items := []racksurface.Item{{Key: "Wow & Flutter", Title: "Wow & Flutter", Slots: 2, Section: "s"}}
	ctls := []Control{{ControlInfo: controlspec.ControlInfo{ID: "wf", Module: "wow & flutter"}}}
	mods := attach(items, ctls)
	if len(mods) != 1 {
		t.Fatalf("got %d modules, want 1 — the control was treated as an orphan", len(mods))
	}
	if len(mods[0].Ctls) != 1 {
		t.Error("the control did not attach to its module")
	}
}

// The cursor walks off the end of one module into the next, in rack order.
func TestTheCursorWalksFromModuleToModule(t *testing.T) {
	_, mods := demoSurface(3, 2)
	a := ctlAt{Module: 0, Index: 2} // the last control of the first module
	if got := a.step(mods, 1); got.Module != 1 || got.Index != 0 {
		t.Errorf("stepping past the end went to %+v, want module 1 index 0", got)
	}
	if got := (ctlAt{Module: 0, Index: 0}).step(mods, -1); got.Module != 0 || got.Index != 0 {
		t.Errorf("stepping back from the first control went to %+v, want to stay", got)
	}
}

func mini(a, b int) int {
	if a < b {
		return a
	}
	return b
}
