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
	list := scope.Call("querySelectorAll", sel)
	for i := 0; i < list.Get("length").Int(); i++ {
		list.Index(i).Set("title", title)
	}
}

// cellCtl returns "Module / <primary label>" — the control level for a cell.
func cellCtl(cell js.Value, mod string) string {
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
func stampSelectorKnobs(cell js.Value, mod, fallbackCtl string) {
	knobs := cell.Call("querySelectorAll", ".knobsel")
	sels := cell.Call("querySelectorAll", "select")
	for i := 0; i < knobs.Get("length").Int(); i++ {
		ctl := fallbackCtl
		if i < sels.Get("length").Int() {
			// Only borrow the select's own name when it's a structured
			// "Name — description" title; otherwise keep the cell's control name.
			if t := strings.TrimSpace(sels.Index(i).Get("title").String()); strings.Contains(t, " — ") {
				ctl = mod + sep + t[:strings.Index(t, " — ")]
			}
		}
		knobs.Index(i).Set("title", ctl+sep+"selector knob")
	}
}
