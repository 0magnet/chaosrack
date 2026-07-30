//go:build js && wasm

package attractor

// Canonical Sprott "simple chaotic flows" (J. C. Sprott, "Some simple chaotic
// flows", Phys. Rev. E 50, R647, 1994), cases B–S — the same set built as
// analog circuits at glensstuff.com. Each is a 3-term-ish dissipative flow
// integrated with the shared double-precision RK4 loop (integrate3D).
//
// The 4D Rössler hyperchaos (O. Rössler, "An equation for hyperchaos", Phys.
// Lett. A 71, 155, 1979) lives here too; it carries a hidden fourth state w
// and plots the (x,y,z) projection.

// sprottCase is one member of the Sprott catalog. deriv returns the time
// derivatives at (x,y,z); the coefficients are baked in (Sprott's systems are
// specific, not tunable families) while dt stays user-adjustable.
type sprottCase struct {
	key   string // mode string, e.g. "sprottb"
	name  string // dropdown label, e.g. "Sprott B"
	eq    string // human-readable ODEs for the info overlay
	dt    float32
	ic    [3]float32
	deriv func(x, y, z float64) (dx, dy, dz float64)
}

var sprottCases = []sprottCase{
	{"sprottb", "Sprott B", "dx/dt = yz\ndy/dt = x − y\ndz/dt = 1 − xy", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return y * z, x - y, 1 - x*y }},
	{"sprottc", "Sprott C", "dx/dt = yz\ndy/dt = x − y\ndz/dt = 1 − x²", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return y * z, x - y, 1 - x*x }},
	{"sprottd", "Sprott D", "dx/dt = −y\ndy/dt = x + z\ndz/dt = xz + 3y²", 0.01, [3]float32{0.05, 0.05, 0.05},
		func(x, y, z float64) (float64, float64, float64) { return -y, x + z, x*z + 3*y*y }},
	{"sprotte", "Sprott E", "dx/dt = yz\ndy/dt = x² − y\ndz/dt = 1 − 4x", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return y * z, x*x - y, 1 - 4*x }},
	{"sprottf", "Sprott F", "dx/dt = y + z\ndy/dt = −x + 0.5y\ndz/dt = x² − z", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return y + z, -x + 0.5*y, x*x - z }},
	{"sprottg", "Sprott G", "dx/dt = 0.4x + z\ndy/dt = xz − y\ndz/dt = −x + y", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return 0.4*x + z, x*z - y, -x + y }},
	{"sprotth", "Sprott H", "dx/dt = −y + z²\ndy/dt = x + 0.5y\ndz/dt = x − z", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return -y + z*z, x + 0.5*y, x - z }},
	{"sprotti", "Sprott I", "dx/dt = −0.2y\ndy/dt = x + z\ndz/dt = x + y² − z", 0.01, [3]float32{0.05, 0.05, 0.05},
		func(x, y, z float64) (float64, float64, float64) { return -0.2 * y, x + z, x + y*y - z }},
	{"sprottj", "Sprott J", "dx/dt = 2z\ndy/dt = −2y + z\ndz/dt = −x + y + y²", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return 2 * z, -2*y + z, -x + y + y*y }},
	{"sprottk", "Sprott K", "dx/dt = xy − z\ndy/dt = x − y\ndz/dt = x + 0.3z", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return x*y - z, x - y, x + 0.3*z }},
	{"sprottl", "Sprott L", "dx/dt = y + 3.9z\ndy/dt = 0.9x² − y\ndz/dt = 1 − x", 0.01, [3]float32{-1, 0, 0},
		func(x, y, z float64) (float64, float64, float64) { return y + 3.9*z, 0.9*x*x - y, 1 - x }},
	{"sprottm", "Sprott M", "dx/dt = −z\ndy/dt = −x² − y\ndz/dt = 1.7 + 1.7x + y", 0.005, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return -z, -x*x - y, 1.7 + 1.7*x + y }},
	{"sprottn", "Sprott N", "dx/dt = −2y\ndy/dt = x + z²\ndz/dt = 1 + y − 2z", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return -2 * y, x + z*z, 1 + y - 2*z }},
	{"sprotto", "Sprott O", "dx/dt = y\ndy/dt = x − z\ndz/dt = x + xz + 2.7y", 0.005, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return y, x - z, x + x*z + 2.7*y }},
	{"sprottp", "Sprott P", "dx/dt = 2.7y + z\ndy/dt = −x + y²\ndz/dt = x + y", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return 2.7*y + z, -x + y*y, x + y }},
	{"sprottq", "Sprott Q", "dx/dt = −z\ndy/dt = x − y\ndz/dt = 3.1x + y² + 0.5z", 0.002, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return -z, x - y, 3.1*x + y*y + 0.5*z }},
	{"sprottr", "Sprott R", "dx/dt = 0.9 − y\ndy/dt = 0.4 + z\ndz/dt = xy − z", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return 0.9 - y, 0.4 + z, x*y - z }},
	{"sprotts", "Sprott S", "dx/dt = −x − 4y\ndy/dt = x + z²\ndz/dt = 1 + x", 0.002, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return -x - 4*y, x + z*z, 1 + x }},
}

// sprottDTs holds the live (user-adjustable) dt for each case; &sprottDTs[i]
// is the stable pointer the param slider binds to. sprottCaseIndex maps a
// mode string to its index for dispatch.
var (
	sprottDTs       []float32
	sprottCaseIndex = map[string]int{}
	// integ3DMode tracks the mode whose IC last seeded integrate3D's x64 state.
	integ3DMode string
)

// ── Rössler hyperchaos (4D) ───────────────────────────────────────────────
var (
	hyperDT float32 = 0.001
	hyperA  float32 = 0.25
	hyperB  float32 = 3.0
	hyperC  float32 = 0.5
	hyperD  float32 = 0.05
	hyperW  float32 // hidden fourth state; reset in resetAttractorState
)

func init() {
	sprottDTs = make([]float32, len(sprottCases))
	for i := range sprottCases {
		c := sprottCases[i]
		sprottDTs[i] = c.dt
		sprottCaseIndex[c.key] = i
		attractorParams[c.key] = []paramDef{
			{c.key + "-dt", "dt", &sprottDTs[i], c.dt, 0.001, 0.05, 0.001},
		}
		attractorInitCond[c.key] = c.ic
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
	attractorInitCond["hyperrossler"] = [3]float32{-10, -6, 0}
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

// hyperScale shrinks the stored coordinates so this attractor's large
// natural extent (~120×120×230) fits the camera auto-fit like the others.
// Integration still runs in the true coordinates.
const hyperScale = 0.2

// hyperRosslerWarmup advances the state through the initial transient so the
// first rendered frame already spans the full attractor — its large z-spikes
// only emerge after ~60 time units, and without this autoFitCamera fits the
// tight early spiral and the model appears zoomed-in.
func hyperRosslerWarmup() {
	dt := hyperDT
	for i := 0; i < 60000; i++ {
		w := hyperW
		x, y, z, hyperW = x+dt*(-y-z), y+dt*(x+hyperA*y+w), z+dt*(hyperB+x*z), w+dt*(-hyperC*z+hyperD*w)
		// With divergent parameters the warmup itself blows up — bail
		// instead of marching 60k steps through NaNs.
		if i%512 == 511 && !(x > -1e6 && x < 1e6 && y > -1e6 && y < 1e6 && z > -1e6 && z < 1e6) {
			return
		}
	}
}

func generateHyperRossler() {
	vertices := vertBuf[:steps*4]
	invN := float32(1) / float32(steps-1)
	sub := effSubSteps(speedSteps, steps, frameBudgetCompiled)
	for i := 0; i < steps; i++ {
		dt := hyperDT * speedScale
		for s := 0; s < sub; s++ {
			w := hyperW
			dx := -y - z
			dy := x + hyperA*y + w
			dz := hyperB + x*z
			dw := -hyperC*z + hyperD*w
			x, y, z, hyperW = x+dt*dx, y+dt*dy, z+dt*dz, w+dt*dw
			checkDiverged()
		}
		j := i * 4
		vertices[j], vertices[j+1], vertices[j+2], vertices[j+3] = x*hyperScale, y*hyperScale, z*hyperScale, float32(i)*invN
	}
	uploadVerticesOnly(vertices, attractorDrawMode, steps)
}
