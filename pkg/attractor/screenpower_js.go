//go:build js && wasm

package attractor

import "syscall/js"

// A module's own screen, and the switch that powers it.
//
// Three modules carry a live display that is redrawn every frame — the
// scope's tube, the Record monitor, the Desk monitor — and each of them
// costs something real: a canvas copy out of the WebGL drawing buffer, or
// a capture and a path. They used to be free most of the time because they
// were behind a module switch and the module was usually put away, so each
// one simply checked whether it was on screen and returned.
//
// The rack does not hide modules any more; a bay shows everything in it.
// So "nobody can see it" has stopped being the thing that turns these off,
// and each screen needs a power switch of its own — which is what a piece
// of equipment with a tube in it has anyway. A scope has a BEAM switch and
// a monitor has a power button; neither is hidden in a menu.
//
// The check is throttled for the reason scopeVisible's is: reading
// offsetParent makes the browser settle style and layout before it can
// answer, and sixty of those a second is a cost paid by every model on
// every frame whether the screen is drawing or not.

// screenPowerEveryMs is how stale "is this screen live" may get. A quarter
// of a second is four layout reads a second instead of sixty, and is far
// below the time it takes to notice a switch has moved.
const screenPowerEveryMs = 250

// screenPower is one module screen's power state.
type screenPower struct {
	// switchID is the module's own on/off switch. Empty means the screen
	// has no switch and is governed only by whether it is on screen.
	switchID string

	checkAt float64 // when the answer below was last measured
	live    bool    // powered, and on screen
	blanked bool    // the dark face has been painted since it went out
}

// on reports whether this screen should draw this frame.
//
// el is the screen's own element, so that a module scrolled out of the
// drawer or taken away by a mode still costs nothing.
func (p *screenPower) on(el js.Value) bool {
	if frameNowMs-p.checkAt < screenPowerEveryMs && p.checkAt != 0 {
		return p.live
	}
	p.checkAt = frameNowMs
	was := p.live
	p.live = p.measure(el)
	if p.live && !was {
		// Coming back on: the next time it goes out it owes a fresh blank.
		p.blanked = false
	}
	return p.live
}

// measure is the real answer.
func (p *screenPower) measure(el js.Value) bool {
	if !el.Truthy() {
		return false
	}
	if p.switchID != "" {
		sw := doc.Call("getElementById", p.switchID)
		if sw.Truthy() && !sw.Get("checked").Bool() {
			return false
		}
	}
	// Skipped during a panel resize, for the reason the monitors already
	// skipped it: the rack re-measures every module on each pointer move,
	// and a canvas copy in the middle of that is how a drag comes to cost
	// the model a frame.
	if resizing {
		return false
	}
	return el.Get("offsetParent").Truthy()
}

// needsBlank reports whether the caller owes the screen one dark frame.
//
// A screen that is switched off should LOOK off, which takes one paint —
// and exactly one. Repainting a dark face every frame is the cost the
// switch was meant to remove.
func (p *screenPower) needsBlank() bool { return !p.blanked }

// markBlanked records that the dark face has been painted.
func (p *screenPower) markBlanked() { p.blanked = true }

// invalidate forces the next frame to measure again, for the moment a
// switch is flipped and waiting a quarter second to notice would be seen.
func (p *screenPower) invalidate() { p.checkAt = 0 }

// The screens.
var (
	scopeScreenPower  = screenPower{switchID: "scope-beam"}
	recScreenPower    = screenPower{switchID: "rec-mon-on"}
	deskScreenPower   = screenPower{switchID: "desk-mon-on"}
	rowMonScreenPower = screenPower{switchID: "rowmon-on"}
)

// wireScreenPower hooks each screen's switch up so flipping it is noticed
// at once rather than at the next check.
func wireScreenPower() {
	for _, p := range []*screenPower{&scopeScreenPower, &recScreenPower, &deskScreenPower, &rowMonScreenPower} {
		sw := doc.Call("getElementById", p.switchID)
		if !sw.Truthy() {
			continue
		}
		pp := p
		sw.Call("addEventListener", "change", trackedFuncOf(func(js.Value, []js.Value) interface{} {
			pp.invalidate()
			return nil
		}))
	}

	// Scrolling the drawer is the one thing that changes which modules are
	// on screen, and waiting a quarter second to notice is long enough to
	// see a readout sitting still after it has come into view. Forget the
	// cached answers as soon as it moves; the next frame measures again.
	forget := trackedFuncOf(func(js.Value, []js.Value) interface{} {
		invalidateOnScreen()
		return nil
	})
	opts := map[string]interface{}{"passive": true}
	if p := doc.Call("getElementById", "controls-panel"); p.Truthy() {
		p.Call("addEventListener", "scroll", forget, opts)
	}
	js.Global().Call("addEventListener", "scroll", forget, opts)
	js.Global().Call("addEventListener", "resize", forget, opts)
}

// ── Is anyone actually looking at this module? ─────────────────────────────
//
// The analyzers each begin by checking offsetParent, which was the right
// test when a module could be switched out of the rack: offsetParent is null
// when something up the tree is display:none. It is NOT a test of whether
// the module is on screen, and once the Console's module switches were
// removed nothing is ever display:none — so every analyzer ran its DSP and
// rewrote its readouts on every frame, forever, including the ones scrolled
// out of the drawer.
//
// Measured with the whole rack shown, that is most of what the panel costs:
//
//	everything shown              132 frames / 6s
//	the three live analyzers hidden   184   (+39%)
//	every module hidden               200   (+52%)
//	the whole panel hidden            215   (+63%)
//
// Three modules out of twenty-eight, and hiding them recovers well over half
// of what hiding the entire panel does. Nothing about that is specific to
// analyzers: it is the general case of doing work for a screen nobody can
// see, which is the same thing screenPower exists to stop for the tubes.
//
// So this is the test they should have been making all along. It is a
// viewport intersection, not a display check, and it is throttled for the
// reason every other geometry read here is: getBoundingClientRect makes the
// browser settle layout before it can answer, and asking sixty times a
// second for each of them costs more than it saves.

// onScreenEveryMs is how stale "is this module visible" may get. The same
// quarter second screenPower uses — four layout reads a second rather than
// sixty, and far below the time it takes to scroll and notice.
const onScreenEveryMs = 250

// onScreenAt remembers the last answer per element id.
var onScreenAt = map[string]struct {
	at  float64
	vis bool
}{}

// moduleOnScreen reports whether the element with this id is both in the
// layout and intersecting the viewport.
//
// A missing element is NOT on screen, which is the safe answer: a module
// that has not been built yet has nothing to draw and no readout to write.
func moduleOnScreen(id string) bool {
	if c, ok := onScreenAt[id]; ok && frameNowMs-c.at < onScreenEveryMs && c.at != 0 {
		return c.vis
	}
	vis := measureOnScreen(id)
	onScreenAt[id] = struct {
		at  float64
		vis bool
	}{frameNowMs, vis}
	return vis
}

// measureOnScreen is the real answer.
func measureOnScreen(id string) bool {
	el := doc.Call("getElementById", id)
	if !el.Truthy() || !el.Get("offsetParent").Truthy() {
		return false
	}
	// Skipped mid-resize for the reason the monitors skip it: the rack
	// re-measures every module on each pointer move, and a rectangle read in
	// the middle of that is how a drag comes to cost the model a frame.
	if resizing {
		return true
	}
	r := el.Call("getBoundingClientRect")
	h := js.Global().Get("innerHeight").Float()
	w := js.Global().Get("innerWidth").Float()
	return r.Get("bottom").Float() > 0 && r.Get("top").Float() < h &&
		r.Get("right").Float() > 0 && r.Get("left").Float() < w
}

// invalidateOnScreen forgets every cached answer, for a scroll or a relayout
// that should be noticed at once rather than at the next quarter second.
func invalidateOnScreen() {
	for k := range onScreenAt {
		delete(onScreenAt, k)
	}
}
