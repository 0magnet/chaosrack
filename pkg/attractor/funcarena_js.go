//go:build js && wasm

package attractor

import "syscall/js"

// Panel-scoped js.Func arena. The param/Modulation/EQ panel content is wiped
// and rebuilt on every mode change (and Reset All), but js.FuncOf handles
// pinned in wasm_exec's ref table don't die with their DOM — historically
// every rebuild leaked all of the old panel's listener closures (~10 per
// param unit, more with audio-mod on).
//
// The cure is scoping, not bookkeeping: trackedFuncOf behaves exactly like
// js.FuncOf, except that while buildParamPanel is rebuilding (panelCollect)
// every func created lands in the arena — anything created in that window IS
// rebuildable panel content, by construction. The next rebuild releases the
// previous arena before wiping the DOM. Funcs created outside the window
// (Run wiring, module builders that live for the page) are untouched.

var (
	panelFuncs   []js.Func
	panelCollect bool
)

// trackedFuncOf is the package's js.FuncOf: identical outside a panel
// rebuild; arena-registered inside one.
func trackedFuncOf(fn func(this js.Value, args []js.Value) interface{}) js.Func {
	f := js.FuncOf(fn)
	if panelCollect {
		panelFuncs = append(panelFuncs, f)
	}
	return f
}

// releasePanelFuncs frees every func from the PREVIOUS panel build. Call
// before wiping the panel DOM — the release and the wipe happen in the same
// synchronous pass, so no event can reach a released func.
func releasePanelFuncs() {
	for _, f := range panelFuncs {
		f.Release()
	}
	panelFuncs = panelFuncs[:0]
}
