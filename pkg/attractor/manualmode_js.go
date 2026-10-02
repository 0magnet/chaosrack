//go:build js && wasm

package attractor

// The manual as a page of its own: the rack opened as /manual (the server
// sends it to /?manual) writes it. One source and one form: the text is the
// manual package's, and the layout — which bay a module is in, a control's
// address — is the rack's, read off the panel it has just laid out, exactly
// as the Info window reads one bay (bayManualHTML). Nothing that depends on
// the layout is ever written into a file, so nothing can go stale.
//
// The front of it is the manual's README (how to read it, the addresses, the
// signal flow); then every bay, in order; then every model, with what it is
// and what each of its constants does, which the bays cannot show — a bank
// shows one model's constants at a time.
//
// A server started with --manual-dir hands over the files on disk
// (/manual/src.json), so an edit to them shows on reload.

import (
	"encoding/json"
	"html"
	"strconv"
	"strings"
	"syscall/js"
	"time"

	"github.com/0magnet/chaosrack/manual"
	"github.com/0magnet/chaosrack/pkg/dom"
)

// enableManualMode writes the manual over the page if it was opened with
// ?manual. Called once the panel is built.
func enableManualMode() {
	loc := js.Global().Get("location")
	if !loc.Truthy() || !js.Global().Get("URLSearchParams").New(loc.Get("search")).Call("has", "manual").Bool() {
		return
	}
	page := dom.Doc.Call("createElement", "div")
	page.Set("id", "rack-manual")
	page.Set("innerHTML", `<main><p class="wait">Writing the manual: laying out the rack…</p></main>`)
	dom.Doc.Get("body").Call("appendChild", page)
	go func() {
		if src := fetchManualSources(); len(src) > 0 {
			manual.Use(src)
			refreshDocTitles()
		}
		waitForAddresses()
		setPowerState(false) // nothing to draw: the manual is the page
		dom.Doc.Set("title", "chaosrack manual")
		// Measured, and the page written, while every module is still in the
		// rack: both read the bays. Then the modules move into the page
		// (manuallive_js.go), and out into windows from there (manualwin_js.go).
		live := manualLive()
		live.Call("measure")
		page.Set("innerHTML", manualPageHTML())
		live.Call("place", page)
		live.Call("maps", page)
		live.Call("link", page)
		manualShown = true
		manualRefresh() // signs each module as written
		wireManualWindows(page)
		if h := loc.Get("hash").String(); len(h) > 1 {
			if el := dom.Doc.Call("getElementById", h[1:]); el.Truthy() {
				el.Call("scrollIntoView")
			}
		}
	}()
}

// fetchManualSources is the markdown the server has for the manual
// (/manual/src.json), or nil where there is none — a static copy of the page
// has no server behind it, and the copy built in is the manual then.
func fetchManualSources() map[string]string {
	done := make(chan string, 1)
	var ok, fail, text js.Func
	fail = js.FuncOf(func(js.Value, []js.Value) any { done <- ""; return nil })
	text = js.FuncOf(func(_ js.Value, a []js.Value) any { done <- a[0].String(); return nil })
	ok = js.FuncOf(func(_ js.Value, a []js.Value) any {
		r := a[0]
		if !r.Get("ok").Bool() {
			done <- ""
			return nil
		}
		r.Call("text").Call("then", text, fail)
		return nil
	})
	js.Global().Call("fetch", "/manual/src.json", map[string]any{"cache": "no-store"}).Call("then", ok, fail)
	body := <-done
	ok.Release()
	fail.Release()
	text.Release()
	var src map[string]string
	if body == "" || json.Unmarshal([]byte(body), &src) != nil {
		return nil
	}
	return src
}

// refreshDocTitles rewrites every tooltip the markup took from the manual
// (data-doc), after the manual has been replaced by the files on disk, and
// readdresses them.
func refreshDocTitles() {
	els := dom.Doc.Call("querySelectorAll", "[data-doc]")
	for i := range els.Length() {
		el := els.Index(i)
		if t := doc(el.Call("getAttribute", "data-doc").String()); t != "" {
			el.Set("title", t)
		}
	}
	scheduleDesignate()
}

// waitForAddresses waits until the rack has been packed and addressed and
// has stopped moving: the same count of addressed controls twice running.
func waitForAddresses() {
	last := -1
	for range 200 {
		time.Sleep(150 * time.Millisecond)
		n := dom.Doc.Call("querySelectorAll", "[data-loc]").Length()
		if n > 50 && n == last {
			return
		}
		last = n
	}
}

// manualPageHTML is the whole manual: the front, every bay, every model.
func manualPageHTML() string {
	bays := fastDOM().Call("bays").Length()
	var nav, body strings.Builder
	nav.WriteString(`<nav><div class="brand">chaosrack manual</div><button class="mmodel" title="` + html.EscapeString(doc("manual-model")) + `">⧉ the model</button><a href="#front">Reading it</a>`)
	body.WriteString(`<main><section id="front">` + manual.Page("README") + `</section>`)

	nav.WriteString(`<div class="grp">The rack</div>`)
	body.WriteString(`<h1 id="rack">The rack, bay by bay</h1>`)
	for n := 1; n <= bays; n++ {
		id := "bay-" + strconv.Itoa(n)
		t := bayTitle(n)
		nav.WriteString(`<a href="#` + id + `">` + html.EscapeString(t) + `</a>`)
		body.WriteString(`<section id="` + id + `"><h2>` + html.EscapeString(t) + ` <button class="mpop" data-pop-bay="` + strconv.Itoa(n) + `" title="` + html.EscapeString(doc("manual-pop-bay")) + `">⧉ window</button></h2><div class="mbaymap" data-bay="` + strconv.Itoa(n) + `"></div>` + bayManualHTML(n, true) + `</section>`)
	}

	nav.WriteString(`<div class="grp">Models</div>`)
	body.WriteString(`<h1 id="models">Models</h1>`)
	seen := map[string]bool{}
	for _, g := range Catalog() {
		gid := "models-" + strings.ToLower(categorySlug(g.Label))
		nav.WriteString(`<a href="#` + gid + `">` + html.EscapeString(g.Label) + `</a>`)
		body.WriteString(`<h2 id="` + gid + `">` + html.EscapeString(g.Label) + `</h2>`)
		if c := manual.HTML("cat." + g.Label); c != "" {
			body.WriteString(c)
		}
		for _, m := range g.Models {
			if seen[m.Key] {
				continue
			}
			seen[m.Key] = true
			body.WriteString(manualModelHTML(m))
		}
	}
	nav.WriteString(`</nav>`)
	body.WriteString(`</main>`)
	return nav.String() + body.String()
}

// manualModelHTML is one model's part of the manual: what it is, and what
// each of its constants does, with a selector's positions under it.
func manualModelHTML(m CatalogModel) string {
	var b strings.Builder
	b.WriteString(`<section id="model-` + html.EscapeString(m.Key) + `" class="model"><h3>` + html.EscapeString(m.Label) + `</h3>`)
	if d := manual.HTML("model." + m.Key); d != "" {
		b.WriteString(d)
	} else if m.Description != "" {
		b.WriteString(`<p>` + strings.ReplaceAll(html.EscapeString(m.Description), "\n", "<br>") + `</p>`)
	}
	ps := attractorParams[m.Key]
	if len(ps) == 0 {
		b.WriteString(`</section>`)
		return b.String()
	}
	b.WriteString(`<dl>`)
	for _, p := range ps {
		b.WriteString(`<dt>` + html.EscapeString(p.Label) + `</dt><dd>`)
		switch {
		case manual.Has("p." + p.ID):
			b.WriteString(manual.HTML("p." + p.ID))
		case helpFor(p.ID) != "":
			b.WriteString(`<p>` + html.EscapeString(helpFor(p.ID)) + `</p>`)
		default:
			b.WriteString(`<p>` + html.EscapeString(p.Label) + `, ` + fmtDialNum(float64(p.Min)) + ` to ` + fmtDialNum(float64(p.Max)) + `</p>`)
		}
		if pos := manual.Positions("p." + p.ID); len(pos) > 0 {
			b.WriteString(infoListHTML(pos))
		} else if names := paramLabels[p.ID]; len(names) > 0 {
			b.WriteString(`<ul>`)
			for _, n := range names {
				b.WriteString(`<li>` + html.EscapeString(n) + `</li>`)
			}
			b.WriteString(`</ul>`)
		}
		b.WriteString(`</dd>`)
	}
	b.WriteString(`</dl></section>`)
	return b.String()
}

// manualShown is set once the manual page is written: from then on the rack
// readdressing itself (designate) writes again what the page says of any
// module that changed — the bank and its readouts follow the model.
var manualShown bool

// manualRefresh writes again each module's text on the manual page whose
// module no longer reads as it did when it was written.
func manualRefresh() {
	live, h := manualLive(), fastDOM()
	texts := dom.Doc.Call("querySelectorAll", `#rack-manual .mtext[data-mloc]`)
	for i := range texts.Length() {
		t := texts.Index(i)
		loc := t.Call("getAttribute", "data-mloc").String()
		m := live.Call("module", loc)
		if !m.Truthy() {
			continue
		}
		s := h.Call("moduleManual", m).String()
		if s == t.Call("getAttribute", "data-sig").String() {
			continue
		}
		first := !t.Call("hasAttribute", "data-sig").Bool()
		t.Call("setAttribute", "data-sig", s)
		if first {
			continue // as written with the page
		}
		var mi infoModule
		if json.Unmarshal([]byte(s), &mi) != nil {
			continue
		}
		t.Set("innerHTML", moduleManualHTML(mi, true))
		live.Call("linkText", t)
		if hold := dom.Doc.Call("querySelector", `#rack-manual .mlive[data-mloc="`+loc+`"]`); hold.Truthy() {
			live.Call("callouts", hold)
		}
	}
}
