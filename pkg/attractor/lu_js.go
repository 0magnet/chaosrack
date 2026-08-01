//go:build js && wasm

package attractor

// The Lü attractor — the third member of the Lorenz–Chen–Lü family. Reuses
// the shared double-precision RK4 integrator (integrate3D) and registers its
// params, initial condition, and description via init().

var luDT, luA, luB, luC float32 = 0.005, 36, 3, 20

func generateLu() {
	a, b, c := float64(luA), float64(luB), float64(luC)
	integrate3D(float64(luDT), func(x, y, z float64) (float64, float64, float64) {
		return a * (y - x), c*y - x*z, x*y - b*z
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
}
