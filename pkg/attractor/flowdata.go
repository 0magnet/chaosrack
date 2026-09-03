package attractor

// Data half of the systems that integrate through the shared RK4 loop: their
// vector fields, timesteps and initial conditions, untagged so that anything
// outside the browser reads the EXACT systems the app renders.
//
// This is the same split sprottdata.go makes, and for the same reason. These
// three used to declare their fields as closures inside their js-only render
// functions, which registered them in the flow registry only at draw time —
// so a host-side run saw nothing. The chaos guard could not certify them and
// the STL export had no model for them. Now the field is here, the render
// loop calls it, and the registry has it from init.

// ── Lü (Jinhu Lü and Guanrong Chen, 2002) ─────────────────────────────────
// The third member of the Lorenz–Chen–Lü family.
var luDT, luA, luB, luC float32 = 0.005, 36, 3, 20

func luDeriv(x, y, z float64) (float64, float64, float64) {
	a, b, c := float64(luA), float64(luB), float64(luC)
	return a * (y - x), c*y - x*z, x*y - b*z
}

// ── Newton–Leipnik ────────────────────────────────────────────────────────
// A rigid-body rotation model with linear feedback torque, carrying two
// coexisting scroll-shaped attractors.
var nlDT, nlA, nlB float32 = 0.005, 0.4, 0.175

func nlDeriv(x, y, z float64) (float64, float64, float64) {
	a, b := float64(nlA), float64(nlB)
	return -a*x + y + 10*y*z, -x - 0.4*y + 5*x*z, b*z - 5*x*y
}

// ── Rabinovich–Fabrikant ──────────────────────────────────────────────────
// Chaotic at α=1.1, γ=0.87 (the canonical set). Stiff, with a small basin,
// which is why it runs in double precision: in single the trajectory escapes.
var rabDT, rabAlpha, rabGamma float32 = 0.001, 1.1, 0.87

func rabDeriv(x, y, z float64) (float64, float64, float64) {
	al, ga := float64(rabAlpha), float64(rabGamma)
	return y*(z-1+x*x) + ga*x, x*(3*z+1-x*x) + ga*y, -2 * z * (al + x*y)
}

func init() {
	// The initial conditions of the two that carried theirs in their js file.
	// Rabinovich–Fabrikant is NOT set here: it already has one in the
	// registry's own table, and writing a made-up one over it put the system
	// outside its small basin, where it escaped during the transient and
	// traced nothing at all.
	attractorInitCond["lu"] = [3]float32{5, 5, 5}
	attractorInitCond["newtonleipnik"] = [3]float32{0.349, 0, -0.16}

	registerFlow64("lu", &luDT, luDeriv)
	registerFlow64("newtonleipnik", &nlDT, nlDeriv)
	registerFlow64("rabinovich", &rabDT, rabDeriv)

	// The Sprott catalog integrates through the same shared loop, so its
	// cases were invisible to the registry for the same reason. Their fields
	// are already untagged in sprottdata.go; this is the registration.
	for i := range sprottCases {
		c := &sprottCases[i]
		registerFlow64(c.key, &c.dt, c.deriv)
	}
}
