//go:build js && wasm

package attractor

import (
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// crashDelayMs is how long after the boot a requested crash waits, so the
// rack is up and drawing when the program goes.
const crashDelayMs = 1500

// crashRequested reports whether the page was opened with ?crash.
func crashRequested() bool {
	loc := js.Global().Get("location")
	return loc.Truthy() && js.Global().Get("URLSearchParams").New(loc.Get("search")).Call("has", "crash").Bool()
}

// crashOnRequest panics shortly after the boot when the page was opened with
// ?crash.
//
// The page says when the program is gone (window.__crCrash in the template),
// and the two runtimes die differently: standard Go ends the program, so
// go.run resolves; TinyGo traps, and a WebAssembly.RuntimeError is thrown
// out of whichever callback was running. Neither can be checked without a
// panic to look at, and one in a callback is how a real one arrives.
func crashOnRequest() {
	if !crashRequested() {
		return
	}
	js.Global().Call("setTimeout", dom.FuncOf(func(js.Value, []js.Value) any {
		panic("chaosrack: crash requested by ?crash")
	}), crashDelayMs)
}
