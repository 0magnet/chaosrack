//go:build js && wasm

package attractor

import (
	_ "embed"
	"math"
	"strconv"
	"syscall/js"

	"github.com/go-gl/mathgl/mgl32"
)

const autoRotYDelta = 0.1

// setAutoRotate turns the gentle auto-spin on/off by adding/removing a fixed
// contribution to the Y spin rate (reflected on the Y rate knob). Idempotent;
// turning it off removes only the auto contribution, leaving a user-set Y
// rate intact. Use this for the switch and Reset All.
func setAutoRotate(on bool) {
	if on == autoRotate {
		return
	}
	autoRotate = on
	sl := doc.Call("getElementById", "rotation-controls-y")
	if sl.Truthy() {
		v, _ := strconv.ParseFloat(sl.Get("value").String(), 64)
		if on {
			v += autoRotYDelta
		} else {
			v -= autoRotYDelta
		}
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		sl.Set("value", strconv.FormatFloat(v, 'g', -1, 64))
		sl.Call("dispatchEvent", js.Global().Get("Event").New("input"))
	}
	if el := doc.Call("getElementById", "auto-rotate"); el.Truthy() {
		el.Set("checked", on)
	}
}

// clearAutoRotateFlag marks auto-rotate off WITHOUT changing the Y rate —
// for callers that have already zeroed the Y spin themselves (grabbing the
// Y knob, resetting Y, spectrogram), so the switch just reflects reality.
func clearAutoRotateFlag() {
	autoRotate = false
	if el := doc.Call("getElementById", "auto-rotate"); el.Truthy() {
		el.Set("checked", false)
	}
}

// selWindow is the little label window on the attractor selector knob.

func normalizeOrientation() {
	angleX, angleY, angleZ = 0, 0, 0
	dragMatrix = mgl32.Ident4()
	zeroRotationSliders()
	clearAutoRotateFlag()
	rebuildModelMatrix()
	updateRotKnobs()
	updateModelMatrix()
}

// randomizeOrientation gives the model a fresh random starting pose
// and a small random rotation rate on each of X/Y/Z. Called on Run()
// startup and from Reset All so the user gets a varied view each
// time instead of always starting at the identity-matrix pose. Only the
// pose is randomized — NOT the X/Y/Z spin-rate sliders. Setting random
// rates made the model spin perpetually and, because the sliders show one
// decimal, it was easy to leave a hidden nonzero rate that kept it turning
// even with auto-rotate off. Spin now comes only from auto-rotate or an
// explicit slider, so unchecking auto-rotate (with the sliders at 0)
// fully stops it.
func randomizeOrientation() {
	mathJS := js.Global().Get("Math")
	randSym := func() float32 { return float32(mathJS.Call("random").Float()*2 - 1) }

	angleX = wrapTwoPi(randSym() * float32(math.Pi))
	angleY = wrapTwoPi(randSym() * float32(math.Pi))
	angleZ = wrapTwoPi(randSym() * float32(math.Pi))
	rebuildModelMatrix()

	// Ensure the spin-rate sliders (and their cache) are zeroed.
	zeroRotationSliders()
	updateRotKnobs()
}
