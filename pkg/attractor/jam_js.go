//go:build js && wasm

package attractor

// Jam / attract mode (Motion > Jam): the app performs itself — a light
// in-app port of the chaos-monkey demo choreography. Every 12–20 seconds it
// hops to a random flow mode; hops sometimes flip persist on (the CRT paint
// look), and always set fresh gentle spin rates. Everything goes through the
// real controls (select dispatch, slider input events) so the panel, the
// permalink and every registry behavior stay truthful. Speaker-output
// switches are never touched.

import (
	"strconv"
	"syscall/js"
)

var (
	jamOn   bool
	jamNext float64 // ms timestamp of the next hop
)

func jamRand() float64 { return js.Global().Get("Math").Call("random").Float() }

// jamTick runs from the render loop; cheap no-op unless armed and due.
func jamTick(nowMs float64) {
	if !jamOn || nowMs < jamNext {
		return
	}
	jamNext = nowMs + 12000 + jamRand()*8000
	jamHop()
}

func jamHop() {
	// Hop to a different flow mode (attractors incl. the Sprott catalog and
	// hyperchaos; custom excluded — its panel is an editor, not a show).
	keys := ModeKeys(ClassFlow3D, ClassFlow4D)
	var pool []string
	for _, k := range keys {
		if k != "custom" && k != selectedMode {
			pool = append(pool, k)
		}
	}
	if len(pool) == 0 {
		return
	}
	next := pool[int(jamRand()*float64(len(pool)))%len(pool)]
	if sel := doc.Call("getElementById", "mode-select"); sel.Truthy() {
		sel.Set("value", next)
		sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
	}
	// Persist paint look ~35% of hops.
	if p := doc.Call("getElementById", "persist-trail"); p.Truthy() {
		p.Set("checked", jamRand() < 0.35)
		p.Call("dispatchEvent", js.Global().Get("Event").New("change"))
	}
	// Fresh gentle spin: always some Y, sometimes a touch of X.
	setSpin := func(axis string, v float64) {
		if sl := doc.Call("getElementById", "rotation-controls-"+axis); sl.Truthy() {
			sl.Set("value", strconv.FormatFloat(float64(int(v*10))/10, 'f', 1, 64))
			sl.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		}
	}
	ySpin := 0.1 + jamRand()*0.2
	if jamRand() < 0.5 {
		ySpin = -ySpin
	}
	setSpin("y", ySpin)
	if jamRand() < 0.4 {
		setSpin("x", (jamRand()-0.5)*0.2)
	} else {
		setSpin("x", 0)
	}
	syncKnobs()
}

func wireJamSwitch() {
	sw := doc.Call("getElementById", "jam-sw")
	if !sw.Truthy() {
		return
	}
	sw.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		jamOn = sw.Get("checked").Bool()
		jamNext = 0 // first hop on the next frame
		return nil
	}))
}
