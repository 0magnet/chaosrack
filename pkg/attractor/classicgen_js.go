//go:build js && wasm

package attractor

// generateClassic is THE render loop for every classic (bespoke-Euler) flow
// mode: forward-Euler at the mode's dt, speedSteps sub-steps per stored
// point, float32 state carried across frames. It replaced ten byte-identical
// per-mode copies — each classic file now contributes only its parameter vars
// and its deriv (registered in flowreg.go), so the equations exist in exactly
// one place, shared with the audio integrator.
func generateClassic(mode string) {
	sys, ok := classicSystems[mode]
	if !ok {
		return
	}
	vertices := vertBuf[:steps*4]
	invN := float32(1) / float32(steps-1)
	sub := effSubSteps(speedSteps, steps, frameBudgetCompiled)
	for i := 0; i < steps; i++ {
		dt := *sys.dt * speedScale
		for s := 0; s < sub; s++ {
			dx, dy, dz := sys.f(x, y, z)
			x, y, z = x+dt*dx, y+dt*dy, z+dt*dz
			checkDiverged()
		}
		j := i * 4
		vertices[j], vertices[j+1], vertices[j+2], vertices[j+3] = x, y, z, float32(i)*invN
	}
	uploadVerticesOnly(vertices, attractorDrawMode, steps)
}
