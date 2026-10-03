//go:build js && wasm

package dom

import (
	"syscall/js"
	"testing"
)

// fakeElement is an object On can listen on: Node has no DOM.
func fakeElement(connected bool) js.Value {
	el := js.Global().Get("Function").New("c", "return {isConnected: c, addEventListener() {}}").Invoke(connected)
	return el
}

// A handler held from Go keeps its element alive, so On's listeners have to
// be dropped by the arenas: the element a build listened on, once the next
// build has replaced it, loses its handlers; one still on the page keeps
// them.
func TestOnDropsWhatTheNextBuildReplaced(t *testing.T) {
	var arena []js.Func
	gone, stays := fakeElement(false), fakeElement(true)
	RebuildInto(&arena, func() {
		On(gone, "click", func(js.Value, []js.Value) any { return nil })
		On(stays, "click", func(js.Value, []js.Value) any { return nil })
	})
	idGone, idStays := gone.Get(onProp).Int(), stays.Get(onProp).Int()
	if onHandlers[idGone] == nil {
		t.Fatal("the build's own listener was dropped by its own build")
	}
	RebuildInto(&arena, func() {})
	if onHandlers[idGone] != nil {
		t.Error("the replaced element kept its handlers")
	}
	if !gone.Get(onProp).IsUndefined() {
		t.Error("the replaced element kept its number")
	}
	if onHandlers[idStays] == nil {
		t.Error("the element still on the page lost its handlers")
	}
	// Still checked after the build after that: off the page now, it goes.
	stays.Set("isConnected", false)
	RebuildInto(&arena, func() {})
	if onHandlers[idStays] != nil {
		t.Error("an element kept once was never checked again")
	}
}

// Outside any build a listener is for the page's life.
func TestOnOutsideABuildStays(t *testing.T) {
	var arena []js.Func
	el := fakeElement(false)
	On(el, "click", func(js.Value, []js.Value) any { return nil })
	RebuildInto(&arena, func() {})
	if onHandlers[el.Get(onProp).Int()] == nil {
		t.Error("a listener added outside any build was dropped")
	}
}
