//go:build js && wasm

package attractor

import (
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
	if b := doc.Call("getElementById", "pong-restart"); b.Truthy() {
		b.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			pongScoreL, pongScoreR = 0, 0
			pongServeBall(1)
			pongSyncScoreboard()
			return nil
		}))
	}
	// Paddle pots: turning one seizes that paddle (same human window as the
	// keys/touch); while the machine or keys drive the paddle, the pot spins
	// to track it — pongSyncScoreboard writes it back with the guard up.
	wirePad := func(slID, stackID string, pad *float64, human *int) js.Value {
		sl := doc.Call("getElementById", slID)
		stack := doc.Call("getElementById", stackID)
		if !sl.Truthy() || !stack.Truthy() {
			return js.Undefined()
		}
		stack.Call("appendChild", makeKnob(sl, js.Undefined(), false, false, true))
		sl.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			if pongKnobGuard {
				return nil
			}
			*pad = fgFloat(sl) * pongH
			*human = 600
			return nil
		}))
		return sl
	}
	pongPadSlL = wirePad("pong-pad-l", "pong-lstack", &pongPadL, &pongHumanL)
	pongPadSlR = wirePad("pong-pad-r", "pong-rstack", &pongPadR, &pongHumanR)
	if in := doc.Call("getElementById", "stext-in"); in.Truthy() {
		in.Set("value", scopeTextStr)
		in.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			scopeTextStr = strings.ToUpper(in.Get("value").String())
			return nil
		}))
	}
	// Launcher: the drop-height pot (initial condition) + the Drop button
	// that releases from it.
	if h, led, stack := doc.Call("getElementById", "bounce-height"),
		doc.Call("getElementById", "bounce-height-led"),
		doc.Call("getElementById", "bounce-hstack"); h.Truthy() && stack.Truthy() {
		led.Set("value", formatLED(fgFloat(h), 1, 2, false))
		sizeLEDField(led, 0.2, 1, 2, false)
		stack.Call("appendChild", makeKnob(h, js.Undefined(), true, false, true))
		h.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			led.Set("value", formatLED(fgFloat(h), 1, 2, false))
			return nil
		}))
		led.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			if v, err := strconv.ParseFloat(led.Get("value").String(), 64); err == nil {
				h.Set("value", strconv.FormatFloat(v, 'f', 2, 64))
				h.Call("dispatchEvent", js.Global().Get("Event").New("input"))
			}
			return nil
		}))
	}
	if b := doc.Call("getElementById", "bounce-drop"); b.Truthy() {
		b.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			bounceX, bounceY = -1.2, bounceDropHeight()
			bounceVY = 0
			bounceVX = float64(bounceDrift)
			if jamRand() < 0.5 {
				bounceVX = -bounceVX
			}
			return nil
		}))
	}
}

// bounceDropHeight reads the Launcher's height pot (court y for a release).
func bounceDropHeight() float64 {
	if h := doc.Call("getElementById", "bounce-height"); h.Truthy() {
		if v := fgFloat(h); v > 0 {
			return v
		}
	}
	return 0.9
}
