//go:build js && wasm

package attractor

// Additional well-known chaotic systems commonly found in other attractor
// explorers: the Lü attractor (the third member of the Lorenz–Chen–Lü
// family), Sprott case A (the conservative Nosé–Hoover oscillator, completing
// the A–S set), and the Newton–Leipnik system. Each reuses the shared
// forward-Euler integrator (integrate3D) and registers its params, initial
// condition, and description via init().

var (
	luDT, luA, luB, luC float32 = 0.005, 36, 3, 20
	sprottADT           float32 = 0.01
	nlDT, nlA, nlB      float32 = 0.005, 0.4, 0.175
)

func generateLu() {
	a, b, c := float64(luA), float64(luB), float64(luC)
	integrate3D(float64(luDT), func(x, y, z float64) (float64, float64, float64) {
		return a * (y - x), c*y - x*z, x*y - b*z
	})
}

func generateSprottA() {
	// Conservative Nosé–Hoover oscillator: needs the double-precision RK4 loop
	// (single-precision Euler damps it into a clean loop) to fill its sea.
	integrate3D(float64(sprottADT), func(x, y, z float64) (float64, float64, float64) {
		return y, -x + y*z, 1 - y*y
	})
}

func generateNewtonLeipnik() {
	a, b := float64(nlA), float64(nlB)
	integrate3D(float64(nlDT), func(x, y, z float64) (float64, float64, float64) {
		return -a*x + y + 10*y*z, -x - 0.4*y + 5*x*z, b*z - 5*x*y
	})
}

func init() {
	attractorParams["lu"] = []paramDef{
		{"lu-dt", "dt", &luDT, 0.005, 0.001, 0.02, 0.001},
		{"lu-a", "a", &luA, 36, 1, 50, 0.1},
		{"lu-b", "b", &luB, 3, 0.1, 10, 0.1},
		{"lu-c", "c", &luC, 20, 1, 40, 0.1},
	}
	attractorInitCond["lu"] = [3]float32{5, 5, 5}
	attractorDescriptions["lu"] = "Lü Attractor — Discovered by Jinhu Lü and Guanrong Chen (2002), " +
		"the third member of the Lorenz–Chen–Lü family; it forms a bridge between the Lorenz and " +
		"Chen systems and produces a two-scroll butterfly.\n\ndx/dt = a(y − x)\ndy/dt = cy − xz\ndz/dt = xy − bz"

	attractorParams["sprotta"] = []paramDef{
		{"sprotta-dt", "dt", &sprottADT, 0.0005, 0.0001, 0.02, 0.0001},
	}
	attractorInitCond["sprotta"] = [3]float32{0, 5, 0}
	attractorDescriptions["sprotta"] = "Sprott A (Nosé–Hoover oscillator) — the first of J. C. Sprott's " +
		"1994 systems and the only conservative one: rather than settling onto a strange attractor it " +
		"fills a chaotic sea.\n\ndx/dt = y\ndy/dt = −x + yz\ndz/dt = 1 − y²"

	attractorParams["newtonleipnik"] = []paramDef{
		{"nl-dt", "dt", &nlDT, 0.005, 0.001, 0.02, 0.001},
		{"nl-a", "a", &nlA, 0.4, 0.1, 1, 0.01},
		{"nl-b", "b", &nlB, 0.175, 0.01, 0.5, 0.01},
	}
	attractorInitCond["newtonleipnik"] = [3]float32{0.349, 0, -0.16}
	attractorDescriptions["newtonleipnik"] = "Newton–Leipnik Attractor — Arises from a rigid-body " +
		"rotation model with a linear feedback torque; it has two coexisting scroll-shaped attractors.\n\n" +
		"dx/dt = −ax + y + 10yz\ndy/dt = −x − 0.4y + 5xz\ndz/dt = bz − 5xy"
}
