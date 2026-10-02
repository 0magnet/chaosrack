//go:build js && wasm

package attractor

import (
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
)

var (
	pongKnobGuard bool                    // set while the game writes the pots back
	pongPots      = map[string]js.Value{} // the paddle pots, once found
)

// pongPot is one of Pong's paddle pots: the bank knob pong-pad-l or -r. Looked
// up the first time it is needed, because the bank builds it after this is
// wired, and kept: the game writes it twenty times a second.
func pongPot(side string) js.Value {
	if p := pongPots[side]; p.Truthy() {
		return p
	}
	p := dom.Doc.Call("getElementById", "pong-pad-"+side)
	pongPots[side] = p
	return p
}

// buildDemoModules wires the demo models' own parts (the holder in
// panelhtml_js.go, placed by modelparts_js.go): Pong's Restart and paddle
// pots, the Banner's text, the Launcher's Drop. Called once from Run.
func buildDemoModules() {
	if b := dom.Doc.Call("getElementById", "pong-restart"); b.Truthy() {
		b.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, a []js.Value) any {
			pong.scoreL, pong.scoreR = 0, 0
			pong.serveBall(1)
			pong.syncScoreboard()
			return nil
		}))
	}
	// Paddle pots: turning one seizes that paddle (same human window as the
	// keys/touch); while the machine or keys drive the paddle, the pot spins
	// to track it — pong.syncScoreboard writes it back with the guard up.
	// On the document, since the pots are bank knobs built after this.
	dom.Doc.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, a []js.Value) any {
		if pongKnobGuard || len(a) == 0 {
			return nil
		}
		t := a[0].Get("target")
		if !t.Truthy() {
			return nil
		}
		switch t.Get("id").String() {
		case "pong-pad-l":
			pong.padL, pong.humanL = fgFloat(t)*pongH, 600
		case "pong-pad-r":
			pong.padR, pong.humanR = fgFloat(t)*pongH, 600
		}
		return nil
	}), true)
	if in := dom.Doc.Call("getElementById", "stext-in"); in.Truthy() {
		in.Set("value", ftext.str)
		in.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, a []js.Value) any {
			ftext.str = strings.ToUpper(in.Get("value").String())
			return nil
		}))
	}
	buildSTLFileModule()
	if b := dom.Doc.Call("getElementById", "bounce-drop"); b.Truthy() {
		b.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, a []js.Value) any {
			ball.x, ball.y = -1.2, bounceDropHeight()
			ball.vy = 0
			ball.vx = float64(ball.drift)
			if jamRand() < 0.5 {
				ball.vx = -ball.vx
			}
			return nil
		}))
	}
}

// bounceDropHeight is the height knob's setting: court y for a release.
func bounceDropHeight() float64 {
	if ball.height > 0 {
		return float64(ball.height)
	}
	return 0.9
}
