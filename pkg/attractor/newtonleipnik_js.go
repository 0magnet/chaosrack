//go:build js && wasm

package attractor

// The Newton–Leipnik system — a rigid-body rotation model with linear
// feedback torque, carrying two coexisting scroll-shaped attractors. Reuses
// the shared double-precision RK4 integrator (integrate3D).

var nlDT, nlA, nlB float32 = 0.005, 0.4, 0.175

func generateNewtonLeipnik() {
	a, b := float64(nlA), float64(nlB)
	integrate3D(float64(nlDT), func(x, y, z float64) (float64, float64, float64) {
		return -a*x + y + 10*y*z, -x - 0.4*y + 5*x*z, b*z - 5*x*y
	})
}

func init() {
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
