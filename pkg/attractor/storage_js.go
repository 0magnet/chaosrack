//go:build js && wasm

package attractor

import "syscall/js"

// Browser storage that cannot take the page down with it.
//
// WHY THIS FILE EXISTS. Every call into JavaScript from Go can throw, and a
// throw arriving back in Go is a panic — which in wasm is not one broken
// feature but the end of the program. The runtime exits, and every listener the
// page is still holding is now calling into something that is gone: the canvas
// keeps its last frame, the knobs turn and do nothing, and nothing in the UI
// says why. Measured while hunting this: patching setItem to throw and then
// toggling a single module switch killed the whole app, and the driver reported
// "30 calls into an exited Go program".
//
// localStorage throws for reasons that have nothing to do with this app being
// wrong:
//
//   - the origin's few megabytes are full, and they are shared with everything
//     else served from the same domain rather than owned by this page;
//   - Safari in private browsing refuses writes outright;
//   - a browser set to block site data throws on the PROPERTY ACCESS, before
//     any method is called — which is why the lookup is inside the recover here
//     rather than done once and cached.
//
// None of those are worth an app for. A failed access means the setting is not
// remembered, which is what "storage is unavailable" ought to cost and no more.
//
// THE CATCH IS IN JAVASCRIPT, not here, and that is the load-bearing detail. A
// deferred recover() in these functions is enough for the standard Go build —
// and does nothing at all in the TinyGo one, where the identical failure still
// went runtime._panic -> unreachable straight through lsSet. Two builds ship
// from this repo and both are somebody's runtime, so the throw is stopped in
// the page (window.__crStore, in index.tmpl.html) where both agree. The
// recover() below is kept as a second line for any host that serves this wasm
// without the shim.
//
// Reads treat an empty value as absent, which the hand-written accessors this
// replaced all did too, by testing the returned value for truthiness.

// lsFailed is set the first time storage refuses, so the console says it once
// rather than on every drag of a panel edge.
var lsFailed bool

func lsNote(op, key string) {
	if lsFailed {
		return
	}
	lsFailed = true
	if c := js.Global().Get("console"); c.Truthy() {
		c.Call("warn", "chaosrack: browser storage unavailable ("+op+" "+key+
			"); settings will not be remembered this session")
	}
}

// crStore is the page's shim, whose whole job is to turn a throw into a return
// value before it can reach Go. It is in the page rather than built here with
// the Function constructor so that a Content-Security-Policy without
// 'unsafe-eval' cannot take it away.
func crStore() js.Value { return js.Global().Get("__crStore") }

// lsGet reads a key. ok is false when storage is unavailable or the key unset.
func lsGet(key string) (s string, ok bool) {
	defer func() {
		if recover() != nil {
			s, ok = "", false
			lsNote("read", key)
		}
	}()
	if st := crStore(); st.Truthy() {
		v := st.Call("get", key).String()
		return v, v != ""
	}
	ls := js.Global().Get("localStorage")
	if !ls.Truthy() {
		return "", false
	}
	v := ls.Call("getItem", key)
	if !v.Truthy() {
		return "", false
	}
	return v.String(), true
}

// lsSet writes a key, and does nothing at all if it cannot.
func lsSet(key, val string) {
	defer func() {
		if recover() != nil {
			lsNote("write", key)
		}
	}()
	if st := crStore(); st.Truthy() {
		if !st.Call("set", key, val).Bool() {
			lsNote("write", key)
		}
		return
	}
	ls := js.Global().Get("localStorage")
	if !ls.Truthy() {
		return
	}
	ls.Call("setItem", key, val)
}

// lsRemove deletes a key.
func lsRemove(key string) {
	defer func() {
		if recover() != nil {
			lsNote("delete", key)
		}
	}()
	if st := crStore(); st.Truthy() {
		st.Call("del", key)
		return
	}
	ls := js.Global().Get("localStorage")
	if !ls.Truthy() {
		return
	}
	ls.Call("removeItem", key)
}
