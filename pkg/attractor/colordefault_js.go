//go:build js && wasm

package attractor

import (
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// The page's half of the model colors (colordefault.go): who turned what, and
// putting a model's colors on the rings.

var (
	colorFollow ColorFollow
	// srcRing and mapRing are the two Controls, so a reset returns each to the
	// running model's position and not the rack's.
	srcRing, mapRing *Control
	// followingColors is set while followModelColors turns the rings itself,
	// which is not the operator turning them.
	followingColors bool
)

// followModelColors makes the rings' defaults the model's, and puts the
// model's colors on them unless a knob in the Colors module has been turned.
// Called on every change of model and once at boot, after a link has been
// applied.
func followModelColors(model string) {
	def := ModelColorDefault(model)
	if srcRing != nil {
		srcRing.selDef = def.Src
	}
	if mapRing != nil {
		mapRing.selDef = def.Map
	}
	c, ok := colorFollow.For(model)
	if !ok {
		return
	}
	followingColors = true
	defer func() { followingColors = false }()
	// The way the panel turns them, so the views, the dimming and the link
	// follow as they do for a hand on the knob.
	_ = inPageRack{}.Set("gradient-source", c.Src) //nolint:errcheck // a ring missing from the page has nothing to show
	_ = inPageRack{}.Set("gradient-colors", c.Map) //nolint:errcheck // as above
}

// noteColorChoice records a knob in the Colors module turned from outside the
// panel: by `chaosrack ctl`, or by a link that carries the rings.
func noteColorChoice(id string) {
	if !followingColors && moduleOfControl(id) == ColorsModule {
		colorFollow.Choose()
	}
}

// watchColorChoice records a hand on the Colors module: any knob, ring or
// switch in it pressed, scrolled or keyed. Its reset buttons are not a
// choice — they return a knob to the model's own position — and neither is
// its header, which is how the module is dragged.
func watchColorChoice() {
	m := dom.Doc.Call("getElementById", "layers-module")
	if !m.Truthy() {
		return
	}
	mark := js.FuncOf(func(_ js.Value, a []js.Value) any {
		e := a[0]
		if !e.Get("isTrusted").Bool() {
			return nil
		}
		if t := e.Get("target"); t.Truthy() && t.Get("closest").Truthy() && t.Call("closest", ".rst, .sect-hdr").Truthy() {
			return nil
		}
		colorFollow.Choose()
		return nil
	})
	for _, ev := range []string{"pointerdown", "wheel", "keydown"} {
		m.Call("addEventListener", ev, mark, true)
	}
}
