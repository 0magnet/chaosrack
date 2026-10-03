//go:build js && wasm

package attractor

// The Display module, built from P-units (buildPUnitModule): LINE, POINTS
// and TRAIL, how the trace is drawn, and on LINE's buttons what is drawn
// over it. The switches that were columns of their own beside them are
// these P-units' buttons now. Each is still the checkbox it was, hidden in
// the module, so the permalink, Reset All and the code that reads it are
// unchanged; the buttons set it, and light from it.

import "github.com/0magnet/chaosrack/pkg/dom"

// setSwitch sets checkbox id as a hand would, firing its change.
func setSwitch(id string, on bool) {
	sw := dom.Doc.Call("getElementById", id)
	if !sw.Truthy() || sw.Get("checked").Bool() == on {
		return
	}
	sw.Set("checked", on)
	dom.Fire(sw, "change")
}

// checkedOn reports whether checkbox id is checked.
func checkedOn(id string) bool {
	sw := dom.Doc.Call("getElementById", id)
	return sw.Truthy() && sw.Get("checked").Bool()
}

func init() {
	// POINTS: + draws every vertex as a dot; 0 is the solid line, no dots
	// and no breaks. The knob between breaks the line into that many points.
	trioPrograms["dash-duty"] = trioProgram{
		help: []string{doc("trio.dash-duty=0"), doc("trio.dash-duty=1")},
		press: func(i int) {
			switch i {
			case 0:
				setSwitch("use-points", true)
			case 1:
				setSwitch("use-points", false)
				setParamSlider("dash-duty", 0)
			}
		},
		lit: func() int {
			switch {
			case checkedOn("use-points"):
				return 0
			case paramSliderValue("dash-duty") == 0:
				return 1
			}
			return -1
		},
		drive: []string{"use-points", "dash-duty"},
	}
	// TRAIL: how the trail is kept. + persists it, never clearing; 0 is the
	// scan, the whole trail drawn again each frame; − is the ring, only the
	// advancing head integrated and the trail its history. One at a time.
	trioPrograms["trail-slider"] = trioProgram{
		help: []string{doc("trio.trail-slider=0"), doc("trio.trail-slider=1"), doc("trio.trail-slider=2")},
		press: func(i int) {
			setSwitch("persist-trail", i == 0)
			setSwitch("ring-sw", i == 2)
		},
		lit: func() int {
			switch {
			case checkedOn("persist-trail"):
				return 0
			case checkedOn("ring-sw"):
				return 2
			}
			return 1
		},
		drive: []string{"persist-trail", "ring-sw"},
	}
	// LINE: what is drawn over the trace, each a switch of its own: S the
	// Poincaré section, G the graticule behind a scope trace.
	over := []string{"sect-sw", "scope-grat"}
	trioPrograms["line-width"] = trioProgram{
		keys: []string{"S", "G", ""},
		help: []string{doc("trio.line-width=0"), doc("trio.line-width=1")},
		press: func(i int) {
			if i < len(over) {
				setSwitch(over[i], !checkedOn(over[i]))
			}
		},
		lits:  func() []bool { return []bool{checkedOn(over[0]), checkedOn(over[1]), false} },
		drive: over,
	}
}

// paramSliderValue is the value of slider id, or -1 where there is none.
func paramSliderValue(id string) float64 {
	s := dom.Doc.Call("getElementById", id)
	if !s.Truthy() {
		return -1
	}
	return parseOr0(s.Get("value").String())
}

// Layers · Colors (buildPUnitModule "layers-bank"): BEHIND, SKIN, SRC, MAP,
// PERIOD and SHIFT are P-units beside the four color knobs, and the
// switches that were a column of their own are their buttons. Each is a
// switch of its own, lit independently.
func init() {
	toggle := func(id string) func(int) { return func(int) { setSwitch(id, !checkedOn(id)) } }
	// BEHIND: F fills the screen with a spectrogram or FVF backdrop, face
	// on; B points the Colors module at the backdrop's colors instead of the
	// model's. Both are about what is behind, so they are its buttons.
	trioPrograms["bg-visual"] = trioProgram{
		keys: []string{"F", "B", ""},
		help: []string{doc("trio.bg-visual=0"), doc("trio.bg-visual=1")},
		press: func(i int) {
			switch i {
			case 0:
				toggle("spect-fill")(i)
			case 1:
				toggle("edit-back")(i)
			}
		},
		lits:  func() []bool { return []bool{checkedOn("spect-fill"), checkedOn("edit-back"), false} },
		drive: []string{"spect-fill", "edit-back"},
	}
	// MAP: I reverses the palette.
	trioPrograms["gradient-colors"] = trioProgram{
		keys:  []string{"I", "", ""},
		help:  []string{doc("trio.gradient-colors=0")},
		press: func(i int) { toggle("gradient-reverse")(i) },
		lits:  func() []bool { return []bool{checkedOn("gradient-reverse"), false, false} },
		drive: []string{"gradient-reverse"},
	}
	// SRC: H holds the color range where it is instead of refitting it to
	// the source as it changes.
	trioPrograms["gradient-source"] = trioProgram{
		keys:  []string{"H", "", ""},
		help:  []string{doc("trio.gradient-source=0")},
		press: func(i int) { toggle("color-lock-sw")(i) },
		lits:  func() []bool { return []bool{checkedOn("color-lock-sw"), false, false} },
		drive: []string{"color-lock-sw", "color-lock"},
	}
}

// settingNames are what a P-unit's setting display says for each position,
// by selector id, where an option's own name is longer than a display holds
// (bankValChars). The list (ledPick) still has the full names. Not
// paramRingLabels: those are printed round a dial, and held to its limits.
var settingNames = map[string][]string{
	"bg-visual":       {"off", "spectro", "xy scope", "terminal", "anim", "desk", "water"},
	"skin-visual":     {"off", "spectro", "terminal", "desk"},
	"gradient-colors": {"2-color", "3-color", "hue", "heat", "blue", "gray", "turbo", "viridis", "magma"},
}
