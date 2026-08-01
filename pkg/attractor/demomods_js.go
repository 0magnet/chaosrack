//go:build js && wasm

package attractor

import (
	"strings"
	"syscall/js"
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
	if in := doc.Call("getElementById", "stext-in"); in.Truthy() {
		in.Set("value", scopeTextStr)
		in.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			scopeTextStr = strings.ToUpper(in.Get("value").String())
			return nil
		}))
	}
	if b := doc.Call("getElementById", "bounce-drop"); b.Truthy() {
		b.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			bounceX, bounceY = -1.2, 0.9
			bounceVY = 0
			bounceVX = float64(bounceDrift)
			if jamRand() < 0.5 {
				bounceVX = -bounceVX
			}
			return nil
		}))
	}
}
