package racktui

import (
	"testing"

	"github.com/gdamore/tcell/v3"
)

// overlayRack is a rack whose page parts can go over the cells.
type overlayRack struct{ sceneRack }

func (*overlayRack) Overlay(*Layout) string { return "" }

// The scene is laid beside the window, and over the whole terminal when the
// window is out of the way; the panel over the window's cells, from the
// view's corner.
func TestTheOverlayFollowsTheWindow(t *testing.T) {
	p := scenePanel(&sceneRack{})
	p.overlay = true
	p.view.X, p.view.Y = 7, 3
	l := p.layoutNow()
	if l == nil {
		t.Fatal("the page look laid nothing out")
	}
	if l.Scene != (Rect{0, 0, 40, 10}) || l.SceneShown != (Rect{0, 0, p.winX, 10}) {
		t.Errorf("half: the scene is %+v, shown %+v; the window starts at column %d", l.Scene, l.SceneShown, p.winX)
	}
	if l.Window != (Rect{p.winX, p.winY, p.view.W, p.view.H}) || l.ViewX != 7 || l.ViewY != 3 {
		t.Errorf("half: the panel is %+v from %d,%d", l.Window, l.ViewX, l.ViewY)
	}
	if l.PxPerCol != pagePxPerCol || l.PxPerRow != pagePxPerRow {
		t.Errorf("a cell is %gx%g panel pixels, want %gx%g", l.PxPerCol, l.PxPerRow, pagePxPerCol, pagePxPerRow)
	}

	p.winMode = winHidden
	p.layoutWindow(p.scrW, p.scrH)
	if l := p.layoutNow(); l.SceneShown != l.Scene || l.Window.W != 0 {
		t.Errorf("hidden: shown %+v of %+v, window %+v", l.SceneShown, l.Scene, l.Window)
	}
	p.winMode = winFull
	p.layoutWindow(p.scrW, p.scrH)
	if l := p.layoutNow(); l.Scene.W != 0 || l.SceneShown.W != 0 || l.Window.W == 0 {
		t.Errorf("full: scene %+v, window %+v", l.Scene, l.Window)
	}
}

// Off, in the list, or in a look with no picture, nothing goes over the
// cells.
func TestTheOverlayIsOnlyOverThePageLook(t *testing.T) {
	p := scenePanel(&sceneRack{})
	if p.layoutNow() != nil {
		t.Error("with the overlay off something was laid out")
	}
	p.overlay = true
	p.list = true
	if p.layoutNow() != nil {
		t.Error("over the list something was laid out")
	}
	p.list, p.pic = false, nil
	if p.layoutNow() != nil {
		t.Error("with no picture something was laid out")
	}
}

// w switches the overlay, where the rack has one; elsewhere it is a letter
// of the filter like any other.
func TestWSwitchesTheOverlay(t *testing.T) {
	w := tcell.NewEventKey(tcell.KeyRune, "w", 0)
	p := scenePanel(&sceneRack{})
	p.src = &overlayRack{}
	p.overlay = true
	p.key(w)
	if p.overlay {
		t.Error("w left the overlay on")
	}
	p.key(w)
	if !p.overlay {
		t.Error("a second w left the overlay off")
	}

	q := newPanel(&fakeRack{ctls: inModule(dial("a", 0, 10, 1, 0, "3"))})
	q.key(w)
	if q.filt != "w" {
		t.Errorf("without an overlay w filtered %q", q.filt)
	}
}

// nativeRack is a rack whose parts go over the cells at their own size: its
// window is 12 x 6 cells, title row included.
type nativeRack struct {
	overlayRack
	hand func(WindowAct)
}

func (*nativeRack) WindowCells() (w, h int)      { return 12, 6 }
func (r *nativeRack) OnWindow(f func(WindowAct)) { r.hand = f }
func (*nativeRack) Dock() string                 { return "" }

// DockCells is 4 cells across any edge.
func (*nativeRack) DockCells(string) int { return 4 }

// Over a native overlay the window floats at the page's size, where it was
// put and kept on the terminal, with the model behind all of it; what is
// done to its title bar moves it, fills the terminal with it, or hides it.
func TestANativeWindowFloats(t *testing.T) {
	p := scenePanel(&sceneRack{})
	p.src = &nativeRack{}
	p.overlay = true
	p.floatX, p.floatY = 2, 2
	whole := func() Rect { return Rect{0, 0, p.scrW, p.scrH} }
	lay := func() *Layout {
		p.layoutWindow(p.scrW, p.scrH)
		return p.layoutNow()
	}

	l := lay()
	if !l.Native || l.Scene != whole() || l.SceneShown != whole() {
		t.Errorf("native %v: the scene is %+v, shown %+v, want the whole %+v", l.Native, l.Scene, l.SceneShown, whole())
	}
	if l.Window != (Rect{2, 2, 12, 6}) {
		t.Errorf("the window is %+v, want 12x6 at 2,2", l.Window)
	}
	if p.winX != 2 || p.winY != 3 {
		t.Errorf("the body starts at %d,%d, want under the title row at 2,3", p.winX, p.winY)
	}

	p.windowAct(WindowAct{Kind: "move", X: 99, Y: -4})
	if l := lay(); l.Window != (Rect{p.scrW - 12, 0, 12, 6}) {
		t.Errorf("moved off the terminal, the window is %+v; it should stay on it", l.Window)
	}
	p.windowAct(WindowAct{Kind: "full"})
	if l := lay(); l.Window != whole() || l.Scene.W != 0 {
		t.Errorf("full: the window is %+v and the scene %+v", l.Window, l.Scene)
	}
	p.windowAct(WindowAct{Kind: "full"})
	p.windowAct(WindowAct{Kind: "hide"})
	if l := lay(); l.Window.W != 0 || l.SceneShown != whole() {
		t.Errorf("hidden: the window is %+v and the scene shown %+v", l.Window, l.SceneShown)
	}
	p.windowAct(WindowAct{Kind: "hide"})
	if l := lay(); l.Window != (Rect{p.scrW - 12, 0, 12, 6}) {
		t.Errorf("shown again, the window is %+v; it should be where it was left", l.Window)
	}

	// Docked, it lies along the whole edge at the page's drawer size, and
	// moved by its title bar it floats again where it is let go.
	for edge, want := range map[string]Rect{
		"bottom": {0, p.scrH - 4, p.scrW, 4},
		"top":    {0, 0, p.scrW, 4},
		"left":   {0, 0, 4, p.scrH},
		"right":  {p.scrW - 4, 0, 4, p.scrH},
	} {
		p.windowAct(WindowAct{Kind: "dock", Edge: edge})
		if l := lay(); l.Window != want || l.Dock != edge || p.winY != want.Y+1 {
			t.Errorf("docked %s: the window is %+v (dock %q, body at row %d), want %+v", edge, l.Window, l.Dock, p.winY, want)
		}
	}
	p.windowAct(WindowAct{Kind: "dock", Edge: "float"})
	if l := lay(); l.Window != (Rect{p.scrW - 12, 0, 12, 6}) || l.Dock != "" {
		t.Errorf("floated again, the window is %+v (dock %q); it should be where it last floated", l.Window, l.Dock)
	}
	p.windowAct(WindowAct{Kind: "dock", Edge: "left"})
	p.windowAct(WindowAct{Kind: "move", X: 3, Y: 1})
	if l := lay(); l.Window != (Rect{3, 1, 12, 6}) || l.Dock != "" {
		t.Errorf("moved off the left edge, the window is %+v (dock %q), want floating at 3,1", l.Window, l.Dock)
	}

	// With the parts off, the window is the half one the cells always had.
	p.overlay = false
	p.layoutWindow(p.scrW, p.scrH)
	if p.winY != 1 || p.view.W != maxi(p.scrW/2, 24)-1 {
		t.Errorf("plain: the window starts at row %d, %d wide", p.winY, p.view.W)
	}
}
