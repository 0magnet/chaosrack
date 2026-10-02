//go:build js && wasm

package dom

// Listeners that live as long as their element.
//
// A listener made with FuncOf lives as long as the build it was made in: the
// next panel build, or the next RebuildInto of its arena, releases it. That is
// right for a listener on an element the build then replaces, and wrong for
// one on an element that stays — the next event on it throws "call to
// released function". Telling the two apart was every caller's job, and the
// sweep dial's answer was to clone its select on every rebuild (freshSelect)
// so nothing old could be left on it, which broke anything else still
// holding the old one.
//
// On ties a listener to its element instead. No js.Func is made for it: one
// per event type is shared by the whole page, and finds the element's Go
// handlers by a number kept on the element. When the element is collected a
// FinalizationRegistry says so and its handlers are forgotten. A listener on
// an element that stays keeps working; one on an element that goes, goes.
//
// OnAs is On under a key: a later OnAs with the same key on the same element
// and event replaces the earlier one. It is how a widget rebuilt on the same
// element (a dial refilled with a new model's options) says "this is the
// display's listener", rather than adding one more each time.

import (
	"fmt"
	"runtime"
	"strings"
	"syscall/js"
)

// handler is one listener: its key ("" for none) and its function.
type handler struct {
	key string
	fn  func(this js.Value, args []js.Value) any
}

var (
	// onHandlers are every element's listeners, by the number On gave it
	// and the event.
	onHandlers = map[int]map[string][]handler{}
	onNext     = 1
	// onShared is the one js.Func each event type is dispatched through.
	onShared = map[string]js.Func{}
	// onGone is told when an element On has numbered is collected.
	onGone js.Value
)

// onProp is where an element keeps its number.
const onProp = "__goOn"

// On calls fn on el's event name for as long as el exists.
func On(el js.Value, name string, fn func(this js.Value, args []js.Value) any) {
	OnAs(el, name, "", fn)
}

// OnAs is On under key: it replaces what an earlier OnAs gave el's name
// under the same key. An empty key replaces nothing.
func OnAs(el js.Value, name, key string, fn func(this js.Value, args []js.Value) any) {
	if !el.Truthy() {
		return
	}
	id := elementNumber(el)
	evs := onHandlers[id]
	hs, had := evs[name]
	if key != "" {
		for i := range hs {
			if hs[i].key == key {
				hs[i].fn = fn
				return
			}
		}
	}
	if arenaCheck.Truthy() && key == "" {
		checkTwice(el, id, name)
	}
	evs[name] = append(hs, handler{key, fn})
	if !had {
		el.Call("addEventListener", name, sharedFor(name))
	}
}

// elementNumber is the number el's handlers are kept under, given the
// first time it is asked for.
func elementNumber(el js.Value) int {
	if v := el.Get(onProp); v.Type() == js.TypeNumber {
		if _, ok := onHandlers[v.Int()]; ok {
			return v.Int()
		}
	}
	id := onNext
	onNext++
	el.Set(onProp, id)
	onHandlers[id] = map[string][]handler{}
	if !onGone.Truthy() {
		onGone = js.Global().Get("FinalizationRegistry").New(js.FuncOf(func(_ js.Value, a []js.Value) any {
			delete(onHandlers, a[0].Int())
			return nil
		}))
	}
	// document and window are never collected; registering them is harmless.
	onGone.Call("register", el, id)
	return id
}

// sharedFor is the js.Func every element's name listeners go through.
func sharedFor(name string) js.Func {
	if f, ok := onShared[name]; ok {
		return f
	}
	f := js.FuncOf(func(this js.Value, args []js.Value) any {
		v := this.Get(onProp)
		if v.Type() != js.TypeNumber {
			return nil
		}
		// A copy: a handler may add or replace handlers as it runs.
		hs := append([]handler(nil), onHandlers[v.Int()][name]...)
		for _, h := range hs {
			h.fn(this, args)
		}
		return nil
	})
	onShared[name] = f
	return f
}

// onSites are, under the arena check, the call sites that have given an
// element an unkeyed listener, by element and event.
var onSites = map[string]bool{}

// checkTwice reports, under the arena check, the same line of code giving
// one element the same event twice without a key: a build that ran again
// over an element it does not replace, which On would otherwise let pile
// up a listener per run. A real second listener from one line is written
// with OnAs.
func checkTwice(el js.Value, id int, name string) {
	// The first caller outside this file.
	file, line := "", 0
	for i := 1; i < 8; i++ {
		_, f, l, ok := runtime.Caller(i)
		if !ok {
			break
		}
		if !strings.HasSuffix(f, "/dom/on.go") {
			file, line = f, l
			break
		}
	}
	k := fmt.Sprintf("%d %s %s:%d", id, name, file, line)
	if !onSites[k] {
		onSites[k] = true
		return
	}
	who := el.Get("id").String()
	if who == "" || who == "<undefined>" {
		who = el.Get("tagName").String()
	}
	msg := fmt.Sprintf("dom: %s listener added twice to %s from %s:%d", name, who, file, line)
	js.Global().Get("console").Call("error", msg)
	js.Global().Get("__domArenaLeaks").Call("push", msg)
}
