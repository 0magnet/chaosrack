//go:build js && wasm

package attractor

import (
	"strings"
	"syscall/js"
)

// Role-aware control tooltips — SINGLE SOURCE for what every element's tooltip
// says. Each element reads "<Module> <control> — <role>", so hovering a LABEL
// says it's a label, an LED readout says it's an LED readout, a swatch says
// swatch, etc., and every one names the control it belongs to. The control name
// is taken once per cell (its primary label); roles are assigned here by element
// kind. Knobs / sliders / selects / reset buttons keep the descriptive titles
// set where they're built (those already state role + name); this pass fills in
// the pieces that otherwise share a title or have none (labels, readouts,
// swatches).

// titleWord upper-cases the first letter and lower-cases the rest of a module
// header ("POSITION" -> "Position").
func titleWord(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	r := []rune(strings.ToLower(s))
	r[0] = []rune(strings.ToUpper(string(r[0])))[0]
	return string(r)
}

// symClass appends " sym" when the label is a math parameter symbol (dt, σ, ρ,
// β, a, b, ω…), which keeps its authored (lower) case. Word labels (RAINBOW,
// SPIN X, SPEED, RADIUS) get uppercased by the base .plabel/.u-lbl CSS rule.
// Callers pass the semantic fact; for parameter labels — which mix symbols
// (most attractors) and words (radius, gain, stacks…) — labelIsSym below is
// the ONE place that decides, so the params module and the Modulation/EQ
// cards it drives can never disagree.
func symClass(base string, sym bool) string { //nolint:unparam // base kept for call-site clarity
	if sym {
		return base + " sym"
	}
	return base
}

// labelIsSym reports whether a parameter label is a math symbol (keep its
// authored case) rather than a word (uppercase): two runes or fewer (a, b,
// dt, ω) or anything containing a Greek letter. Three-plus latin letters
// (radius, gain, stacks, glide, LvA) read as words on the panel.
func labelIsSym(label string) bool {
	r := []rune(label)
	if len(r) <= 2 {
		return true
	}
	for _, c := range r {
		if c >= 0x0370 && c <= 0x03ff {
			return true
		}
	}
	return false
}

// sep joins the hierarchy levels: Module / Control / element.
const sep = " / "

// stampAll sets the same full title on every element matching sel within scope.
func stampAll(scope js.Value, sel, title string) {
	if queueStamp(sel, title, -1) {
		return
	}
	list := scope.Call("querySelectorAll", sel)
	for i := range list.Get("length").Int() {
		list.Index(i).Set("title", title)
	}
}

// cellCtl returns "Module / <primary label>" — the control level for a cell.
//
// f is what the read pass found, where there was one; see fastdom_js.go.
func cellCtl(f *cellRead, cell js.Value, mod string) string {
	if f != nil {
		if t := strings.TrimSpace(f.Label); t != "" {
			return mod + sep + t
		}
		return mod
	}
	if l := cell.Call("querySelector", ".plabel, .u-lbl"); l.Truthy() {
		if t := strings.TrimSpace(l.Get("textContent").String()); t != "" {
			return mod + sep + t
		}
	}
	return mod
}

// stampSelectorKnobs names each selector knob in a cell from its own select
// (paired by DOM order), so a concentric dual-knob cell names its rings
// distinctly (e.g. Colors / Gradient source vs Colors / Number of colors).
func stampSelectorKnobs(f *cellRead, cell js.Value, mod, fallbackCtl string) {
	n := 0
	var knobs js.Value
	var selTitle func(int) (string, bool)
	if f != nil {
		n = f.NKnob
		selTitle = func(i int) (string, bool) {
			if i < len(f.Sels) {
				return f.Sels[i], true
			}
			return "", false
		}
	} else {
		knobs = cell.Call("querySelectorAll", ".knobsel")
		sels := cell.Call("querySelectorAll", "select")
		n = knobs.Get("length").Int()
		selTitle = func(i int) (string, bool) {
			if i < sels.Get("length").Int() {
				return sels.Index(i).Get("title").String(), true
			}
			return "", false
		}
	}
	for i := range n {
		ctl := fallbackCtl
		if raw, ok := selTitle(i); ok {
			// Only borrow the select's own name when it's a structured
			// "Name — description" title; otherwise keep the cell's control name.
			if name, _, ok := strings.Cut(strings.TrimSpace(raw), " — "); ok {
				ctl = mod + sep + name
			}
		}
		if !queueStamp(".knobsel", ctl+sep+"selector knob", i) && knobs.Truthy() {
			knobs.Index(i).Set("title", ctl+sep+"selector knob")
		}
	}
}

// cellHelp is the paramHelp sentence for a cell, found from the hidden
// slider that carries the parameter id, or "" when the knob has no entry.
func cellHelp(f *cellRead, cell js.Value) string {
	if f != nil {
		return helpFor(f.RID)
	}
	s := cell.Call("querySelector", "input[type=range]")
	if !s.Truthy() {
		return ""
	}
	return helpFor(s.Get("id").String())
}

// withHelp appends the sentence to a tooltip. Used on the label, the knob
// and the readout — the three things a hand actually rests on — and not on
// every element, because the same paragraph repeated on a reset button and
// a step field is noise rather than help.
func withHelp(tip, help string) string {
	if help == "" {
		return tip
	}
	return tip + sep + help
}
