//go:build js && wasm

// Package hostauth is how a pane gets the host agent's token.
//
// There are two ways, and which one is in use is the server's decision.
//
// INJECTED is the default: desk-serve puts the token straight into the page it
// serves, and nothing is asked of anyone. That is the right trade on a machine
// with one person on it, where anything that could read the token could read
// the shell it protects.
//
// ASKED FOR is what --auth turns on, and it exists because the injected token
// is only as private as the page. THE ORIGIN CHECK DOES NOT STOP A LOCAL
// PROCESS: it stops a hostile web PAGE, because a browser sets Origin itself
// and script cannot change it, but anything that is not a browser simply sends
// the header it likes. So on a shared machine another user can fetch the
// served page, read the token out of it, forge an Origin and have a shell as
// you. With --auth the token is printed to the terminal that started the
// server and never appears in the page, so reading the page gets an attacker
// nothing.
//
// WHY THERE IS NO --password FLAG. A password on a command line is not a
// secret: /proc/<pid>/cmdline is readable by other users on exactly the
// machines where this threat exists, so the flag would hand the password to
// the attacker it was meant to stop. The secret is generated instead, and only
// ever travels from the server's own terminal to the person reading it.
//
// It is kept in sessionStorage, not localStorage: a token that outlives the tab
// is a token that outlives the reason for trusting it.
package hostauth

import "syscall/js"

// storeKey is where an entered token is remembered for the rest of the tab's
// life, so this is asked once rather than once per window.
const storeKey = "deskHostToken"

// Token returns the agent's token, asking for it if the server did not inject
// one, and an empty string if there is no agent or nobody supplied it.
//
// The prompt is synchronous, which is unfashionable and exactly right here:
// hostfs is a synchronous interface, the answer is needed before the first
// stat, and this happens once per tab.
func Token() string {
	h := js.Global().Get("__deskHost")
	if !h.Truthy() {
		return "" // no agent; every static host, and the default run
	}
	if t := h.Get("token"); t.Truthy() {
		return t.String()
	}
	if !h.Get("auth").Truthy() {
		return "" // no token and no invitation to supply one
	}
	return stored()
}

// Remember stores a token supplied by the person, for the rest of the tab's
// life, so the next window does not have to ask for it again.
func Remember(t string) {
	remember(t)
	notify(t)
}

// listeners are told when the tab's token changes, so that a part of the desk
// which was built before the token arrived can rebuild itself.
//
// It exists because --auth makes the token LATE: it is typed into a terminal
// well after the page loaded, and the filesystem behind the shell and the file
// manager was composed at load, when there was nothing to compose it from.
//
// No lock. This is a wasm tab: everything here runs on the one JavaScript
// thread, and the callbacks are registered while the desk is being built,
// before any of them can fire.
var listeners []func(token string)

// OnToken registers a callback for the token changing, and fires it for every
// later change — an empty token meaning it was withdrawn, which is what a
// refusal does before asking again.
func OnToken(fn func(token string)) { listeners = append(listeners, fn) }

func notify(t string) {
	for _, fn := range listeners {
		fn(t)
	}
}

// Required reports whether the server is serving an agent that wants a token
// supplied, which is what lets a pane say so instead of looking broken.
func Required() bool {
	h := js.Global().Get("__deskHost")
	return h.Truthy() && !h.Get("token").Truthy() && h.Get("auth").Truthy()
}

// Forget drops a remembered token, for when it turns out to be the wrong one.
func Forget() {
	if s := session(); s.Truthy() {
		s.Call("removeItem", storeKey)
	}
	// Told as an empty token, so that anything built on the old one can go back
	// to what it was doing before. A refused token is not a neutral state: the
	// filesystem behind it answers 403 to everything, and a file manager left
	// pointed at it looks broken rather than unauthorized.
	notify("")
}

func session() js.Value { return js.Global().Get("sessionStorage") }

func stored() string {
	s := session()
	if !s.Truthy() {
		return ""
	}
	v := s.Call("getItem", storeKey)
	if !v.Truthy() {
		return ""
	}
	return v.String()
}

func remember(t string) {
	if s := session(); s.Truthy() {
		s.Call("setItem", storeKey, t)
	}
}
