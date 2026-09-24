//go:build js && wasm

package dom

import "syscall/js"

// The js.Func arena.
//
// The panel's content is wiped and rebuilt on every mode change and on Reset
// All, but js.FuncOf handles pinned in wasm_exec's reference table do not die
// with the DOM they were attached to — historically every rebuild leaked all
// of the previous panel's listener closures, about ten per parameter unit and
// more with audio modulation on.
//
// The cure is scoping rather than bookkeeping. FuncOf behaves exactly like
// js.FuncOf, except that while a rebuild is in progress every func it makes
// lands in an arena: anything created in that window IS rebuildable content,
// by construction, so nothing has to remember to register it. The next rebuild
// releases the previous arena before wiping the DOM. Funcs made outside the
// window — the boot wiring, the module builders that live for the page — are
// untouched.
var (
	panelFuncs   []js.Func
	panelCollect bool
	// altArena, when set, takes the funcs instead of the panel arena. It is
	// for a widget that lives outside the panel but is rebuilt on its own
	// schedule — the sweep dial, whose ring is remade per mode, and the focus
	// dial, whose ring is remade per grid size. Such a widget cannot use the
	// panel arena: the panel rebuilds far more often than it does, and would
	// release listeners still attached to elements on screen.
	altArena *[]js.Func
)

// FuncOf is this app's js.FuncOf: identical outside a rebuild, and
// arena-registered inside one.
func FuncOf(fn func(this js.Value, args []js.Value) any) js.Func {
	f := js.FuncOf(fn)
	switch {
	case altArena != nil:
		*altArena = append(*altArena, f)
	case panelCollect:
		panelFuncs = append(panelFuncs, f)
	}
	return f
}

// StartPanelBuild frees every func from the previous panel build and starts
// collecting this one. It returns the function that stops collecting:
//
//	defer dom.StartPanelBuild()()
//
// One call rather than the three moving parts it replaces — release, raise the
// flag, lower the flag — because the ORDER is the correctness. The release has
// to come first and in the same synchronous pass as the wipe that follows, so
// that no event can reach a freed func in between; and the flag has to come
// down on every path out, including a panic. A caller doing that by hand is a
// caller who can get it wrong, and the leak it produces is invisible until the
// tab is out of memory.
//
// Shaped for defer rather than taking the build as a callback because the
// build is the whole body of a long function, and wrapping it in a closure to
// satisfy this would indent two hundred lines to say nothing.
func StartPanelBuild() (done func()) {
	for _, f := range panelFuncs {
		f.Release()
	}
	panelFuncs = panelFuncs[:0]
	panelCollect = true
	return func() { panelCollect = false }
}

// RebuildInto releases whatever the last call put in arena, then runs build
// with every func it creates landing there.
//
// The release comes first and in the same synchronous pass as the rebuild, for
// BuildPanel's reason: nothing can dispatch an event to a freed func between
// the two.
func RebuildInto(arena *[]js.Func, build func()) {
	for _, f := range *arena {
		f.Release()
	}
	*arena = (*arena)[:0]
	prev := altArena
	altArena = arena
	defer func() { altArena = prev }()
	build()
}

// AltArena reports the arena currently collecting, for a test that needs to
// save and restore it.
func AltArena() *[]js.Func { return altArena }

// SetAltArena points the collector at an arena, returning the previous one.
func SetAltArena(a *[]js.Func) *[]js.Func {
	prev := altArena
	altArena = a
	return prev
}
