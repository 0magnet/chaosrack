//go:build js && wasm

package attractor

import "syscall/js"

// The single shared AudioContext. Before this file, five features each built
// (and tore down) a private context: Model Out, the FVF listen engine, the
// test tone, the function generators, and microphone capture. Browsers cap
// concurrent AudioContexts (each costs an audio thread), and the per-feature
// close/recreate churn made the autoplay policy a per-feature battle. Now:
// one lazily-created context that is never closed. A feature acquires a
// named lease and builds its own subgraph; the context suspends only while
// nobody holds a lease.

var (
	sharedAudioCtx js.Value
	audioCtxUsers  = map[string]bool{}
)

// audioCtxRef returns the shared context, creating it on first use.
// Undefined when Web Audio is unavailable.
func audioCtxRef() js.Value {
	if !sharedAudioCtx.Truthy() {
		ctor := js.Global().Get("AudioContext")
		if !ctor.Truthy() {
			ctor = js.Global().Get("webkitAudioContext")
		}
		if !ctor.Truthy() {
			return js.Undefined()
		}
		sharedAudioCtx = ctor.New()
	}
	return sharedAudioCtx
}

// acquireAudioCtx registers user as an active subgraph and returns the
// (resumed) shared context. First call should happen inside a user-gesture
// handler so the autoplay policy lets the context start.
func acquireAudioCtx(user string) js.Value {
	ctx := audioCtxRef()
	if ctx.Truthy() {
		audioCtxUsers[user] = true
		ctx.Call("resume")
	}
	return ctx
}

// releaseAudioCtx drops user's lease; when the last lease goes, the context
// suspends (never closes — subgraphs stay valid for the next resume).
func releaseAudioCtx(user string) {
	delete(audioCtxUsers, user)
	if len(audioCtxUsers) == 0 && sharedAudioCtx.Truthy() {
		sharedAudioCtx.Call("suspend")
	}
}
