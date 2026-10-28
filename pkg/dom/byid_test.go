//go:build js && wasm

package dom

import (
	"syscall/js"
	"testing"
)

// ByID looks an element up once, and again only when the one kept has left
// the page.
func TestByIDKeepsTheElementUntilItLeavesThePage(t *testing.T) {
	fake := js.Global().Get("Function").New(`
		const d = {looked: 0, el: {isConnected: true}};
		d.getElementById = (id) => { d.looked++; return id === "x" ? d.el : null; };
		return d;`).Invoke()
	defer Swap(Swap(fake))
	var kept js.Value
	for range 3 {
		if !ByID(&kept, "x").Truthy() {
			t.Fatal("no element")
		}
	}
	if n := fake.Get("looked").Int(); n != 1 {
		t.Fatalf("looked up %d times, want once", n)
	}
	fake.Get("el").Set("isConnected", false)
	fake.Set("el", js.Global().Get("Function").New("return {isConnected: true}").Invoke())
	if !ByID(&kept, "x").Get("isConnected").Bool() {
		t.Fatal("still the element that left the page")
	}
	if n := fake.Get("looked").Int(); n != 2 {
		t.Fatalf("looked up %d times, want twice", n)
	}
	var none js.Value
	if ByID(&none, "missing").Truthy() {
		t.Fatal("an element for a missing id")
	}
}
