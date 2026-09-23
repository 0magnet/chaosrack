//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"syscall/js"

	"github.com/go-gl/mathgl/mgl32"
)

// Drawing the scope graticule. The geometry is in scopegrat.go; this is only
// the part that has to know about GL.
//
// Three draw calls rather than one, because the three weights are the point:
// a graticule whose ticks are as bright as its center axes is a grid, and a
// grid behind a trace is something to look past rather than something to
// read against. The brightness ladder is what a photo-etched face does with
// line width, which a one-pixel line renderer cannot do.

// gratShade is how bright each weight is drawn, as a multiple of the base
// graticule gray. The center axes sit above the division lines and the ticks
// well below them — on a real face the ticks are hairlines you find when you
// look for them and ignore otherwise.
var gratShade = map[gratWeight]float32{
	gratWeightAxis: 1.45,
	gratWeightDiv:  1.0,
	gratWeightTick: 0.6,
}

// gratBase is the graticule gray the shades multiply, matched to the stereo
// goniometer's own so the two graticules read as the same instrument.
var gratBase = [3]float32{0.22, 0.26, 0.32}

// scopeGratBuf is the per-weight vertex scratch, kept between frames for the
// reason gratBuf is: the figure is fixed and reallocating it every frame
// would be the largest thing this function does.
var scopeGratBuf []float32

// drawScopeGraticule puts the scope's ruled face up behind the trace.
//
// halfH is the world half-height of the screen — the divisions are scaled to
// it, and the width follows from the 10-by-8 aspect rather than from the
// canvas, so a division is SQUARE. That is not cosmetic: a division is the
// unit both axes are read in, and a graticule whose divisions are oblong
// makes every volts-per-division reading a different size from every
// seconds-per-division one.
func drawScopeGraticule(halfH float32) {
	if !(halfH > 0) {
		return
	}
	// Face-on, always. The graticule is the FACE OF THE TUBE and not an
	// object in the scene: it does not tumble when the model is dragged, it
	// does not foreshorten, and the trace moves across it rather than with
	// it. Drawn under an identity model matrix and put back afterwards, the
	// way the color uniforms below are — the pose the operator set is still
	// the pose, and the next thing drawn has to see it.
	gl.Call("useProgram", shaderProgram)
	gl.Call("uniformMatrix4fv", uMmatrixLoc, false, mat4ToTyped(&identMatrix))
	defer updateModelMatrix()

	perDiv := halfH / float32(gratHalfH)
	lines := scopeGraticule()
	if cap(scopeGratBuf) < len(lines)*8 {
		scopeGratBuf = make([]float32, 0, len(lines)*8)
	}

	// One pass per weight, in the order they stack: ticks first so the
	// heavier lines land on top of them where they cross.
	for _, w := range []gratWeight{gratWeightTick, gratWeightDiv, gratWeightAxis} {
		v := scopeGratBuf[:0]
		for _, l := range lines {
			if l.W != w {
				continue
			}
			// The fourth float is the color coordinate, which the flat
			// override ignores; a constant keeps it out of any gradient
			// still bound. Same convention as drawGraticule.
			v = append(v,
				l.X0*perDiv, l.Y0*perDiv, 0, 0,
				l.X1*perDiv, l.Y1*perDiv, 0, 0)
		}
		if len(v) == 0 {
			continue
		}
		s := gratShade[w]
		gl.Call("uniform1i", uGradientColorsLoc, 1)
		gl.Call("uniform3f", uBaseColorLoc, gratBase[0]*s, gratBase[1]*s, gratBase[2]*s)
		uploadVerticesOnly(v, glTypes.Lines, len(v)/4)
	}

	// Hand the uniforms back, exactly as drawGraticule does: the trail's own
	// gradient is whatever the color knobs say, and leaving the override set
	// paints the next thing drawn in graticule gray.
	if phosphorActive() {
		// The phosphor owns these two while it is on.
		applyPhosphorColor()
		return
	}
	gl.Call("uniform1i", uGradientColorsLoc, gradientColorsUniform())
	gl.Call("uniform3f", uBaseColorLoc, baseColor[0], baseColor[1], baseColor[2])
}

// scopeFaceOn reports whether the scope face should be drawn: the CRT look is
// showing, and the operator has not switched the graticule off.
func scopeFaceOn() bool { return crtLook() && scopeGratWanted() }

func scopeGratEl() js.Value { return dom.Doc.Call("getElementById", "scope-grat") }

// scopeGratWanted reads the switch. Absent, the face is on: the graticule is
// what makes a scope trace measurable, so it is the default and the switch is
// there to take it away.
func scopeGratWanted() bool {
	el := scopeGratEl()
	if !el.Truthy() {
		return true
	}
	return el.Get("checked").Bool()
}

// identMatrix is the face-on pose the graticule is drawn under. A package
// variable rather than a literal because mat4ToTyped takes its address.
var identMatrix = mgl32.Ident4()
