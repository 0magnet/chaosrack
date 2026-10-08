//go:build js && wasm

package web

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"syscall/js"

	"github.com/0magnet/websh/progressive"
)

// Shipped widgets (PROTOCOL.md): a program not in the tab — on another
// machine, over ssh — sends a widget as a document, and the host runs it in a
// sandboxed iframe. The sandbox allows scripts and nothing else: no
// same-origin access, so the widget can draw and compute but cannot touch
// the page, its storage or its cookies. It talks only to the program, over
// the same line as any widget.

// shipLimit is the most a command may ship, all widgets together.
const shipLimit = 8 << 20

// shipBoot runs first in every shipped widget: it takes the line the host
// transfers in, and gives the document websh.send and websh.onmessage.
// Messages are JSON text on the wire, values in the document.
const shipBoot = `<script>(()=>{let port=null,q=[],h=null;` +
	`addEventListener("message",e=>{if(e.source!==parent||e.data!=="websh-line"||port||!e.ports[0])return;` +
	`port=e.ports[0];port.onmessage=m=>{if(h)try{h(JSON.parse(m.data))}catch(_){}};q.forEach(s=>port.postMessage(s));q=[]});` +
	`window.websh={send(v){const s=JSON.stringify(v);if(port)port.postMessage(s);else q.push(s)},onmessage(f){h=f}};})();</script>`

// shipping is what a command has shipped: finished widgets by name, and the
// ones still arriving.
type shipping struct {
	done map[string]string
	part map[string]*strings.Builder
	size int
}

// ship takes one chunk of widget name; the last one makes it placeable.
func (p *placements) ship(name, meta, chunk string) {
	var m struct {
		Kind string `json:"kind"`
		More bool   `json:"more"`
	}
	mb, err := base64.StdEncoding.DecodeString(meta)
	if err != nil || json.Unmarshal(mb, &m) != nil || m.Kind != "html" || name == "" {
		return
	}
	b, err := base64.StdEncoding.DecodeString(chunk)
	if err != nil {
		return
	}
	sh := &p.shipped
	if sh.done == nil {
		sh.done, sh.part = map[string]string{}, map[string]*strings.Builder{}
	}
	if sh.size+len(b) > shipLimit {
		delete(sh.part, name)
		return
	}
	sh.size += len(b)
	part := sh.part[name]
	if part == nil {
		part = &strings.Builder{}
		sh.part[name] = part
	}
	part.Write(b)
	if !m.More {
		sh.done[name] = part.String()
		delete(sh.part, name)
	}
}

// forgetShipped drops what the last command shipped, as it ends.
func (p *placements) forgetShipped() { p.shipped = shipping{} }

// mountShipped fills pl with a shipped widget: a sandboxed iframe holding its
// document, and the other end of the line transferred in once it has loaded.
// Clicks inside it stay inside it, so a shipped widget tells the program what
// was done to it itself, by sending.
func (p *placements) mountShipped(pl *placement, html string) {
	ch := js.Global().Get("MessageChannel").New()
	pl.port = ch.Get("port1")
	if pl.d.Events {
		pl.onMsg = js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) == 0 {
				return nil
			}
			d := args[0].Get("data")
			if d.Type() != js.TypeString || !json.Valid([]byte(d.String())) {
				return nil
			}
			p.s.event(pl.id, &progressive.Event{Type: "message", Data: json.RawMessage(d.String())})
			return nil
		})
		pl.port.Set("onmessage", pl.onMsg)
	}
	f := js.Global().Get("document").Call("createElement", "iframe")
	f.Call("setAttribute", "sandbox", "allow-scripts")
	f.Get("style").Set("cssText", "position:absolute;inset:0;width:100%;height:100%;border:0;background:transparent")
	f.Set("srcdoc", shipBoot+html)
	var loaded js.Func
	loaded = js.FuncOf(func(js.Value, []js.Value) any {
		if w := f.Get("contentWindow"); w.Truthy() {
			w.Call("postMessage", "websh-line", "*", js.ValueOf([]any{ch.Get("port2")}))
		}
		loaded.Release()
		return nil
	})
	f.Call("addEventListener", "load", loaded, map[string]any{"once": true})
	pl.el.Call("append", f)
}
