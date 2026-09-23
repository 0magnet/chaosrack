//go:build js && wasm

// Package dom owns the browser handles the whole app reaches for, and the
// lifetimes of the callbacks hung off them.
//
// It exists because of what those handles were doing to the package they lived
// in. `doc` was one package-level variable in pkg/attractor touched by 91 of
// its 134 browser files, which meant no file that mentions the document could
// leave that package — not to be tested, not to be reused, not to be given a
// name of its own. A shared global is not merely untidy; it is a wall around
// everything that touches it.
//
// Two things live here and they belong together, which is worth saying because
// "the DOM and a list of function handles" does not sound like one subject. It
// is: a js.Func handed to addEventListener is pinned in wasm_exec's reference
// table for as long as the page lives, whether or not the element it was
// attached to still exists. The arena below is the lifetime of a callback
// EXPRESSED AS the lifetime of the DOM it was attached to, so it is exactly as
// much about the document as the document handle is.
package dom

import "syscall/js"

// Doc and Body are the document and its body.
//
// Variables rather than functions, and assigned at Init rather than at package
// var time, for the reason the original comment in glstate_js.go gives: when
// this is imported as a library the host's DOM may not exist yet, and
// getElementById on a null document panics rather than returning nothing.
//
// They are writable on purpose. The rack's layout tests replace the document
// with a fake one and assert on what gets built, which is the only way to test
// DOM-building code on a host with no DOM — see Swap.
var (
	Doc  js.Value
	Body js.Value
)

// Init takes the handles. Call it once the page exists.
func Init() {
	Doc = js.Global().Get("document")
	Body = Doc.Get("body")
}

// Swap replaces the document and returns the previous one, for a test that
// wants to build against a fake and put the real one back:
//
//	defer dom.Swap(dom.Swap(fake))
//
// A function rather than direct assignment so the seam is a named thing a
// reader can find, rather than a global that anything might be writing to.
func Swap(v js.Value) js.Value {
	prev := Doc
	Doc = v
	return prev
}
