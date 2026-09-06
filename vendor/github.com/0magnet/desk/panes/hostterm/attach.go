//go:build js && wasm

package hostterm

import (
	"encoding/json"
	"errors"
	"syscall/js"

	"github.com/0magnet/desk/panes/hostproto"
	xterm "github.com/0magnet/xterm-go"
)

// A host shell in the terminal you were already in, rather than in a window.
//
// Pane is the window version: it makes a terminal, mounts it, and owns the
// keyboard through Core.OnData. That is right for an app you launch from a
// menu, and it is not how a desktop behaves. On a Linux machine you do not
// get a new window when you type ssh — the program takes over the terminal it
// was started in, and when it exits you are back at your prompt in the same
// scrollback. Attach is that: the applet form.
//
// Two differences from Pane, and both are the point:
//
//   - IT DOES NOT TOUCH Core.OnData. websh owns that, and while an applet runs
//     in raw mode it already forwards raw bytes to the applet's stdin — Ctrl+C
//     included, so the shell can still cancel. Stealing OnData would take the
//     keyboard from underneath the thing that is supposed to be able to stop
//     us. The applet pumps its own stdin into Send instead.
//   - It does not create or own the terminal. The one it is handed belongs to
//     the shell, and is still there when this is finished with it, which is
//     what makes "back at the prompt" mean anything.
//
// Done closes when the remote shell exits, which is what turns `exit` into
// "return to the prompt" rather than "leave a dead window on screen".

// Attachment is one host session running on somebody else's terminal.
type Attachment struct {
	ws            js.Value
	term          *xterm.Terminal
	done          chan struct{}
	restoreResize func()
	funcs         []js.Func
	shut          bool
}

// Attach opens a host session on term. A non-empty session names it, so it
// survives this attachment the way `host NAME` does in a window; empty is an
// ordinary shell that ends when the socket does.
//
// It returns as soon as the socket is constructed rather than waiting for the
// handshake: the agent may refuse, and the refusal arrives as a close, which
// is what Done reports. A caller that waited for "connected" would hang on
// exactly the case it most needs to report.
func Attach(term *xterm.Terminal, session string) (*Attachment, error) {
	if term == nil {
		return nil, errors.New("hostterm: no terminal to attach to")
	}
	cfg, ok := hostAgent()
	if !ok {
		return nil, errors.New("hostterm: this page was not served with --shell")
	}

	a := &Attachment{term: term, done: make(chan struct{})}
	// socketURL reads the terminal for the handshake size, so the stand-in Pane
	// needs the real terminal — with a nil one it dereferences and panics.
	p := &Pane{session: session, term: term}
	ws := js.Global().Get("WebSocket").New(p.socketURL(cfg))
	ws.Set("binaryType", "arraybuffer")
	a.ws = ws

	ws.Call("addEventListener", "message", a.fn(func(args []js.Value) {
		data := args[0].Get("data")
		if data.Type() == js.TypeString {
			a.term.WriteString(data.String())
			return
		}
		u8 := js.Global().Get("Uint8Array").New(data)
		buf := make([]byte, u8.Get("length").Int())
		js.CopyBytesToGo(buf, u8)
		a.term.Write(buf)
	}))
	ws.Call("addEventListener", "close", a.fn(func([]js.Value) { a.finish() }))
	ws.Call("addEventListener", "error", a.fn(func([]js.Value) { a.finish() }))

	// Tell the pty how big it is, now and whenever the window is dragged.
	// Terminal.OnResize rather than Core.OnResize for the same reason Pane
	// gives: Open owns Core's, and taking it breaks the reallocation that
	// makes the grid the right shape.
	a.Resize(term.Core.Cols(), term.Core.Rows())
	prev := term.OnResize
	term.OnResize = func(cols, rows int) {
		if prev != nil {
			prev(cols, rows)
		}
		a.Resize(cols, rows)
	}
	// Restoring the previous handler is Close's job, or the shell keeps
	// telling a socket that is gone how big it is.
	a.restoreResize = func() { term.OnResize = prev }
	return a, nil
}

// send writes one protocol message, dropping it if the socket is not open yet.
// The first keystroke lands in the gap between construction and the handshake
// often enough that this is the normal case, not an edge one.
func (a *Attachment) send(m hostproto.Msg) {
	if !a.ws.Truthy() || a.ws.Get("readyState").Int() != 1 {
		return
	}
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	a.ws.Call("send", string(b))
}

// Send forwards raw input — what the applet read from its stdin — to the pty.
func (a *Attachment) Send(b []byte) {
	if len(b) == 0 {
		return
	}
	a.send(hostproto.Msg{T: hostproto.TypeInput, D: string(b)})
}

// Resize tells the pty the grid changed.
func (a *Attachment) Resize(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	a.send(hostproto.Msg{T: hostproto.TypeResize, C: cols, R: rows})
}

// Done closes when the remote shell exits or the connection goes away. It is
// how an applet knows to stop and hand the terminal back.
func (a *Attachment) Done() <-chan struct{} { return a.done }

// Close ends the attachment. Safe to call after the socket already closed,
// which is the ordinary path: the shell exits, Done fires, the applet calls
// this on its way out.
func (a *Attachment) Close() {
	a.shut = true
	if a.restoreResize != nil {
		a.restoreResize()
		a.restoreResize = nil
	}
	if a.ws.Truthy() {
		a.ws.Call("close")
		a.ws = js.Value{}
	}
	a.finish()
	for _, f := range a.funcs {
		f.Release()
	}
	a.funcs = nil
}

// finish closes done exactly once. Both socket handlers and Close can reach
// it, and closing a closed channel panics.
func (a *Attachment) finish() {
	select {
	case <-a.done:
	default:
		close(a.done)
	}
}

// fn registers a callback and remembers it, so Close can release it. A js.Func
// that is never released leaks its Go closure, and an applet run from a prompt
// is a thing somebody does repeatedly.
func (a *Attachment) fn(h func([]js.Value)) js.Func {
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		h(args)
		return nil
	})
	a.funcs = append(a.funcs, f)
	return f
}
