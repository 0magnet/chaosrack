//go:build js && wasm

package attractor

import "strings"

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
// header ("VIEW" -> "View").
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

// stampAll sets the same full title on every element matching sel in the
// control's cell.
func stampAll(sel, title string) { queueStamp(sel, title, -1) }

// cellCtl returns "Module / <primary label>" — the control level for a cell.
//
// f is what the read pass found; see fastdom_js.go.
func cellCtl(f *cellRead, mod string) string {
	if t := strings.TrimSpace(f.Label); t != "" {
		return mod + sep + t
	}
	return mod
}

// stampSelectorKnobs names each selector knob in a cell from its own select
// (paired by DOM order), so a concentric dual-knob cell names its rings
// distinctly (e.g. Colors / Gradient source vs Colors / Number of colors).
// A cell with one selector knob is one parameter, and its knob carries that
// parameter's sentence the way a plain knob does; with two, the sentence
// could be either ring's, so neither gets it.
func stampSelectorKnobs(f *cellRead, mod, fallbackCtl, help string) {
	n := f.NKnob
	for i := range n {
		ctl, desc := fallbackCtl, ""
		if i < len(f.Sels) {
			// Only borrow the select's own name when it's a structured
			// "Name — description" title; otherwise keep the cell's control name.
			// The description comes too: it is the one place a selector says
			// what it is for, and the knob is what a hand rests on. Its address
			// is not borrowed — the knob gets its own (designators_js.go).
			t := stripAddress(strings.TrimSpace(f.Sels[i]))
			if name, d, ok := strings.Cut(t, " — "); ok {
				ctl, desc = mod+sep+name, d
			} else if n == 1 && strings.Contains(t, " ") {
				// A plain sentence ("How often the distortion measurement is
				// made…") is the description of the one knob there is.
				desc = t
			}
		}
		tip := ctl + sep + "selector knob"
		switch {
		case desc != "":
			tip = withHelp(tip, desc)
		case n == 1:
			tip = withHelp(tip, help)
		}
		queueStamp(".knobsel", tip, i)
	}
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
