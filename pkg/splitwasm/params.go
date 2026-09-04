//go:build js && wasm

// Package splitwasm is the prototype behind the renderer / control-plane wasm
// split. It builds three ways from one body of code:
//
//	RunMonolith — renderer and control plane in ONE module, which is the shape
//	              chaosrack has today
//	RunRenderer — the frame-budget half on its own: geometry, GL, camera, the
//	              rAF loop, and nothing else
//	RunControl  — everything that runs on events rather than every frame
//
// The split is by frame budget, not by feature. What decides it is that
// TinyGo's collector is conservative and marks the whole live heap on every
// cycle: measured on chaosrack, ~50 ms of runtime.markRoot per collection over
// an 18 MB, 25000-object, pointer-dense heap, a collection every ~370 ms, and
// a 66-100 ms stall each time. The renderer needs almost none of those 18 MB,
// so giving it its own runtime gives it its own — small — heap to mark.
package splitwasm

import "syscall/js"

// The parameter block the control plane writes and the renderer reads.
//
// The rule this exists to keep: nothing crosses the wasm boundary inside the
// render loop. Crossing costs per crossing, so a knob does not call the
// renderer and the renderer does not call out to ask what a knob says. The
// control plane writes values into a Float32Array held by the page; the
// renderer copies that array into its own linear memory once per frame with a
// single js.CopyBytesToGo (one bulk memcpy, not one call per parameter) and
// then reads it as ordinary Go memory.
//
// Two wasm modules cannot literally share one linear memory — each TinyGo
// instance owns its own — so the JS-side typed array is the shared page, and
// the once-per-frame bulk copy is what "reads it as plain memory" costs in
// practice: one crossing per frame instead of one per parameter.
const (
	PRotX = iota // spin rate, X
	PRotY        // spin rate, Y
	PRotZ        // spin rate, Z
	PZoom        // camera distance offset
	PPanX
	PPanY
	PLat   // globe parallels
	PLon   // globe meridians
	PTwist // meridian twist
	PAuto  // auto-rotate, 0 or 1
	PR     // line color
	PG
	PB
	PGeomSeq // bumped by the control plane whenever geometry must be rebuilt
	ParamCount
)

// SharedName is the property on window holding the Float32Array both modules
// use. The page creates it before either module starts so neither has to wait
// for the other.
const SharedName = "__splitParams"

// Defaults are what the page starts at, and what the control plane's knobs are
// initialized from.
var Defaults = [ParamCount]float32{
	PRotX: 0, PRotY: 0.9, PRotZ: 0,
	PZoom: 0, PPanX: 0, PPanY: 0,
	PLat: 18, PLon: 36, PTwist: 0,
	PAuto: 1,
	PR:    0.45, PG: 0.85, PB: 1.0,
	PGeomSeq: 0,
}

// EnsureShared returns the shared Float32Array, creating and priming it if the
// page has not already. Either module may be first to load.
func EnsureShared() js.Value {
	g := js.Global()
	v := g.Get(SharedName)
	if !v.IsUndefined() && !v.IsNull() {
		return v
	}
	v = g.Get("Float32Array").New(ParamCount)
	for i := 0; i < ParamCount; i++ {
		v.SetIndex(i, Defaults[i])
	}
	g.Set(SharedName, v)
	return v
}
