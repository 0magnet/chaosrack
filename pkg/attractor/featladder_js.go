//go:build js && wasm

package attractor

import (
	"math"
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// featLadders draws the Features module's lit segments, on a canvas over each
// ladder's well (.lad-cv); the unlit ones under it are the stylesheet's.
//
// They were the stylesheet's too, a lit layer clipped to a --lit property.
// That made a reading one write, but the clip is an effect, and changing an
// effect anywhere on this page makes Chrome rebuild the compositing of the
// whole rack (PaintArtifactCompositor, 30 ms and more): with music playing the
// ladders move on most frames, so the rack was rebuilt on every frame and
// drew at half its rate while the module was on screen. A canvas's pixels are
// its own layer's, and drawing them repaints nothing else.
type featLadders struct {
	cv   [len(featNames)]js.Value // the canvas over each well
	g    [len(featNames)]js.Value // its 2D context
	w, h [len(featNames)]float64  // its backing size, device pixels; 0 until measured
	lit  [len(featNames)]int      // segments lit
	gap  float64                  // the dark line between segments, device pixels
	ro   js.Value                 // sizes the canvases, after layout and never in a frame
	tint [3]js.Value              // green, amber, red
}

// ladderTints are the segment colors, by ladderTint.
var ladderTints = [3]string{"#37d457", "#f0b429", "#ff3b2f"}

// set lights n segments of ladder i, drawing only the ones that change. The
// canvas is made, or found again, only when a write is due and the one held
// has left the page. A ladder that has never lit needs no canvas to stay dark.
func (l *featLadders) set(i, n int) {
	if n == l.lit[i] && (l.cv[i].Truthy() || n == 0) {
		return
	}
	if !l.cv[i].Truthy() || !l.cv[i].Get("isConnected").Bool() {
		if !l.attach(i) {
			return
		}
	}
	from := l.lit[i]
	l.lit[i] = n
	l.draw(i, from, n)
}

// attach puts a canvas over ladder i's well. It is sized when the observer
// first reports it, after layout, and drawn then.
func (l *featLadders) attach(i int) bool {
	bar := dom.Doc.Call("getElementById", "feat-bar-"+strconv.Itoa(i))
	if !bar.Truthy() {
		return false
	}
	if !l.ro.Truthy() {
		l.ro = js.Global().Get("ResizeObserver").New(js.FuncOf(l.resized)) // for the life of the page
		for k, c := range ladderTints {
			l.tint[k] = js.ValueOf(c)
		}
	}
	cv := dom.Doc.Call("createElement", "canvas")
	cv.Set("className", "lad-cv")
	bar.Call("appendChild", cv)
	l.cv[i], l.g[i] = cv, cv.Call("getContext", "2d")
	l.w[i], l.h[i], l.lit[i] = 0, 0, 0
	l.ro.Call("observe", cv)
	return true
}

// resized sizes each canvas the observer reports to its box in device pixels
// and draws it again: a new backing store starts blank.
func (l *featLadders) resized(_ js.Value, a []js.Value) any {
	dpr := js.Global().Get("devicePixelRatio").Float()
	l.gap = 2 * dpr
	entries := a[0]
	for e := range entries.Length() {
		entry := entries.Index(e)
		target := entry.Get("target")
		for i := range l.cv {
			if !l.cv[i].Equal(target) {
				continue
			}
			box := entry.Get("contentRect")
			w := float64(int(box.Get("width").Float()*dpr + 0.5))
			h := float64(int(box.Get("height").Float()*dpr + 0.5))
			if w == l.w[i] && h == l.h[i] {
				break
			}
			l.w[i], l.h[i] = w, h
			l.cv[i].Set("width", w)
			l.cv[i].Set("height", h)
			l.draw(i, 0, l.lit[i])
			break
		}
	}
	return nil
}

// draw changes ladder i from from lit segments to to: lights the ones between
// on the way up, puts them out on the way down.
func (l *featLadders) draw(i, from, to int) {
	if l.w[i] == 0 || from == to {
		return
	}
	g, w := l.g[i], l.w[i]
	// Where segment k's slot starts, from the bottom, on a whole pixel: slots
	// then share their edges exactly, and putting segments out cannot leave
	// part of an edge pixel lit, or take one from a segment still lit.
	pitch := l.h[i] / featSegs
	edge := func(k int) float64 { return math.Round(l.h[i] - float64(k)*pitch) }
	if to < from {
		g.Call("clearRect", 0, edge(from), w, edge(to)-edge(from))
		return
	}
	tint := -1
	for k := from; k < to; k++ {
		if t := ladderTint(k); t != tint {
			tint = t
			g.Set("fillStyle", l.tint[t])
		}
		// A segment is its slot less the gap above it, as the stylesheet's
		// mask cuts them.
		top := math.Round(edge(k+1) + l.gap)
		g.Call("fillRect", 0, top, w, edge(k)-top)
	}
}
