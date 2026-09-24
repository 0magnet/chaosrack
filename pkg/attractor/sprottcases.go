//go:build js && wasm

package attractor

import "github.com/0magnet/chaosrack/pkg/dynamics"

// Render/panel half of the Sprott "simple chaotic flows" (J. C. Sprott, "Some
// simple chaotic flows", Phys. Rev. E 50, R647, 1994), cases B–S — the same
// set built as analog circuits at glensstuff.com. Each is a 3-term-ish
// dissipative flow integrated with the shared double-precision RK4 loop
// (integrate3D). The equations themselves live untagged in sprottdata.go so
// the native chaos guard covers them.
//
// The 4D Rössler hyperchaos (O. Rössler, "An equation for hyperchaos", Phys.
// Lett. A 71, 155, 1979) render loop lives here too; it carries a hidden
// fourth state w and plots the (x,y,z) projection.

// integ3DMode tracks the mode whose IC last seeded integrate3D's x64 state.
var integ3DMode string

func init() {
	for i := range dynamics.SprottCases {
		c := dynamics.SprottCases[i]
		idx := i
		registerGenerate(c.Key, func() { generateSprottCase(idx) })
		attractorParams[c.Key] = []paramDef{
			{c.Key + "-dt", "dt", &dynamics.SprottDTs[i], c.DT, 0.001, 0.05, 0.001},
		}
	}

	attractorParams["hyperrossler"] = []paramDef{
		{"hyperrossler-dt", "dt", &dynamics.HyperDT, 0.001, 0.0002, 0.005, 0.0002},
		{"hyperrossler-a", "a", &dynamics.HyperA, 0.25, 0.01, 0.5, 0.01},
		{"hyperrossler-b", "b", &dynamics.HyperB, 3.0, 0.1, 6, 0.1},
		{"hyperrossler-c", "c", &dynamics.HyperC, 0.5, 0.05, 2, 0.01},
		{"hyperrossler-d", "d", &dynamics.HyperD, 0.05, 0.01, 0.5, 0.01},
	}
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
	dynamics.Capture(selectedMode, dt, deriv)
	vertices := vertBuf[:steps*4]
	invN := float32(1) / float32(steps-1)
	d := dt * float64(speedScale)
	ic := dynamics.InitCond[selectedMode]
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
	gpu.uploadVerticesOnly(vertices, gpu.drawMode, steps)
}

func generateSprottCase(idx int) {
	integrate3D(float64(dynamics.SprottDTs[idx]), dynamics.SprottCases[idx].Deriv)
}

// hyperRosslerWarmup advances the state through the initial transient so the
// first rendered frame already lives on the adult attractor. This system
// expands SLOWLY: from the canonical IC the orbits keep growing for over a
// thousand time units before settling into the full ±270 figure (verified in
// float64 — by t=60 the trajectory is still a small juvenile spiral, which
// made autoFitCamera frame a tiny inner arc and the mode looked broken).
// 1.5M Euler steps at dt=0.001 is ~1500 time units — a few ms of wasm time.
func hyperRosslerWarmup() {
	dt := float64(dynamics.HyperDT)
	xf, yf, zf, wf := float64(x), float64(y), float64(z), float64(dynamics.HyperW)
	// Track the attractor's real extent over the settled tail of the warmup:
	// the visible trail is only a short arc that ORBITS this structure, so
	// the camera and the (frozen) centering must frame the whole thing, not
	// the arc's momentary position.
	const nWarm = 1_500_000
	minX, maxX := 1e30, -1e30
	minY, maxY := 1e30, -1e30
	minZ, maxZ := 1e30, -1e30
	for i := 0; i < nWarm; i++ {
		dx, dy, dz, dw := dynamics.HyperDeriv(xf, yf, zf, wf)
		xf, yf, zf, wf = xf+dt*dx, yf+dt*dy, zf+dt*dz, wf+dt*dw
		// With divergent parameters the warmup itself blows up — bail
		// instead of marching through NaNs.
		if i%512 == 511 && !(xf > -1e6 && xf < 1e6 && yf > -1e6 && yf < 1e6 && zf > -1e6 && zf < 1e6) {
			return
		}
		if i > nWarm/3 {
			if xf < minX {
				minX = xf
			}
			if xf > maxX {
				maxX = xf
			}
			if yf < minY {
				minY = yf
			}
			if yf > maxY {
				maxY = yf
			}
			if zf < minZ {
				minZ = zf
			}
			if zf > maxZ {
				maxZ = zf
			}
		}
	}
	x, y, z, dynamics.HyperW = float32(xf), float32(yf), float32(zf), float32(wf)
	// Freeze centering on the structure's true middle and hand autoFitCamera
	// its true half-extent (both in display coordinates, i.e. ×dynamics.HyperScale).
	cx, cy, cz := (minX+maxX)/2, (minY+maxY)/2, (minZ+maxZ)/2
	centerOffset = [3]float32{float32(cx) * dynamics.HyperScale, float32(cy) * dynamics.HyperScale, float32(cz) * dynamics.HyperScale}
	centerReady = true
	ext := maxX - cx
	for _, e := range []float64{maxY - cy, maxZ - cz} {
		if e > ext {
			ext = e
		}
	}
	view.fitOverride = float32(ext) * dynamics.HyperScale
	// Also set the camera DIRECTLY: boot/priming call autoFitCamera in orders
	// that can pair the one-shot override with the wrong invocation, and any
	// unpaired call would fit the momentary arc (or worse) instead of the
	// structure. Direct assignment has no ordering dependence.
	dist := fitDistFor(view.fitOverride)
	view.initDist = dist
	view.defaultDist = dist
	updateViewMatrix()
}

// hyperPrimed guards the first frame after a hash boot: a page loaded
// directly on #hyperrossler renders before any resetAttractorState, so the
// state would be the unseeded origin with w=0 — which DIVERGES, and the boot
// camera fit framed that garbage (the mode looked blank; the divergence
// guard then quietly reseeded onto a healthy orbit far too small on screen).
var hyperPrimed bool

func generateHyperRossler() {
	if !hyperPrimed {
		hyperPrimed = true
		ic := dynamics.InitCond["hyperrossler"]
		x, y, z, dynamics.HyperW = ic[0], ic[1], ic[2], dynamics.HyperW0
		hyperRosslerWarmup() // also centers + fits the camera to the true extent
	}
	vertices := vertBuf[:steps*4]
	invN := float32(1) / float32(steps-1)
	sub := effSubSteps(speedSteps, steps, frameBudgetCompiled)
	for i := 0; i < steps; i++ {
		dt := float64(dynamics.HyperDT * speedScale)
		for s := 0; s < sub; s++ {
			dx, dy, dz, dw := dynamics.HyperDeriv(float64(x), float64(y), float64(z), float64(dynamics.HyperW))
			x, y, z, dynamics.HyperW = x+float32(dt*dx), y+float32(dt*dy), z+float32(dt*dz), dynamics.HyperW+float32(dt*dw)
			checkDiverged()
		}
		j := i * 4
		vertices[j], vertices[j+1], vertices[j+2], vertices[j+3] = x*dynamics.HyperScale, y*dynamics.HyperScale, z*dynamics.HyperScale, float32(i)*invN
	}
	gpu.uploadVerticesOnly(vertices, gpu.drawMode, steps)
}
