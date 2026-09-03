//go:build js && wasm

package term

import (
	"syscall/js"

	"github.com/0magnet/afero"
	"github.com/0magnet/desk/dom"
	"github.com/0magnet/desk/panes/hostauth"
	"github.com/0magnet/desk/panes/hostfs"
)

// Giving the desk the machine's filesystem, whenever it becomes available.
//
// Without --auth this is settled at load: the token is in the page, the host
// filesystem is built from it, and SetFS installs it before a pane exists. With
// --auth there is no token at load — that is the point of the flag — so the
// desk starts on memory and the token is typed into a terminal later. This is
// the part that notices, and it is why FS hands out a switch rather than a
// filesystem.

func notifyFSChanged() {
	ev := js.Global().Get("CustomEvent")
	if !ev.Truthy() {
		return
	}
	js.Global().Call("dispatchEvent", ev.New(dom.FSChangedEvent))
}

// preHostFS is what the desk was using before a token first replaced it, kept
// so that a token being withdrawn can put it back.
var preHostFS afero.Fs

// UseHostFS points the desk at the machine's filesystem if the page was served
// with one, and arranges to do it later if the token has not been supplied yet.
//
// One call replaces the composition every desk was doing by hand.
func UseHostFS() {
	if fsys, ok := hostfs.Compose(); ok {
		SetFS(fsys)
		return
	}
	if !hostauth.Required() {
		return // no agent, or no filesystem behind it: memory is the answer
	}
	hostauth.OnToken(func(tok string) {
		if tok == "" {
			// Withdrawn, which is what a refused token does on its way to being
			// asked for again. Anything mounted on it now answers 403 to every
			// call, so going back to memory is the honest state.
			if preHostFS != nil {
				SwapFS(preHostFS)
				preHostFS = nil
			}
			return
		}
		fsys, ok := hostfs.Compose()
		if !ok {
			return
		}
		if preHostFS == nil {
			preHostFS = sharedFS.get()
		}
		SwapFS(fsys)
	})
}
