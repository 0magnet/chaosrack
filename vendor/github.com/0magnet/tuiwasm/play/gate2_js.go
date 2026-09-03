//go:build js && wasm

package play

import "github.com/gdamore/tcell/v2"

// gated2 is gated for the tcell v2 lineage: a tcell/v2.Screen that does not paint when nobody is looking at it.
//
// Why this is needed at all. A page runs Go on one thread, and every open
// demo's loop is a goroutine on it. Drawing a full screen through tcell is by
// far the most expensive thing these demos do — several times more than
// computing the frame — and an animation asks for it sixty times a second. Two
// windows fit; three do not, and the page stops answering rather than merely
// running slowly, because the JS event loop never gets a turn.
//
// Only one screen holds the page globals at a time, which is exactly the
// window in front. The others are still running, and their frames are drawn
// into a terminal nobody can see. Suppressing that costs nothing visible and
// is the difference between three windows working and the tab wedging.
//
// The animation is not paused, only its output: it keeps time, so a window
// brought back to the front shows the state it would have reached rather than
// resuming from where it was hidden. Everything except Show is forwarded, so
// the demo's own bookkeeping is unaffected.
type gated2 struct {
	tcell.Screen
	active func() bool
}

func (g *gated2) Show() {
	if g.active == nil || g.active() {
		g.Screen.Show()
	}
}

// Sync is the unconditional cousin of Show and is used to repaint after a
// resize or a resume, so it has to be gated the same way or a hidden window
// pays the full cost anyway.
func (g *gated2) Sync() {
	if g.active == nil || g.active() {
		g.Screen.Sync()
	}
}

// Active reports whether this window is the one in front.
//
// canvas in termanim looks for exactly this method on the screen it is given
// and skips the whole frame when it returns false. Gating Show alone is not
// enough: most of the cost of a frame is spent before anything reaches the
// screen, in computing the surface and setting every cell.
func (g *gated2) Active() bool { return g.active == nil || g.active() }
