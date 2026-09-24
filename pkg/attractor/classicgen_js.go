//go:build js && wasm

package attractor

import "github.com/0magnet/chaosrack/pkg/dynamics"

// generateClassic is THE render loop for every classic (bespoke-Euler) flow
// mode: forward-Euler at the mode's dt, speedSteps sub-steps per stored
// point, float32 state carried across frames. It replaced ten byte-identical
// per-mode copies — each classic file now contributes only its parameter vars
// and its deriv (registered in flowregistry.go), so the equations exist in exactly
// one place, shared with the audio integrator.
func generateClassic(mode string) {
	dtp, deriv, ok := dynamics.Classic(mode)
	if !ok {
		return
	}
	vertices := sim.vertBuf[:sim.steps*4]
	invN := float32(1) / float32(sim.steps-1)
	sub := effSubSteps(sim.speedSteps, sim.steps, frameBudgetCompiled)
	for i := 0; i < sim.steps; i++ {
		dt := *dtp * sim.speedScale
		for range sub {
			dx, dy, dz := deriv(sim.x, sim.y, sim.z)
			sim.x, sim.y, sim.z = sim.x+dt*dx, sim.y+dt*dy, sim.z+dt*dz
			sim.checkDiverged()
		}
		j := i * 4
		vertices[j], vertices[j+1], vertices[j+2], vertices[j+3] = sim.x, sim.y, sim.z, float32(i)*invN
	}
	gpu.uploadVerticesOnly(vertices, gpu.drawMode, sim.steps)
}
