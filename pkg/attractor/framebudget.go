package attractor

// Frame pacing: how much integration one frame may do.
//
// This lived in the flow registry, beside the systems, and it is not about
// them — a vector field has no frame rate. It is the render loop's budget, so
// when the systems moved out to pkg/dynamics it stayed behind with the loop
// that spends it.

// effSubSteps caps the per-frame integration work so the Speed knob can
// never freeze the page: at long trail lengths the requested sub-steps are
// clamped to keep steps×substeps within a frame budget (visual speed tops
// out instead of blocking the main thread for seconds — the "page
// unresponsive" class found by the demo recorder, and the likely cause of
// the one fuzzer FROZEN hit in hyperrossler). Budgets differ by engine cost:
// compiled vector fields are ~10× cheaper per step than the interpreted
// equation engine.
func effSubSteps(requested, points, budget int) int { //nolint:unused // built but not wired up yet; kept deliberately
	if requested <= 1 || points <= 0 {
		return requested
	}
	hi := budget / points
	if hi < 1 {
		hi = 1
	}
	if requested > hi {
		return hi
	}
	return requested
}

const (
	frameBudgetCompiled    = 4_000_000 //nolint:unused // classic Euler / integrate3D RK4 steps per frame
	frameBudgetInterpreted = 400_000   //nolint:unused // equation-engine AST evaluations per frame (per expression set)
)
