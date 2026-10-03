//go:build js && wasm

package dom

// Listeners that live as long as their element stays on the page.
//
// A listener made with FuncOf lives as long as the build it was made in: the
// next panel build, or the next RebuildInto of its arena, releases it. That is
// right for a listener on an element the build then replaces, and wrong for
// one on an element that stays — the next event on it throws "call to
// released function". Telling the two apart was every caller's job, and the
// sweep dial's answer was to clone its select on every rebuild (the old
// freshSelect) so nothing old could be left on it, which broke anything else
// still holding the old one.
//
// On ties a listener to its element instead. No js.Func is made for it: one
// per event type is shared by the whole page, and finds the element's Go
// handlers by a number kept on the element.
//
// The handlers are dropped by the arenas, not by the garbage collector. A
// handler nearly always holds its own element (a slider's reads the slider),
// and a js.Value held from Go keeps its object alive in wasm_exec's table, so
// an element with a handler is never collected and a FinalizationRegistry on
// it would never fire; TinyGo does not finalize js.Values at all. So a
// listener added during a panel build or a RebuildInto is recorded with that
// arena, and when the arena's next build has run, each recorded element no
// longer on the page has its handlers dropped — the old panel, by then
// replaced. One still on the page (a persistent element a build listens on)
// keeps them, and is checked again after the build after that. A listener
// added outside any build is for the page's life, as FuncOf's are.
//
// OnAs is On under a key: a later OnAs with the same key on the same element
// and event replaces the earlier one. It is how a widget rebuilt on the same
// element (a dial refilled with a new model's options) says "this is the
// display's listener", rather than adding one more each time.
//
// A handler's return value is ignored, as addEventListener ignores it; the
// listener is a plain bubble-phase one. A listener that needs capture,
// passive or once is made with FuncOf.

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
	// onRefs is a WeakRef to each element an arena recorded, so the sweep
	// can ask whether it is still on the page without keeping it alive.
	onRefs = map[int]js.Value{}
	// panelIDs and altIDs are the elements each arena recorded: the panel
	// build's, and each RebuildInto arena's.
	panelIDs []int
	altIDs   = map[*[]js.Func][]int{}
)

// onProp is where an element keeps its number.
const onProp = "__goOn"

// On calls fn on el's event name for as long as el is on the page.
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
	record(el, id)
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
// first time it is asked for, or again once a sweep has dropped them.
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
	return id
}

// record notes element id with the arena collecting, if one is.
func record(el js.Value, id int) {
	if altArena == nil && !panelCollect {
		return
	}
	if _, ok := onRefs[id]; !ok {
		onRefs[id] = js.Global().Get("WeakRef").New(el)
	}
	if altArena != nil {
		altIDs[altArena] = append(altIDs[altArena], id)
	} else {
		panelIDs = append(panelIDs, id)
	}
}

// sweep drops the handlers of each of ids whose element is no longer on the
// page, and returns the rest, to be checked again after the next build.
func sweep(ids []int) []int {
	var kept []int
	seen := make(map[int]bool, len(ids))
	gone := map[int]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		ref, ok := onRefs[id]
		if !ok {
			continue
		}
		el := ref.Call("deref")
		if el.Truthy() && el.Get("isConnected").Bool() {
			kept = append(kept, id)
			continue
		}
		delete(onHandlers, id)
		delete(onRefs, id)
		if el.Truthy() {
			el.Delete(onProp)
		}
		gone[id] = true
	}
	if arenaCheck.Truthy() {
		for k := range onSites {
			var id int
			if _, err := fmt.Sscan(k, &id); err == nil && gone[id] {
				delete(onSites, k)
			}
		}
		// What a leak walk reads: how many elements have handlers.
		js.Global().Set("__domOnLive", len(onHandlers))
	}
	return kept
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
	// Never released, and on every element On has touched: the arena
	// check must not remember the elements it is added to, or it would keep
	// every one of them alive.
	f.Value.Set("__goShared", true)
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
	// The first caller outside this file. TinyGo cannot say (runtime.Caller
	// is never ok there), and every site would then look like one.
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
	if file == "" {
		return
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
