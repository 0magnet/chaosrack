//go:build js && wasm

package attractor

// Render/panel half of the Sprott "simple chaotic flows" (J. C. Sprott, "Some
// simple chaotic flows", Phys. Rev. E 50, R647, 1994), cases B–S — the same
// set built as analog circuits at glensstuff.com. Each is a 3-term-ish
// dissipative flow integrated with the shared double-precision RK4 loop
// (integrate3D). The equations themselves live untagged in sprott_data.go so
// the native chaos guard covers them.
//
// The 4D Rössler hyperchaos (O. Rössler, "An equation for hyperchaos", Phys.
// Lett. A 71, 155, 1979) render loop lives here too; it carries a hidden
// fourth state w and plots the (x,y,z) projection.

// integ3DMode tracks the mode whose IC last seeded integrate3D's x64 state.
var integ3DMode string

func init() {
	for i := range sprottCases {
		c := sprottCases[i]
		idx := i
		registerGenerate(c.key, func() { generateSprottCase(idx) })
		attractorParams[c.key] = []paramDef{
			{c.key + "-dt", "dt", &sprottDTs[i], c.dt, 0.001, 0.05, 0.001},
		}
		attractorDescriptions[c.key] = c.name +
			" — one of J. C. Sprott's simple chaotic flows (1994), realized as an" +
			" analog circuit at glensstuff.com. Found by systematic search for the" +
			" algebraically simplest systems that still produce chaos.\n\n" + c.eq
	}

	attractorParams["hyperrossler"] = []paramDef{
		{"hyperrossler-dt", "dt", &hyperDT, 0.001, 0.0002, 0.005, 0.0002},
		{"hyperrossler-a", "a", &hyperA, 0.25, 0.01, 0.5, 0.01},
		{"hyperrossler-b", "b", &hyperB, 3.0, 0.1, 6, 0.1},
		{"hyperrossler-c", "c", &hyperC, 0.5, 0.05, 2, 0.01},
		{"hyperrossler-d", "d", &hyperD, 0.05, 0.01, 0.5, 0.01},
	}
	attractorDescriptions["hyperrossler"] = "Rössler Hyperchaos (1979) — Otto Rössler's" +
		" 4-dimensional extension of his attractor, with two positive Lyapunov" +
		" exponents (hyperchaos). A hidden fourth coordinate w feeds back into y;" +
		" the plot is the (x,y,z) projection.\n\n" +
		"dx/dt = −y − z\ndy/dt = x + ay + w\ndz/dt = b + xz\ndw/dt = −cz + dw"
}

// integrate3D advances a 3D flow with a fourth-order Runge–Kutta step, keeping
// the integrator state in double precision (x64,y64,z64) *across frames* and
// writing only float32 into the GPU vertex buffer. Double precision matters
// here: several of these systems (Sprott D, L, O and the Rabinovich–Fabrikant
// system that reuses this loop) sit on a thin/weakly-bounded attractor whose
// trajectory escapes to the divergence guard — rendering blank — once state is
// rounded to float32 every frame; in float64 it stays on the attractor. The
// float32 x,y,z globals are kept in sync for the rest of the pipeline.
func integrate3D(dt float64, deriv func(x, y, z float64) (float64, float64, float64)) {
	// Publish this mode's vector field to the flow registry (Model Out FLOW
	// sonification) — free coverage for every integrate3D system.
	flowCapMode, flowCapDT, flowCapF = selectedMode, dt, deriv
	vertices := vertBuf[:steps*4]
	invN := float32(1) / float32(steps-1)
	d := dt * float64(speedScale)
	ic := attractorInitCond[selectedMode]
	// Seed the double-precision state from the initial condition when this
	// mode first runs (resetAttractorState may not have run before the first
	// frame on initial page load, which would otherwise leave x64 at 0 and
	// strand systems whose origin is a fixed point).
	if integ3DMode != selectedMode {
		x64, y64, z64 = float64(ic[0]), float64(ic[1]), float64(ic[2])
		integ3DMode = selectedMode
	}
	const lim = 1e4
	sub := effSubSteps(speedSteps, steps, frameBudgetCompiled)
	for i := 0; i < steps; i++ {
		for s := 0; s < sub; s++ {
			k1x, k1y, k1z := deriv(x64, y64, z64)
			k2x, k2y, k2z := deriv(x64+d/2*k1x, y64+d/2*k1y, z64+d/2*k1z)
			k3x, k3y, k3z := deriv(x64+d/2*k2x, y64+d/2*k2y, z64+d/2*k2z)
			k4x, k4y, k4z := deriv(x64+d*k3x, y64+d*k3y, z64+d*k3z)
			x64 += d / 6 * (k1x + 2*k2x + 2*k3x + k4x)
			y64 += d / 6 * (k1y + 2*k2y + 2*k3y + k4y)
			z64 += d / 6 * (k1z + 2*k2z + 2*k3z + k4z)
			if x64 != x64 || x64 > lim || x64 < -lim || y64 > lim || y64 < -lim || z64 > lim || z64 < -lim {
				x64, y64, z64 = float64(ic[0]), float64(ic[1]), float64(ic[2])
			}
		}
		j := i * 4
		vertices[j], vertices[j+1], vertices[j+2], vertices[j+3] = float32(x64), float32(y64), float32(z64), float32(i)*invN
	}
	x, y, z = float32(x64), float32(y64), float32(z64)
	uploadVerticesOnly(vertices, attractorDrawMode, steps)
}

func generateSprottCase(idx int) {
	integrate3D(float64(sprottDTs[idx]), sprottCases[idx].deriv)
}

// hyperRosslerWarmup advances the state through the initial transient so the
// first rendered frame already spans the full attractor — its large z-spikes
// only emerge after ~60 time units, and without this autoFitCamera fits the
// tight early spiral and the model appears zoomed-in.
func hyperRosslerWarmup() {
	dt := float64(hyperDT)
	xf, yf, zf, wf := float64(x), float64(y), float64(z), float64(hyperW)
	for i := 0; i < 60000; i++ {
		dx, dy, dz, dw := hyperDeriv(xf, yf, zf, wf)
		xf, yf, zf, wf = xf+dt*dx, yf+dt*dy, zf+dt*dz, wf+dt*dw
		// With divergent parameters the warmup itself blows up — bail
		// instead of marching 60k steps through NaNs.
		if i%512 == 511 && !(xf > -1e6 && xf < 1e6 && yf > -1e6 && yf < 1e6 && zf > -1e6 && zf < 1e6) {
			return
		}
	}
	x, y, z, hyperW = float32(xf), float32(yf), float32(zf), float32(wf)
}

func generateHyperRossler() {
	vertices := vertBuf[:steps*4]
	invN := float32(1) / float32(steps-1)
	sub := effSubSteps(speedSteps, steps, frameBudgetCompiled)
	for i := 0; i < steps; i++ {
		dt := float64(hyperDT * speedScale)
		for s := 0; s < sub; s++ {
			dx, dy, dz, dw := hyperDeriv(float64(x), float64(y), float64(z), float64(hyperW))
			x, y, z, hyperW = x+float32(dt*dx), y+float32(dt*dy), z+float32(dt*dz), hyperW+float32(dt*dw)
			checkDiverged()
		}
		j := i * 4
		vertices[j], vertices[j+1], vertices[j+2], vertices[j+3] = x*hyperScale, y*hyperScale, z*hyperScale, float32(i)*invN
	}
	uploadVerticesOnly(vertices, attractorDrawMode, steps)
}
