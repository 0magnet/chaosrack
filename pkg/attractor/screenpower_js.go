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

// The three screens.
var (
	scopeScreenPower = screenPower{switchID: "scope-beam"}
	recScreenPower   = screenPower{switchID: "rec-mon-on"}
	deskScreenPower  = screenPower{switchID: "desk-mon-on"}
)

// wireScreenPower hooks each screen's switch up so flipping it is noticed
// at once rather than at the next check.
func wireScreenPower() {
	for _, p := range []*screenPower{&scopeScreenPower, &recScreenPower, &deskScreenPower} {
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
}
