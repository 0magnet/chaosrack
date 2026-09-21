//go:build js && wasm

package attractor

import "syscall/js"

// Boot timing.
//
// The page comes up in eleven seconds and the WebAssembly is compiled and
// running after five hundred milliseconds of that, so the other ten and a
// half are this program building the panel. Which part of it was a guess
// until these marks existed: a console line says what happened in what
// order, a mark says when, and the difference between "compiling a
// twenty-four megabyte binary is slow" and "one loop reads the layout once
// per control" is the difference between a fix and a shrug.
//
// They cost one js call each and land in the same performance timeline as
// the loader's own marks, so `performance.getEntriesByType('mark')` in a
// console — or uitool — reads the whole boot end to end.
func bootMark(name string) {
	if !doc.Truthy() {
		return
	}
	p := js.Global().Get("performance")
	if !p.Truthy() {
		return
	}
	p.Call("mark", "go-"+name)
}
