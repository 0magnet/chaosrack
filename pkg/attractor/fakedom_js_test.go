//go:build js && wasm

package attractor

import "syscall/js"

// A fake DOM, for testing the js/wasm half of this package under Node.
//
// `make test-wasm` runs the suite compiled for js/wasm through Go's own
// go_js_wasm_exec wrapper, which proves the wasm build compiles and that the
// untagged logic behaves there. What it did NOT do is exercise a single line
// of js-tagged code, because every test file was untagged — so the half of
// the package that only exists in a browser had no tests at all.
//
// Node has no document, no window layout and no rendering. It does have a
// complete JavaScript engine, which is enough: an element is an object with
// the handful of properties the code under test reads, and geometry is
// whatever the test says it is. That makes the layout arithmetic testable
// without a browser, and it is the arithmetic that has had the bugs.

// fakeEl builds an element with the properties the bay layout reads.
type fakeEl struct{ js.Value }

func newFakeEl(class string) fakeEl {
	obj := js.Global().Get("Object").New()
	obj.Set("className", class)
	obj.Set("offsetTop", 0)
	obj.Set("offsetLeft", 0)
	obj.Set("offsetWidth", 0)
	obj.Set("offsetHeight", 0)
	obj.Set("style", js.Global().Get("Object").New())
	obj.Set("children", js.Global().Get("Array").New())

	// classList.contains, over the space-separated className.
	cl := js.Global().Get("Object").New()
	cl.Set("contains", js.FuncOf(func(this js.Value, args []js.Value) any {
		want := args[0].String()
		for _, c := range splitFields(obj.Get("className").String()) {
			if c == want {
				return true
			}
		}
		return false
	}))
	cl.Set("add", js.FuncOf(func(js.Value, []js.Value) any { return nil }))
	cl.Set("remove", js.FuncOf(func(js.Value, []js.Value) any { return nil }))
	obj.Set("classList", cl)

	obj.Set("appendChild", js.FuncOf(func(this js.Value, args []js.Value) any {
		obj.Get("children").Call("push", args[0])
		return args[0]
	}))
	return fakeEl{obj}
}

// at places the element, the way a browser's layout would have.
func (e fakeEl) at(left, top, w, h float64) fakeEl {
	e.Set("offsetLeft", left)
	e.Set("offsetTop", top)
	e.Set("offsetWidth", w)
	e.Set("offsetHeight", h)
	return e
}

// newFakeHost builds a .modules container holding the given children.
func newFakeHost(clientWidth float64, kids ...fakeEl) js.Value {
	host := newFakeEl("modules")
	host.Set("clientWidth", clientWidth)
	arr := js.Global().Get("Array").New()
	for _, k := range kids {
		arr.Call("push", k.Value)
	}
	host.Set("children", arr)
	// querySelectorAll is only used to clear the previous pass; an empty
	// result is the honest answer for a container nothing has been added to.
	host.Set("querySelectorAll", js.FuncOf(func(js.Value, []js.Value) any {
		return js.Global().Get("Array").New()
	}))
	return host.Value
}

func splitFields(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ' ' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
