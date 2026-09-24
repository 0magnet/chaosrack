package panelart

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// The pointer has to actually move, and it has to move monotonically round
// the sweep — a knob whose pointer jumps back at some value reads as broken.
func TestPointerSweepsMonotonically(t *testing.T) {
	prev := math.Inf(-1)
	for f := 0.0; f <= 1.0001; f += 0.05 {
		a := KnobAngle(f)
		if a <= prev {
			t.Fatalf("angle at %.2f is %v, not past the previous %v", f, a, prev)
		}
		prev = a
	}
}

func TestKnobAngleClampsOutsideTheSweep(t *testing.T) {
	if KnobAngle(-1) != KnobAngle(0) {
		t.Error("below zero should clamp to the minimum")
	}
	if KnobAngle(2) != KnobAngle(1) {
		t.Error("above one should clamp to the maximum")
	}
}

// The sweep leaves a dead zone at the bottom, so the two ends of the travel
// must not land on the same spot.
func TestTheEndsOfTheSweepAreDistinguishable(t *testing.T) {
	lo, hi := KnobAngle(0), KnobAngle(1)
	if math.Abs(hi-lo-knobSweep) > 1e-9 {
		t.Errorf("sweep is %v, want %v", hi-lo, knobSweep)
	}
	if hi-lo >= 2*math.Pi {
		t.Error("a full turn makes the ends indistinguishable")
	}
}

// Turning the knob must change the picture. This is the test that would have
// caught a pointer drawn at a constant angle.
func TestTurningTheKnobChangesTheCells(t *testing.T) {
	a, _ := KnobCells(12, 0.1, 0, Dark)
	b, _ := KnobCells(12, 0.9, 0, Dark)
	if len(a) != len(b) || len(a) == 0 {
		t.Fatalf("got %d and %d cells", len(a), len(b))
	}
	same := 0
	for i := range a {
		if a[i] == b[i] {
			same++
		}
	}
	if same == len(a) {
		t.Error("the knob looks identical at 0.1 and 0.9")
	}
}

// A knob has to be ROUND on screen, which means its cell box has to be about
// CellAspect times wider in columns than it is tall in rows.
func TestAKnobIsRoundOnScreenNotInCells(t *testing.T) {
	for _, cols := range []int{8, 10, 12, 16, 20, 26} {
		_, rows := KnobCells(cols, 0.5, 0, Dark)
		got := float64(cols) / float64(rows)
		if math.Abs(got-CellAspect()) > 0.25 {
			t.Errorf("%d cols gave %d rows (ratio %.2f), want about %.2f",
				cols, rows, got, CellAspect())
		}
	}
}

func TestRowsForRoundsRatherThanTruncates(t *testing.T) {
	// 10/1.67 = 5.99, which truncation would call 5.
	if got := RowsFor(10, 10, 10, 1.67); got != 6 {
		t.Errorf("RowsFor = %d, want 6", got)
	}
}

func TestRowsForRefusesNonsense(t *testing.T) {
	for _, c := range [][4]int{{0, 10, 10, 0}, {10, 0, 10, 0}, {10, 10, 0, 0}} {
		if got := RowsFor(c[0], c[1], c[2], CellAspect()); got != c[3] {
			t.Errorf("RowsFor(%d,%d,%d) = %d, want %d", c[0], c[1], c[2], got, c[3])
		}
	}
}

// Render must average, not sample one pixel: a half-black half-white source
// should come back grey rather than one or the other.
func TestRenderAveragesRatherThanPicking(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			c := color.RGBA{A: 255}
			if x%2 == 0 {
				c = color.RGBA{255, 255, 255, 255}
			}
			im.SetRGBA(x, y, c)
		}
	}
	cells := Render(im, 2, 2)
	if len(cells) != 4 {
		t.Fatalf("got %d cells, want 4", len(cells))
	}
	for i, c := range cells {
		if c.Top.R < 100 || c.Top.R > 155 {
			t.Errorf("cell %d top red %d, want about 128 (averaged)", i, c.Top.R)
		}
	}
}

func TestRenderRefusesNonsense(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 4, 4))
	if Render(im, 0, 2) != nil || Render(im, 2, 0) != nil {
		t.Error("a zero dimension should render nothing")
	}
	if Render(image.NewRGBA(image.Rect(0, 0, 0, 0)), 2, 2) != nil {
		t.Error("an empty image should render nothing")
	}
}

// Detents are drawn outside the body, so asking for them must not paint over
// the knob itself — and asking for fewer than two must draw none.
func TestDetentsAreOptional(t *testing.T) {
	none, _ := KnobCells(16, 0.5, 0, Dark)
	some, _ := KnobCells(16, 0.5, 7, Dark)
	if len(none) != len(some) {
		t.Fatal("detents changed the size of the knob")
	}
	diff := 0
	for i := range none {
		if none[i] != some[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Error("asking for 7 detents drew none")
	}
	one, _ := KnobCells(16, 0.5, 1, Dark)
	for i := range one {
		if one[i] != none[i] {
			t.Error("a single detent should draw nothing: there is no position to mark")
			break
		}
	}
}

func TestLampLightsUp(t *testing.T) {
	off := Render(Lamp(16, false, Dark), 8, 5)
	on := Render(Lamp(16, true, Dark), 8, 5)
	sum := func(cs []Cell) int {
		n := 0
		for _, c := range cs {
			n += int(c.Top.R) + int(c.Bottom.R)
		}
		return n
	}
	if sum(on) <= sum(off) {
		t.Error("a lit lamp is not brighter than an unlit one")
	}
}

func TestKnobRefusesNonsenseSizes(t *testing.T) {
	if cells, rows := KnobCells(0, 0.5, 0, Dark); cells != nil || rows != 0 {
		t.Error("a zero-column knob should draw nothing")
	}
	if im := Knob(0, 0.5, 0, Dark); im.Bounds().Dx() < 2 {
		t.Error("Knob should clamp a nonsense size rather than panic")
	}
}

// The aspect is settable, and a knob follows it — which is the whole point of
// it not being a constant.
func TestTheKnobFollowsTheCellAspect(t *testing.T) {
	defer SetCellAspect(0) // back to the default
	SetCellAspect(1.67)
	_, wide := KnobCells(12, 0.5, 0, Dark)
	SetCellAspect(2.37)
	_, tall := KnobCells(12, 0.5, 0, Dark)
	if wide <= tall {
		t.Errorf("a 1.67 cell gave %d rows and a 2.37 cell %d; a taller cell needs FEWER rows", wide, tall)
	}
}

func TestSetCellAspectRefusesNonsense(t *testing.T) {
	SetCellAspect(1.5)
	SetCellAspect(0)
	if CellAspect() != DefaultCellAspect {
		t.Errorf("zero should restore the default, got %v", CellAspect())
	}
	SetCellAspect(-3)
	if CellAspect() != DefaultCellAspect {
		t.Errorf("a negative should restore the default, got %v", CellAspect())
	}
}

// A lamp has to be the same size as a knob, or a panel of mixed controls
// comes out ragged.
func TestALampIsTheSameBoxAsAKnob(t *testing.T) {
	for _, cols := range []int{8, 12, 16} {
		_, kr := KnobCells(cols, 0.5, 0, Dark)
		_, lr := LampCells(cols, true, Dark)
		if kr != lr {
			t.Errorf("%d cols: knob is %d rows, lamp is %d", cols, kr, lr)
		}
	}
}

func TestLampCellsLightUp(t *testing.T) {
	off, _ := LampCells(12, false, Dark)
	on, _ := LampCells(12, true, Dark)
	sum := func(cs []Cell) int {
		n := 0
		for _, c := range cs {
			n += int(c.Top.R) + int(c.Bottom.R)
		}
		return n
	}
	if sum(on) <= sum(off) {
		t.Error("a lit lamp is not brighter than an unlit one")
	}
	if c, r := LampCells(0, true, Dark); c != nil || r != 0 {
		t.Error("a zero-column lamp should draw nothing")
	}
}
