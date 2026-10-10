package racktui

import "github.com/gdamore/tcell/v3"

// The progressive panel: in a terminal that lays things over its cells (a
// progressive terminal, websh: OSC 7337 place), the page's own parts go over
// the page look's cells — the model's canvas over the scene, the real panel
// over the window — at the same places and the same scale, so the cells
// under them are the same picture drawn coarser. They are what any other
// terminal shows, and what this one shows with the overlay off (--plain, or
// w): one panel, drawn twice over.

// Overlay is a Source that can lay the page's parts over the panel's cells.
type Overlay interface {
	// Overlay is what to write to the terminal, after a frame, to lay the
	// parts over l; Overlay(nil) is what takes them away. It is called after
	// every frame, and the panel writes only what differs from the last.
	Overlay(l *Layout) string
}

// Native is an Overlay whose parts are drawn at their own size: the panel at
// the page's scale and the model at the terminal's, so the cells only say where
// they go. That is what lets the panel in a terminal BE the page's interface
// at an ordinary cell size, where the page look's picture of it — drawn at
// pagePxPerCol to a cell — looks right only zoomed far out. The page look's
// cells under the parts are what the terminal shows with them off.
type Native interface {
	Overlay
	// WindowCells is the panel's window at the page's own size, its title
	// row included, in this terminal's cells: 0, 0 when it cannot say yet.
	WindowCells() (w, h int)
	// Dock is where the page last put its panel: "top", "bottom", "left" or
	// "right" docks the window against that edge, anything else floats it.
	Dock() string
	// DockCells is the window docked against edge, across it, in cells: the
	// page's own drawer size, its title row included. 0 when it cannot say.
	DockCells(edge string) int
	// OnWindow hands the page what moves the window: what is done to its
	// title bar there comes back to the panel through f. nil lets go.
	OnWindow(f func(WindowAct))
}

// WindowAct is something done to the window by hand, in the page.
type WindowAct struct {
	// Kind is "move" (its top left to X, Y, floating), "dock" (against Edge,
	// or floating where it last floated for any other Edge), "full" (in and
	// out of the whole terminal) or "hide" (out of the way and back).
	Kind string
	X, Y int
	Edge string
}

// Rect is a rectangle of cells.
type Rect struct{ X, Y, W, H int }

// Layout is where the page look put the scene and the panel, in cells.
type Layout struct {
	// Scene is the cells the scene is sampled across, and SceneShown the part
	// of them the window does not cover (its title and bars included). Window
	// is the cells the panel is drawn in, without the title and bars. Each is
	// zero when not shown.
	Scene, SceneShown, Window Rect
	// ViewX, ViewY is the panel's cell at the window's top left.
	ViewX, ViewY int
	// PxPerCol, PxPerRow is the panel's own pixels a cell.
	PxPerCol, PxPerRow float64
	// Native is a Layout for a Native overlay: the scene is the whole
	// terminal, behind the window, and Window is the whole window, its title
	// row included, in which the panel is the page's size and scrolls.
	Native bool
	// Dock is the edge the Native window is docked against, or "" floating.
	Dock string
}

// plain turns the overlay off from the start (SetPlain).
var plain bool

// SetPlain draws cells only, whatever the terminal can lay over them.
func SetPlain(b bool) { plain = b }

// layoutNow is the page look's layout as last drawn, or nil when there is
// nothing to lay anything over: the drawn looks, or the list.
func (p *panel) layoutNow() *Layout {
	if p.pic == nil || p.list || !p.overlay || p.scrW == 0 {
		return nil
	}
	l := &Layout{ViewX: p.view.X, ViewY: p.view.Y, PxPerCol: pagePxPerCol, PxPerRow: pagePxPerRow}
	if p.nativeNow() {
		// The page's interface: the model behind everything, the window
		// floating over it, as the page floats its controls over the model.
		l.Native = true
		if p.winMode != winFull {
			l.Scene = Rect{0, 0, p.scrW, p.scrH}
			l.SceneShown = l.Scene
		}
		switch p.winMode {
		case winFull:
			l.Window = Rect{0, 0, p.scrW, p.scrH}
		case winHalf:
			l.Window = p.winR
			l.Dock = p.dock
		}
		return l
	}
	if p.winMode != winFull {
		l.Scene = Rect{0, 0, p.scrW, maxi(p.scrH-1, 1)}
		l.SceneShown = l.Scene
		if p.winMode == winHalf {
			l.SceneShown.W = p.winX
		}
	}
	if p.winMode != winHidden {
		l.Window = Rect{p.winX, p.winY, p.view.W, p.view.H}
	}
	return l
}

// nativeNow reports whether the parts are laid over the cells at their own
// size now: the page look, with the overlay on, from a Native source.
func (p *panel) nativeNow() bool {
	if p.pic == nil || p.list || !p.overlay {
		return false
	}
	_, ok := p.src.(Native)
	return ok
}

// windowAct does what was done to the window by hand, in the page.
func (p *panel) windowAct(a WindowAct) {
	switch a.Kind {
	case "move":
		p.floatX, p.floatY = a.X, a.Y
		p.dock = ""
		p.winMode = winHalf
	case "dock":
		p.dock = dockEdge(a.Edge)
		p.winMode = winHalf
	case "full":
		if p.winMode == winFull {
			p.winMode = winHalf
		} else {
			p.winMode = winFull
		}
	case "hide":
		if p.winMode == winHidden {
			p.winMode = winHalf
		} else {
			p.winMode = winHidden
		}
	}
	p.scene, p.canv = nil, nil
}

// syncOverlay writes what lays the parts over the frame just shown, when it
// differs from what was written last.
func (p *panel) syncOverlay(sc tcell.Screen) {
	o, ok := p.src.(Overlay)
	if !ok {
		return
	}
	p.writeOverlay(sc, o.Overlay(p.layoutNow()))
}

// clearOverlay takes the parts away, before the screen is let go.
func (p *panel) clearOverlay(sc tcell.Screen) {
	if o, ok := p.src.(Overlay); ok {
		p.writeOverlay(sc, o.Overlay(nil))
	}
}

func (p *panel) writeOverlay(sc tcell.Screen, seq string) {
	if seq == p.laidOut {
		return
	}
	p.laidOut = seq
	if t, ok := sc.Tty(); ok && seq != "" {
		_, _ = t.Write([]byte(seq)) //nolint:errcheck // a terminal that will not take it shows the cells, which is the point
	}
}

// dockEdge is edge if a window docks against it, else "": floating.
func dockEdge(edge string) string {
	switch edge {
	case "top", "bottom", "left", "right":
		return edge
	}
	return ""
}
