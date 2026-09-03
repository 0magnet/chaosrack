//go:build js && wasm

// Package hostterm is a shell on the machine, as a desk pane.
//
// It is the counterpart to panes/term, and the difference between them is the
// whole point: term runs websh, a Bash interpreter compiled to wasm against a
// filesystem that lives in IndexedDB, and can touch nothing outside the tab.
// This one is a pty on the host. Same terminal, same window, entirely different
// blast radius — which is why the agent that serves it is off unless asked for,
// and why a pane that finds no agent says so instead of looking broken.
//
// Nothing here decides whether the machine should be reachable. That decision
// is desk-serve's, expressed by whether it injected a token into the page. This
// pane only reports the answer.
package hostterm

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"syscall/js"

	xterm "github.com/0magnet/xterm-go"
	"github.com/0magnet/xterm-go/vt"

	"github.com/0magnet/desk/panes/hostauth"

	"github.com/0magnet/desk/panes/hostproto"
)

// Pane is a terminal attached to a shell on the host.
type Pane struct {
	term   *xterm.Terminal
	ws     js.Value
	el     js.Value
	funcs  []js.Func
	shut   bool // Close was called; stop reporting the socket going away
	opened bool // the socket got as far as opening, so a close is a disconnect
}

// New makes a host terminal pane.
func New() *Pane { return &Pane{} }

// Mount opens the terminal and connects it to the agent.
//
// The terminal is opened even when there is no agent to connect to. A pane that
// mounted nothing would be an empty rectangle with no way to find out why,
// whereas a terminal with a paragraph in it is a terminal with a paragraph in
// it — and the paragraph names the flag.
func (p *Pane) Mount(el js.Value) error {
	p.el = el

	opts := vt.NewOptions()
	p.term = xterm.New(opts)
	p.term.Open(el)

	// Best effort, and not an error. Without it this pane still works as a
	// terminal; it just cannot be composited, because the DOM cannot be
	// sampled into a texture and a canvas can. Canvas reports that by
	// returning nothing, which the compositor already treats as "not now".
	_ = p.term.EnableWebGL() //nolint:errcheck // reported by Canvas returning nothing

	// AutoFit watches the element, so dragging the window edge re-measures
	// the grid without anyone passing the message along. That is also what
	// makes OnResize fire, which is what tells the pty.
	p.term.AutoFit()

	cfg, ok := hostAgent()
	if !ok {
		if hostauth.Required() {
			p.askForToken()
		} else {
			p.writeNoAgent()
		}
		return nil
	}
	p.connect(cfg)
	return nil
}

// agentConfig is what desk-serve injected into the page.
type agentConfig struct {
	path  string
	token string
}

// hostAgent reads the global desk-serve injects.
//
// Absent is the normal case, not a failure: it is what any static host serves,
// including the project's own GitHub Pages build, and what desk-serve itself
// serves without --shell.
func hostAgent() (agentConfig, bool) {
	h := js.Global().Get("__deskHost")
	if !h.Truthy() {
		return agentConfig{}, false
	}
	path := h.Get("path")
	if !path.Truthy() {
		return agentConfig{}, false // an agent, but no pty half of it
	}
	// May ask the person for it — see hostauth.
	token := hostauth.Token()
	if token == "" {
		return agentConfig{}, false
	}
	return agentConfig{path: path.String(), token: token}, true
}

// socketURL builds the agent's address from the page's own.
//
// Derived from location rather than configured, because the Origin check on the
// other end only passes for the page the agent itself served — so any other
// address would be refused, and building one would only produce a failure that
// looks like a network problem.
func (p *Pane) socketURL(cfg agentConfig) string {
	loc := js.Global().Get("location")
	scheme := "ws://"
	if loc.Get("protocol").String() == "https:" {
		scheme = "wss://"
	}
	q := url.Values{}
	q.Set(hostproto.TokenParam, cfg.token)
	// The size goes on the handshake so the pty starts at it. A shell reads
	// its window size when it draws the first prompt; correcting afterwards
	// leaves that prompt wrapped at a width nothing redraws.
	q.Set(hostproto.ColsParam, strconv.Itoa(p.term.Core.Cols()))
	q.Set(hostproto.RowsParam, strconv.Itoa(p.term.Core.Rows()))
	return scheme + loc.Get("host").String() + cfg.path + "?" + q.Encode()
}

func (p *Pane) connect(cfg agentConfig) {
	ws := js.Global().Get("WebSocket").New(p.socketURL(cfg))
	ws.Set("binaryType", "arraybuffer")
	p.ws = ws

	ws.Call("addEventListener", "message", p.fn(func(args []js.Value) {
		data := args[0].Get("data")
		if data.Type() == js.TypeString {
			p.term.WriteString(data.String())
			return
		}
		u8 := js.Global().Get("Uint8Array").New(data)
		buf := make([]byte, u8.Get("length").Int())
		js.CopyBytesToGo(buf, u8)
		p.term.Write(buf)
	}))

	ws.Call("addEventListener", "open", p.fn(func([]js.Value) { p.opened = true }))
	ws.Call("addEventListener", "close", p.fn(func([]js.Value) {
		if p.shut {
			return // the window closed; the socket following it is not news
		}
		if !p.opened {
			// It never opened, so this is a refusal rather than a
			// disconnection — almost always the wrong token, since that is
			// the only thing the person supplies.
			// No prompt printed here: askForToken prints its own, and doing
			// both gives two carets and no explanation between them.
			p.term.WriteString("\r\n\x1b[31mrefused.\x1b[0m\r\n\r\n")
			hostauth.Forget()
			p.askForToken()
			return
		}
		// Distinguished from a mount with no agent at all, because the
		// remedies are different: this one is usually the server having
		// stopped, or a token from a previous run in a stale tab.
		p.term.WriteString("\r\n\x1b[33m[disconnected from the host agent]\x1b[0m\r\n")
	}))

	send := func(m hostproto.Msg) {
		// readyState 1 is OPEN. Sending before then throws, and the first
		// keystroke can easily land in the gap between construction and the
		// handshake completing.
		if p.ws.Get("readyState").Int() != 1 {
			return
		}
		b, err := json.Marshal(m)
		if err != nil {
			return
		}
		p.ws.Call("send", string(b))
	}

	p.term.Core.OnData = func(data string) {
		send(hostproto.Msg{T: hostproto.TypeInput, D: data})
	}
	p.term.Core.OnBinary = func(data string) {
		// OnBinary hands over latin-1 bytes packed one per rune, which is
		// how mouse reports arrive. Widening them back to bytes before
		// base64 is what keeps a click past column 95 from being reported
		// as a different column.
		raw := make([]byte, len(data))
		for i := 0; i < len(data); i++ {
			raw[i] = data[i]
		}
		send(hostproto.Msg{T: hostproto.TypeBinary, D: base64.StdEncoding.EncodeToString(raw)})
	}
	// Terminal.OnResize, NOT Core.OnResize, and the distinction is not
	// cosmetic. Open owns Core.OnResize, and its handler is what reallocates
	// the WebGL renderer for the new grid; taking that field leaves the render
	// model sized for the old one and the next frame indexes past the end of
	// it. In wasm that panic ends the Go program, so the symptom is every
	// window in the desk freezing at once and the pty on the far end of this
	// socket orphaned because the page never closes it.
	//
	// This pane is why xterm-go grew the safe hook: it was written the wrong
	// way first, and the wrong way looks right because Attach assigns
	// Core.OnData. Terminal.OnResize is fanned out from that handler rather
	// than replacing it, and fires after the renderer is ready.
	p.term.OnResize = func(cols, rows int) {
		send(hostproto.Msg{T: hostproto.TypeResize, C: cols, R: rows})
	}
}

// writeNeedsToken explains the other absence: there IS an agent, and nobody
// has supplied its token. Distinguished from having no agent at all because
// the remedies are different, and "no host agent" would be a lie.
// askForToken reads the token in the terminal itself.
//
// Here rather than in a window.prompt(), which is what this did first and was
// wrong twice over: a native dialog blocks the whole page before anything is on
// screen, and it demanded a token from everyone whether or not they were going
// to open a shell. Asking in the pane that needs it means the question appears
// exactly when it is relevant, in a terminal that is already good at reading a
// line of text.
func (p *Pane) askForToken() {
	p.term.WriteString("\x1b[1;33mToken required.\x1b[0m \x1b[2m--auth: the server printed it.\x1b[0m\r\n\r\n> ")

	p.opened = false
	var typed []rune
	p.term.Core.OnData = func(data string) {
		for _, r := range data {
			switch r {
			case '\r', '\n':
				tok := strings.TrimSpace(string(typed))
				typed = typed[:0]
				p.term.WriteString("\r\n")
				if tok == "" {
					p.term.WriteString("> ")
					continue
				}
				hostauth.Remember(tok)
				cfg, ok := hostAgent()
				if !ok {
					p.term.WriteString("\x1b[31mstill no agent to connect to.\x1b[0m\r\n")
					return
				}
				p.term.WriteString("\x1b[2mconnecting…\x1b[0m\r\n")
				p.connect(cfg)
			case 127, 8: // backspace
				if len(typed) > 0 {
					typed = typed[:len(typed)-1]
					p.term.WriteString("\b \b")
				}
			default:
				if r >= 32 {
					typed = append(typed, r)
					// Echoed as dots. It is a credential, and a terminal in a
					// window manager is a terminal somebody may be screen
					// sharing.
					p.term.WriteString("•")
				}
			}
		}
	}
}

// writeNoAgent explains the absence, with the command that fixes it.
func (p *Pane) writeNoAgent() {
	p.term.WriteString("\x1b[1;33mNo host agent.\x1b[0m\r\n\r\n")
	p.term.WriteString("This page was served without one, so there is nothing on the\r\n")
	p.term.WriteString("other side of this terminal. To run a real shell here:\r\n\r\n")
	p.term.WriteString("    \x1b[1;36mdesk-serve --shell\x1b[0m\r\n\r\n")
	p.term.WriteString("\x1b[2mIt binds loopback only and prints nothing to disk; stopping the\r\n")
	p.term.WriteString("server revokes access. For a shell that cannot reach the machine\r\n")
	p.term.WriteString("at all, the ordinary Terminal app runs websh in this tab.\x1b[0m\r\n")
}

// Canvas implements desk.TexturePane.
//
// Same reasoning as the websh terminal's: with the WebGL renderer every glyph
// on screen lives in one canvas, and texImage2D takes a canvas directly. Not
// cached, because EnableWebGL and DisableWebGL create and remove this element
// at runtime and a cached hit would name a detached node.
func (p *Pane) Canvas() js.Value {
	if !p.el.Truthy() {
		return js.Value{}
	}
	return p.el.Call("querySelector", "canvas.xterm-webgl-canvas")
}

// Close ends the session and releases the callbacks.
//
// shut is set first so the close handler stays quiet: closing the window is not
// a disconnection worth reporting, and reporting it would write into a terminal
// that is being disposed.
func (p *Pane) Close() {
	p.shut = true
	if p.ws.Truthy() {
		p.ws.Call("close")
		p.ws = js.Value{}
	}
	if p.term != nil {
		p.term.Dispose()
		p.term = nil
	}
	for _, f := range p.funcs {
		f.Release()
	}
	p.funcs = nil
}

// fn registers a callback and remembers it, because a js.Func that is never
// released leaks its Go closure for the life of the page — and a desk is a
// thing windows are opened and closed in all day.
func (p *Pane) fn(h func([]js.Value)) js.Func {
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		h(args)
		return nil
	})
	p.funcs = append(p.funcs, f)
	return f
}
