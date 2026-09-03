//go:build js && wasm

package attractor

import (
	"syscall/js"
	"testing"
)

// fakeDeskRoot builds the smallest element the desk shell touches: something
// with a style bag, answering querySelector with a panel and a menu that have
// style bags of their own.
func fakeDeskRoot(t *testing.T) (root, panel, menu js.Value) {
	t.Helper()
	mk := func() js.Value {
		el := js.Global().Get("Object").New()
		el.Set("style", js.Global().Get("Object").New())
		return el
	}
	root, panel, menu = mk(), mk(), mk()
	fn := js.FuncOf(func(_ js.Value, a []js.Value) any {
		switch a[0].String() {
		case ".dk-panel":
			return panel
		case ".dk-menu":
			return menu
		}
		return js.Undefined()
	})
	t.Cleanup(fn.Release)
	root.Set("querySelector", fn)

	prev := deskEl
	deskEl = root
	t.Cleanup(func() { deskEl = prev })
	return root, panel, menu
}

// keepFloatGeom restores the package's float geometry after a test has moved it.
func keepFloatGeom(t *testing.T) {
	t.Helper()
	x, y, w, h := floatX, floatY, floatW, floatH
	t.Cleanup(func() { floatX, floatY, floatW, floatH = x, y, w, h })
}

func TestHealFloatGeomLeavesAnArrangedWindowAlone(t *testing.T) {
	keepFloatGeom(t)
	floatX, floatY, floatW, floatH = 300, 200, 500, 400
	healFloatGeom()
	if floatX != 300 || floatY != 200 || floatW != 500 || floatH != 400 {
		t.Errorf("healed a perfectly good geometry to %v,%v %vx%v", floatX, floatY, floatW, floatH)
	}
}

// The exact numbers the bug wrote: winbox's minimize slot, saved as though the
// window had been resized there. 251 is eleven pixels ABOVE the minimum width,
// which is why the width alone proves nothing and all four have to go back.
func TestHealFloatGeomReplacesTheMinimizeSlotWholesale(t *testing.T) {
	keepFloatGeom(t)
	floatX, floatY, floatW, floatH = 24, 959, 251, 35
	healFloatGeom()
	if floatW != floatDefW || floatH != floatDefH {
		t.Errorf("size healed to %vx%v, want the %vx%v defaults", floatW, floatH, floatDefW, floatDefH)
	}
	// The position is the half that would otherwise survive: 959 is a legal
	// place to put a window and an impossible place to put a 720-tall one.
	if floatX != floatDefX || floatY != floatDefY {
		t.Errorf("position healed to %v,%v, want %v,%v", floatX, floatY, floatDefX, floatDefY)
	}
}

func TestHealFloatGeomCatchesAnImpossibleHeightOnItsOwn(t *testing.T) {
	keepFloatGeom(t)
	floatW, floatH = 900, 20 // wide enough to pass a width-only check
	healFloatGeom()
	if floatH != floatDefH {
		t.Errorf("height %v survived, want %v", floatH, floatDefH)
	}
}

func TestShowDeskPanelMovesThePanelAndTheMenuTogether(t *testing.T) {
	_, panel, menu := fakeDeskRoot(t)

	showDeskPanel(true)
	for name, el := range map[string]js.Value{"panel": panel, "menu": menu} {
		if got := el.Get("style").Get("visibility").String(); got != "visible" {
			t.Errorf("%s visibility %q, want visible", name, got)
		}
	}

	// Hidden by VISIBILITY, not display: the compositor still has to be able to
	// read the panel's rectangle to draw it when the desk is used as a model.
	showDeskPanel(false)
	for name, el := range map[string]js.Value{"panel": panel, "menu": menu} {
		if got := el.Get("style").Get("visibility").String(); got != "hidden" {
			t.Errorf("%s visibility %q, want hidden", name, got)
		}
	}
}

func TestDeskLayerVisibleTakesTheWholeEnvironment(t *testing.T) {
	root, _, _ := fakeDeskRoot(t)

	deskLayerVisible(false)
	if got := root.Get("style").Get("display").String(); got != "none" {
		t.Errorf("desk root display %q, want none", got)
	}
	deskLayerVisible(true)
	if got := root.Get("style").Get("display").String(); got != "" {
		t.Errorf("desk root display %q, want empty", got)
	}
}

// The ▤ button only reaches for the desk while the desk is the environment.
// Without Contain it is an overlay the button has no business closing.
func TestDeskContainHidesEverythingNeedsBothHalves(t *testing.T) {
	prev := deskContain
	t.Cleanup(func() { deskContain = prev })

	fakeDeskRoot(t)
	deskContain = false
	if deskContainHidesEverything() {
		t.Error("claimed the button should hide the desk with Contain off")
	}
	deskContain = true
	if !deskContainHidesEverything() {
		t.Error("claimed the button should leave the desk alone with Contain on")
	}

	// No desk built yet: nothing to hide, whatever the switch says.
	deskEl = js.Undefined()
	if deskContainHidesEverything() {
		t.Error("claimed there was a desk to hide before one was built")
	}
}
