//go:build js && wasm

package attractor

import (
	_ "embed"
	"math"
	"syscall/js"

	"github.com/go-gl/mathgl/mgl32"
)

const autoRotYDelta = 0.1

// setAutoRotate turns the gentle auto-spin on or off. It sets a flag and the
// switch, and nothing else: the contribution itself is added to the Y rate in
// the render loop (see render.go), NOT written into the Y rate slider.
//
// It used to add and remove autoRotYDelta from that slider, which made the
// switch and the rate two representations of one thing. Every path that
// touched either had to keep them in step and they came apart repeatedly —
// most visibly through the permalink, which serializes ar and ry as
// independent fields.
func setAutoRotate(on bool) {
	autoRotate = on
	if el := doc.Call("getElementById", "auto-rotate"); el.Truthy() {
		el.Set("checked", on)
	}
}

// clearAutoRotateFlag marks auto-rotate off. Kept as its own name because the
// callers that use it (grabbing the Y knob, resetting Y, the spectrogram
// camera) mean "the user has taken the spin into their own hands", which reads
// differently from flipping the switch even though it now does the same thing.
func clearAutoRotateFlag() { setAutoRotate(false) }

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
