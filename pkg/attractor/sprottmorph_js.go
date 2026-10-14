//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"syscall/js"
)

// Mode wiring for Sprott Morph: the run itself is SprottMorph
// (sprottmorphrun.go), which a native rack draws too, and the
// coefficient-space machinery is in pkg/dynamics (sprottmorph.go). This is
// the page's side: the trail into the vertex buffer, the camera, and the
// PATCH readout.

// sprottMorph is the Sprott Morph mode: the run, and its readout.
type sprottMorph struct {
	SprottMorph
	led  js.Value
	tick int
}

var morph = sprottMorph{SprottMorph: *NewSprottMorph(jamRand)}

// generateSprottMorph integrates the blended flow into the trail ring and
// keeps the PATCH readout current.
func (sp *sprottMorph) generateSprottMorph() {
	if !sp.Advance(sim.speedSteps, sim.speedScale, sim.steps) {
		return
	}
	vertices := sim.vertBuf[:sim.steps*4]
	invN := float32(1) / float32(sim.steps-1)
	sp.Trail(sim.steps, func(k int, x, y, z float64) {
		v := k * 4
		vertices[v] = float32(x)
		vertices[v+1] = float32(y)
		vertices[v+2] = float32(z)
		vertices[v+3] = float32(k) * invN
	})
	gpu.uploadVerticesOnly(vertices, gpu.drawMode, sim.steps)
	sp.tick++
	// Throttled: DOM writes are not free.
	if sp.led.Truthy() && sp.tick%10 == 0 {
		setDotText(sp.led, sp.Label())
	}
}

// syncSprottMorphExtras runs on every panel rebuild: entering the mode seeds
// the trail and fits the camera to it, and leaving it lets the next entry do
// so again.
func (sp *sprottMorph) syncSprottMorphExtras(mode string) {
	sp.led = dom.Doc.Call("getElementById", "smorph-led")
	if mode != "sprottmorph" {
		sp.Leave()
		return
	}
	if !sp.Active() {
		// Frame the warmed structure directly (no fit-ordering dependence).
		view.fitOverride = float32(sp.Enter(sim.steps))
		dist := fitDistFor(view.fitOverride)
		view.initDist = dist
		view.defaultDist = dist
		view.updateViewMatrix()
	}
}
