//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/led"
	"strconv"
	"strings"
	"syscall/js"
)

var (
	pongKnobGuard bool     // set while the game writes the pots back
	pongPadSlL    js.Value // the Scoreboard paddle pots (motorized)
	pongPadSlR    js.Value
)

// buildDemoModules wires the mode-scoped demo modules' controls (static
// markup in panelhtml_js.go, shown/hidden by each mode's sync hook):
// Scoreboard's Restart, Banner's text field, Launcher's Drop. Called once
// from Run.
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
	wirePad := func(slID, stackID string, pad *float64, human *int) js.Value {
		sl := dom.Doc.Call("getElementById", slID)
		stack := dom.Doc.Call("getElementById", stackID)
		if !sl.Truthy() || !stack.Truthy() {
			return js.Undefined()
		}
		stack.Call("appendChild", makeKnob(sl, js.Undefined(), false, false, true))
		sl.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, a []js.Value) any {
			if pongKnobGuard {
				return nil
			}
			*pad = fgFloat(sl) * pongH
			*human = 600
			return nil
		}))
		return sl
	}
	pongPadSlL = wirePad("pong-pad-l", "pong-lstack", &pong.padL, &pong.humanL)
	pongPadSlR = wirePad("pong-pad-r", "pong-rstack", &pong.padR, &pong.humanR)
	if in := dom.Doc.Call("getElementById", "stext-in"); in.Truthy() {
		in.Set("value", ftext.str)
		in.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, a []js.Value) any {
			ftext.str = strings.ToUpper(in.Get("value").String())
			return nil
		}))
	}
	// Launcher: the drop-height pot (initial condition) + the Drop button
	// that releases from it.
	if h, ledEl, stack := dom.Doc.Call("getElementById", "bounce-height"),
		dom.Doc.Call("getElementById", "bounce-height-led"),
		dom.Doc.Call("getElementById", "bounce-hstack"); h.Truthy() && stack.Truthy() {
		ledEl.Set("value", led.Format(fgFloat(h), 1, 2, false))
		sizeLEDField(ledEl, 0.2, 1, 2, false)
		stack.Call("appendChild", makeKnob(h, js.Undefined(), true, false, true))
		h.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, a []js.Value) any {
			ledEl.Set("value", led.Format(fgFloat(h), 1, 2, false))
			return nil
		}))
		ledEl.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, a []js.Value) any {
			if v, err := strconv.ParseFloat(ledEl.Get("value").String(), 64); err == nil {
				h.Set("value", strconv.FormatFloat(v, 'f', 2, 64))
				h.Call("dispatchEvent", js.Global().Get("Event").New("input"))
			}
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

// bounceDropHeight reads the Launcher's height pot (court y for a release).
func bounceDropHeight() float64 {
	if h := dom.Doc.Call("getElementById", "bounce-height"); h.Truthy() {
		if v := fgFloat(h); v > 0 {
			return v
		}
	}
	return 0.9
}
