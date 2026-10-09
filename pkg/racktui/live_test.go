package racktui

import (
	"testing"

	"github.com/gdamore/tcell/v3"
)

// A knob turned on the page shows here without a key pressed here.
func TestTheValueFollowsTheRack(t *testing.T) {
	f := &fakeRack{ctls: []Control{dial("zoom", -8, 8, 0.1, 0, "1.5")}}
	p := newPanel(f)
	if p.refresh() {
		t.Fatal("nothing changed, and the panel would redraw")
	}
	f.ctls[0].Value = "2.5"
	if !p.refresh() {
		t.Fatal("the rack moved and the panel did not notice")
	}
	if c, _ := p.at(); c.Value != "2.5" {
		t.Errorf("the panel shows %q, the rack 2.5", c.Value)
	}
	a, _ := cursorOf(p.mods, "zoom")
	if got := p.mods[a.Module].Ctls[a.Index].Value; got != "2.5" {
		t.Errorf("the surface shows %q, the rack 2.5", got)
	}
}

// A control that comes or goes is a change to the rack, not to a value.
func TestANewControlIsAReload(t *testing.T) {
	f := &fakeRack{ctls: []Control{dial("zoom", -8, 8, 0.1, 0, "1.5")}}
	p := newPanel(f)
	f.ctls = append(f.ctls, dial("twist", 0, 1, 0.1, 0, "0"))
	if !p.refresh() || len(p.all) != 2 {
		t.Fatalf("after a control was added the panel has %d", len(p.all))
	}
}

// cellOf is a surface cell inside control id, on screen with the view at 0,0.
func cellOf(t *testing.T, p *panel, id string) (int, int) {
	t.Helper()
	a, ok := cursorOf(p.mods, id)
	if !ok {
		t.Fatalf("%s is not on the surface", id)
	}
	x, y, _, _, ok := ctlRect(p.surf, p.mods, a)
	if !ok {
		t.Fatalf("%s has no rect", id)
	}
	return x, y
}

func click(p *panel, x, y int) {
	p.mouse(tcell.NewEventMouse(x, y, tcell.Button1, 0), 400, 400)
	p.mouse(tcell.NewEventMouse(x, y, 0, 0), 400, 400)
}

// Clicking a control chooses it; the wheel over it turns it.
func TestTheMouseChoosesAndTurns(t *testing.T) {
	f := &fakeRack{ctls: inModule(dial("a", 0, 10, 1, 0, "3"), dial("b", 0, 10, 1, 0, "3"))}
	p := newPanel(f)
	p.view.W, p.view.H = 399, 398
	x, y := cellOf(t, p, "b")
	click(p, x+1, y+1)
	if c, _ := p.at(); c.ID != "b" {
		t.Fatalf("clicked b, chose %q", c.ID)
	}
	x, y = cellOf(t, p, "a")
	p.mouse(tcell.NewEventMouse(x+1, y+1, tcell.WheelUp, 0), 400, 400)
	if len(f.sets) != 1 || f.sets[0] != "a=4" {
		t.Fatalf("the wheel up over a sent %v, want [a=4]", f.sets)
	}
	if c, _ := p.at(); c.ID != "a" {
		t.Errorf("the wheel turned a but left %q chosen", c.ID)
	}
}

// Holding the button and moving is not a second click.
func TestADragIsNotAClick(t *testing.T) {
	withPUnitLook(t)
	f := &fakeRack{ctls: inModule(dial("a", 0, 10, 1, 0, "3"))}
	p := newPanel(f)
	p.view.W, p.view.H = 399, 398
	x, y := cellOf(t, p, "a")
	p.mouse(tcell.NewEventMouse(x+puButtons, y+2, tcell.Button1, 0), 400, 400)
	p.mouse(tcell.NewEventMouse(x+puButtons, y+2, tcell.Button1, 0), 400, 400)
	if len(f.sets) != 1 {
		t.Fatalf("one press of + sent %v", f.sets)
	}
}

// A P-unit's buttons do what they say.
func TestThePUnitButtonsWork(t *testing.T) {
	withPUnitLook(t)
	f := &fakeRack{ctls: inModule(dial("a", 0, 10, 1, 0, "3"))}
	p := newPanel(f)
	p.view.W, p.view.H = 399, 398
	x, y := cellOf(t, p, "a")
	click(p, x+puButtons, y+2) // +
	click(p, x+puButtons, y+4) // −
	click(p, x+puButtons, y+3) // 0
	want := []string{"a=4", "a=3", "a=0"}
	if len(f.sets) != len(want) {
		t.Fatalf("+ − 0 sent %v, want %v", f.sets, want)
	}
	for i := range want {
		if f.sets[i] != want[i] {
			t.Errorf("press %d sent %q, want %q", i, f.sets[i], want[i])
		}
	}
}

// Two controls sharing a P-unit: a second click on it chooses the other.
func TestASecondClickStepsThroughASharedUnit(t *testing.T) {
	withPUnitLook(t)
	f := &fakeRack{ctls: []Control{at("atk", "5.1.1"), at("dcy", "5.1.1")}}
	for i := range f.ctls {
		f.ctls[i].Module = "env"
		f.ctls[i].Max = 1
	}
	p := newPanel(f)
	p.view.W, p.view.H = 399, 398
	x, y := cellOf(t, p, "atk")
	click(p, x+3, y+1)
	first, _ := p.at()
	click(p, x+3, y+1)
	second, _ := p.at()
	if first.ID == second.ID {
		t.Errorf("two clicks on the shared unit both chose %q", first.ID)
	}
}

// inModule puts controls in a module, which a control needs to be on the
// surface.
func inModule(cs ...Control) []Control {
	for i := range cs {
		cs[i].Module = "m"
	}
	return cs
}

// The scroll bars take the mouse: a press on one moves the window there, and
// dragging it follows the pointer until the button comes up.
func TestTheScrollBarsDrag(t *testing.T) {
	var cs []Control
	for i := range 60 {
		cs = append(cs, dial("c"+itoa(i), 0, 10, 1, 0, "3"))
	}
	p := newPanel(&fakeRack{ctls: inModule(cs...)})
	const w, h = 30, 12 // a window much smaller than the rack
	p.view.W, p.view.H = w-1, h-2
	s := p.surface()
	if s.Rows <= h || s.Cols <= w {
		t.Fatalf("the rack is %dx%d, too small to scroll in %dx%d", s.Cols, s.Rows, w, h)
	}
	p.mouse(tcell.NewEventMouse(w-1, h-3, tcell.Button1, 0), w, h) // the bottom of the vertical bar
	if p.view.Y+p.view.H < s.Rows-1 {
		t.Errorf("a press at the bottom of the bar left the window at row %d of %d", p.view.Y, s.Rows)
	}
	p.mouse(tcell.NewEventMouse(w-1, 0, tcell.Button1, 0), w, h) // dragged to the top
	if p.view.Y != 0 {
		t.Errorf("dragged to the top, the window is at row %d", p.view.Y)
	}
	p.mouse(tcell.NewEventMouse(w-1, 0, 0, 0), w, h)
	p.mouse(tcell.NewEventMouse(w-1, h-3, 0, 0), w, h) // moving with the button up does nothing
	if p.view.Y != 0 {
		t.Errorf("after the button came up the window moved to row %d", p.view.Y)
	}
	p.mouse(tcell.NewEventMouse(w-2, h-2, tcell.Button1, 0), w, h) // the right end of the horizontal bar
	if p.view.X+p.view.W < s.Cols-1 {
		t.Errorf("a press at the right of the bar left the window at column %d of %d", p.view.X, s.Cols)
	}
}
