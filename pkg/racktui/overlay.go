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
