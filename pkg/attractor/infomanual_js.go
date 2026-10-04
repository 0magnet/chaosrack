//go:build js && wasm

package attractor

// The Info window's manual: what is on the bay you are working at. Each
// module on it, what its header says it is for, and each of its controls by
// address with its entry in the manual (the manual directory, docs.go) — the
// service manual's page for the part of the rack in front of you. Shown only
// while the rack is off: with a model on screen, Info is the model's
// description, as it always was.
//
// Which bay is the one last pressed in. One scrolled mostly out of sight
// gives way to whichever bay is most in view, and before anything has been
// pressed that is the one too.
//
// Which controls are there, and their addresses, are read off the page
// (fastDOM's bayManual): an address is where a control is, and only the page
// knows that. What each one is comes from the manual.

import (
	"encoding/json"
	"github.com/0magnet/chaosrack/manual"
	"github.com/0magnet/chaosrack/pkg/dom"
	"html"
	"strconv"
	"strings"
	"syscall/js"
)

// infoBay is the bay the manual is about, counting from 1; 0 until one has
// been pressed in or seen.
var infoBay int

// infoManualKey is what the manual on the page was written for, so a refresh
// that changes nothing leaves it, and where it was scrolled to, alone.
var infoManualKey string

// infoBayWatch is set once the listeners that follow the bay are in.
var infoBayWatch bool

// watchInfoBay follows which bay the manual is about: a press in one makes it
// that one, and a scroll that leaves it mostly out of sight hands it to the
// bay most in view. Presses are followed whether the window is open or not,
// so it opens on the bay last worked at — not on the Console, where the Info
// switch that opens it is; that switch, and the window, are not a bay's.
func watchInfoBay() {
	if infoBayWatch {
		return
	}
	infoBayWatch = true
	opts := map[string]any{"capture": true, "passive": true}
	dom.Doc.Call("addEventListener", "pointerdown", js.FuncOf(func(_ js.Value, a []js.Value) any {
		t := a[0].Get("target")
		if t.Call("closest", "#info-window, label:has(#show-info)").Truthy() {
			return nil
		}
		if b := fastDOM().Call("bayOf", t).Int(); b > 0 && b != infoBay {
			infoBay = b
			if infoShown() {
				updateInfoOverlay()
			}
		}
		return nil
	}), opts)
	var timer js.Value
	settle := js.FuncOf(func(js.Value, []js.Value) any {
		timer = js.Undefined()
		if !infoShown() {
			return nil
		}
		if infoBay == 0 || fastDOM().Call("bayShown", infoBay).Float() < 0.5 {
			if b := fastDOM().Call("mostVisibleBay").Int(); b > 0 && b != infoBay {
				infoBay = b
				updateInfoOverlay()
			}
		}
		return nil
	})
	dom.Doc.Call("addEventListener", "scroll", js.FuncOf(func(js.Value, []js.Value) any {
		if timer.Truthy() {
			js.Global().Call("clearTimeout", timer)
		}
		timer = js.Global().Call("setTimeout", settle, 250)
		return nil
	}), opts)
}

// infoShown reports whether the Info window is open.
func infoShown() bool {
	sw := dom.Doc.Call("getElementById", "show-info")
	return sw.Truthy() && sw.Get("checked").Bool()
}

// infoModule is one module of a bay as bayManual reads it.
type infoModule struct {
	Name  string     `json:"h"`
	About string     `json:"t"`
	Range string     `json:"r"` // the addresses it covers
	Cells []infoCell `json:"c"`
	Keys  []string   `json:"k"` // what the manual may have it under
}

// infoCell is one addressed control: its address, what its tooltip says,
// and what the manual may have it under.
type infoCell struct {
	Loc  string   `json:"l"`
	Tip  string   `json:"t"`
	Keys []string `json:"k"`
}

// writeInfo fills the Info window: the running model's description while
// one is on screen, and the manual for the bay being worked at while the rack
// is off.
func writeInfo(body js.Value, modelText string) {
	if infoBay == 0 {
		infoBay = fastDOM().Call("mostVisibleBay").Int()
	}
	// A model on screen is what Info is about, and the manual would bury its
	// description; the manual is for the rack switched off, when there is
	// nothing to describe but the rack.
	off := infoRackOff()
	desc := infoPart(body, "info-model")
	desc.Set("innerHTML", infoModelHTML(modelText))
	desc.Get("style").Set("display", map[bool]string{true: "none", false: ""}[off])
	man := infoPart(body, "info-manual")
	man.Get("style").Set("display", map[bool]string{true: "", false: "none"}[off])
	if !off {
		infoManualKey = ""
		return
	}

	key := strconv.Itoa(infoBay) + "|" + run.selectedMode
	if key == infoManualKey {
		return
	}
	infoManualKey = key
	infoPart(body, "info-manual").Set("innerHTML",
		`<h3>`+html.EscapeString(bayTitle(infoBay))+`</h3>`+bayManualHTML(infoBay, false))
}

// bayManualHTML is bay n's page of the manual, as the Info window and the
// manual page (manualmode_js.go) both show it: each module on it, as it is
// placed, its address and what it is, then each of its controls by address
// with its entry. With figures, for the manual page, each module and control
// has an id to be linked to (manualAnchor), and each module a place under its
// heading for the module itself (manuallive_js.go) and a button to take it
// out into a window.
func bayManualHTML(n int, figures bool) string {
	var mods []infoModule
	if err := json.Unmarshal([]byte(fastDOM().Call("bayManual", n).String()), &mods); err != nil {
		return ""
	}
	var b strings.Builder
	for _, m := range mods {
		if figures && m.Range != "" {
			b.WriteString(`<h4 id="` + manualAnchor(m.Range) + `">` + html.EscapeString(m.Name))
		} else {
			b.WriteString(`<h4>` + html.EscapeString(m.Name))
		}
		if m.Range != "" {
			b.WriteString(` <span>` + html.EscapeString(m.Range) + `</span>`)
		}
		b.WriteString(`</h4>`)
		if !figures || m.Range == "" {
			b.WriteString(moduleManualHTML(m, figures))
			continue
		}
		// What it says, in a part of its own that is written again when the
		// module changes (manualRefresh). The module itself is in its bay's
		// strip above (manuallive_js.go).
		r := html.EscapeString(m.Range)
		b.WriteString(`<div class="mtext" data-mloc="` + r + `">` + moduleManualHTML(m, figures) + `</div>`)
	}
	return b.String()
}

// moduleManualHTML is what the manual says of module m: what it is, then
// each of its controls by address with its entry.
func moduleManualHTML(m infoModule, figures bool) string {
	var b strings.Builder
	if m.About != "" {
		b.WriteString(infoEntryHTML(m.About, m.Keys))
	}
	if len(m.Cells) == 0 {
		return b.String()
	}
	b.WriteString(`<dl>`)
	said := map[string]string{} // a tooltip already given in this module, by where
	for _, c := range m.Cells {
		dd := infoEntryHTML(c.Tip, c.Keys)
		if at, ok := said[c.Tip]; ok {
			dd = `<p>As ` + html.EscapeString(at) + `.</p>`
		} else {
			said[c.Tip] = c.Loc
		}
		dt := `<dt>`
		if figures {
			dt = `<dt id="` + manualAnchor(c.Loc) + `">`
		}
		b.WriteString(dt + html.EscapeString(c.Loc) + `</dt><dd>` + dd + `</dd>`)
	}
	b.WriteString(`</dl>`)
	return b.String()
}

// manualAnchor is the id of the module or control at address loc on the
// manual page: 8.1.3 is c-8-1-3. The illustrations link to it.
func manualAnchor(loc string) string { return "c-" + strings.ReplaceAll(loc, ".", "-") }

// bayTitle is bay n as its heading names it: its number and its section, as
// the rack groups them (racksection.go), from the first module in it.
func bayTitle(n int) string {
	t := "Bay " + strconv.Itoa(n)
	var mods []struct {
		Name string `json:"h"`
	}
	if err := json.Unmarshal([]byte(fastDOM().Call("bayManual", n).String()), &mods); err == nil && len(mods) > 0 {
		sec := moduleSection(strings.ToLower(strings.TrimSpace(mods[0].Name)))
		if s := sectionTitle[sec]; s != "" {
			t += " — " + s
		} else if sec != "" {
			t += " — " + strings.ToUpper(strings.TrimPrefix(sec, "cat:")) // a category's row names itself
		}
	}
	return t
}

// infoRackOff reports whether no model is on screen, which is when Info is
// the manual.
func infoRackOff() bool { return run.stopped || editMode() == "" }

// infoEntryHTML is a control's page in the manual: its entry rendered, when
// one of keys is the entry its tooltip says (tip), else the tooltip as it
// stands — a tooltip Go has rewritten since is newer than the entry — and
// under it a selector's positions, where the manual lists them.
func infoEntryHTML(tip string, keys []string) string {
	var b strings.Builder
	found := false
	for _, k := range keys {
		if manual.Text(k) == tip {
			b.WriteString(manual.HTML(k))
			for _, l := range infoPartLists[k] {
				b.WriteString(infoListHTML(manual.Positions(l)))
			}
			found = true
			break
		}
	}
	if !found {
		b.WriteString(`<p>` + strings.ReplaceAll(html.EscapeString(tip), "\n", "<br>") + `</p>`)
	}
	for _, k := range keys {
		ps := manual.Positions(k)
		if len(ps) == 0 {
			continue
		}
		b.WriteString(infoListHTML(ps))
		break
	}
	return b.String()
}

// infoPartLists are the lists a part's page carries under it: a pin matrix's
// columns and rows, each of which is an entry of its own (the tooltip over
// it) and none of which is a control with an address.
var infoPartLists = map[string][]string{
	"mixer-matrix": {"mix-group", "mix-src", "mix-row"},
	"mod-grid":     {"mod-group", "mod-src"},
}

// infoListHTML is entries keys as a list, each without its paragraph.
func infoListHTML(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<ul>`)
	for _, k := range keys {
		b.WriteString(`<li>` + strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(manual.HTML(k)), "<p>"), "</p>") + `</li>`)
	}
	b.WriteString(`</ul>`)
	return b.String()
}

// infoModelHTML is the running model's description: its entry in the
// manual, and anything the model adds to it now (the turtle's figure).
func infoModelHTML(text string) string {
	k := "model." + run.selectedMode
	if t := manual.Text(k); t != "" && strings.HasPrefix(text, t) {
		out := manual.HTML(k)
		if rest := strings.TrimSpace(text[len(t):]); rest != "" {
			out += `<p>` + html.EscapeString(rest) + `</p>`
		}
		return out
	}
	return manual.Render(descriptionMarkdown(text))
}

// infoPart is the window's part with class cls, made the first time.
func infoPart(body js.Value, cls string) js.Value {
	if p := body.Call("querySelector", "."+cls); p.Truthy() {
		return p
	}
	if body.Call("querySelector", ".info-model").IsNull() {
		body.Set("textContent", "") // the plain text it held before it had parts
	}
	p := dom.Doc.Call("createElement", "div")
	p.Set("className", cls)
	body.Call("appendChild", p)
	return p
}
