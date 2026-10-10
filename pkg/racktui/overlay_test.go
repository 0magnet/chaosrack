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
