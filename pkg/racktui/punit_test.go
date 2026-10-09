package racktui

import (
	"testing"

	"github.com/0magnet/chaosrack/pkg/controlspec"
	"github.com/0magnet/chaosrack/pkg/panelart"
	"github.com/0magnet/chaosrack/pkg/racksurface"
)

func withLook(t *testing.T, l Look) {
	t.Helper()
	old := look
	SetLook(l)
	t.Cleanup(func() { SetLook(old) })
}

func at(id, loc string) Control {
	return Control{ControlInfo: controlspec.ControlInfo{ID: id}, Loc: loc}
}

func TestAnAddressIsAColumnAndARow(t *testing.T) {
	a, ok := parseLoc("3.11.2")
	if !ok || a.col != 11 || a.row != 2 || a.letter != "" {
		t.Fatalf("3.11.2 read as %+v %v", a, ok)
	}
	a, ok = parseLoc("3.3.2.g")
	if !ok || a.col != 3 || a.row != 2 || a.letter != "g" {
		t.Fatalf("3.3.2.g read as %+v %v", a, ok)
	}
	if _, ok := parseLoc(""); ok {
		t.Fatal("an empty address parsed")
	}
}

// A control stands where the page has it: its column counted from the bay's
// left, less the column the module starts at.
func TestAPUnitStandsAtItsAddress(t *testing.T) {
	withLook(t, LookPUnit)
	_, bh, _ := block()
	pan := racksurface.Panel{X: 4 * puSlotCols, Y: 10, W: 3 * puSlotCols, Slot: 4}
	cells, idx := placeModule(pan, []Control{at("a", "2.5.1"), at("b", "2.7.3")})
	if got := cells[idx[0]]; got.x != pan.X+1 || got.y != pan.Y+1 {
		t.Errorf("5.1 at %d,%d, want the module's first cell", got.x, got.y)
	}
	if got := cells[idx[1]]; got.x != pan.X+1+2*puSlotCols || got.y != pan.Y+1+2*bh {
		t.Errorf("7.3 at %d,%d, want the third column, third row", got.x, got.y)
	}
}

// Controls sharing a cell are one programmable unit, and one the page has
// not placed still gets a cell rather than vanishing.
func TestASharedCellIsOneUnitAndNothingIsDropped(t *testing.T) {
	withLook(t, LookPUnit)
	pan := racksurface.Panel{W: 2 * puSlotCols}
	cells, idx := placeModule(pan, []Control{
		at("atk", "5.1.2"), at("dcy", "5.1.2"), at("loose", ""),
	})
	if idx[0] != idx[1] || len(cells[idx[0]].members) != 2 {
		t.Errorf("the two at 5.1.2 are in cells %d and %d", idx[0], idx[1])
	}
	if idx[2] < 0 || idx[2] == idx[0] {
		t.Errorf("the unplaced control got cell %d", idx[2])
	}
}

// The page letters a cell's controls down each column and then across, and
// the cursor walks them in that order.
func TestTheCursorWalksInAddressOrder(t *testing.T) {
	cs := []Control{at("c", "1.2.1"), at("x", ""), at("b", "1.1.2"), at("a", "1.1.1")}
	byAddress(cs)
	got := ""
	for _, c := range cs {
		got += c.ID
	}
	if got != "abcx" {
		t.Errorf("walked %q, want abcx", got)
	}
}

// Turning the encoder up lights more of the ring, and a selector lights only
// the stretch round its position.
func TestTheRingLightsToTheValue(t *testing.T) {
	lit := func(g []panelart.Glyph) (n int) {
		for _, c := range g {
			if c.Fg == panelart.Dark.LEDOn {
				n++
			}
		}
		return n
	}
	p := panelart.Dark
	none, half, full := lit(panelart.Ring(6, 3, 0, 0, p)), lit(panelart.Ring(6, 3, 0.5, 0, p)), lit(panelart.Ring(6, 3, 1, 0, p))
	if none != 0 || half >= full || half == 0 {
		t.Errorf("lit cells at 0, 0.5, 1: %d %d %d", none, half, full)
	}
	if sel := lit(panelart.Ring(6, 3, 0.5, 5, p)); sel == 0 || sel >= full {
		t.Errorf("a selector lit %d cells, a full ring %d", sel, full)
	}
}
