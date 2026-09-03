package attractor

// Flow-derivative registry: one place that knows each flow mode's vector
// field, so anything that wants to integrate the SAME system the renderer
// draws (Model Out's FLOW sonification, and eventually others) reads it from
// here instead of duplicating equations that could drift.
//
// Sources, in priority order:
//   - flowSystems: the classic bespoke-loop modes register their deriv here
//     (each classic file defines its deriv ONCE and both its render loop and
//     this registry use it).
//   - integrate3D capture: the shared RK4 loop stashes the (dt, deriv) it was
//     last called with, which covers every integrate3D mode (Sprott B–S,
//     Rabinovich, Lü, …) with zero per-mode wiring.
//
// Modes in neither (4D equation modes with w/t state, parametric curves,
// geometry) simply aren't flows here — callers fall back to trail scanning.

type flowDeriv func(x, y, z float64) (float64, float64, float64)

type flowSys struct {
	dt func() float64 // the mode's BASE dt (live: reads the param var)
	f  flowDeriv
}

var flowSystems = map[string]flowSys{}

// classicSys is the native-float32 form of a classic system, used by the ONE
// shared render loop (generateClassic) — no float64 round-trips in the hot
// path, unlike the flowSystems wrapper the audio integrator uses.
type classicSys struct {
	dt *float32
	f  func(x, y, z float32) (float32, float32, float32)
}

var classicSystems = map[string]classicSys{}

// registerFlow adds a classic float32 system to BOTH registries: native form
// for the render loop, float64-wrapped for the audio integrator. dt is
// captured by pointer so param edits are live.
func registerFlow(mode string, dt *float32, f func(x, y, z float32) (float32, float32, float32)) {
	classicSystems[mode] = classicSys{dt: dt, f: f}
	flowSystems[mode] = flowSys{
		dt: func() float64 { return float64(*dt) },
		f: func(x, y, z float64) (float64, float64, float64) {
			a, b, c := f(float32(x), float32(y), float32(z))
			return float64(a), float64(b), float64(c)
		},
	}
}

// registerFlow64 adds a system whose field is already double precision — the
// ones that integrate through the shared RK4 loop rather than a bespoke
// float32 render loop. They go into flowSystems only: there is no float32
// form to put in classicSystems, which is the point of them.
//
// Before this existed those systems reached the registry only through the
// integrate3D capture below, which is refreshed by the RENDER LOOP — so they
// were registered only while being drawn, and a run with no browser saw an
// empty registry where the Sprott catalog should have been.
func registerFlow64(mode string, dt *float32, f flowDeriv) {
	flowSystems[mode] = flowSys{
		dt: func() float64 { return float64(*dt) },
		f:  f,
	}
}

// integrate3D capture — refreshed every frame the shared RK4 loop runs.
var (
	flowCapMode string
	flowCapDT   float64
	flowCapF    flowDeriv
)

// flowFor returns the vector field + dt for a mode, if it is a known 3D flow.
func flowFor(mode string) (flowSys, bool) {
	if s, ok := flowSystems[mode]; ok {
		return s, true
	}
	if mode != "" && mode == flowCapMode && flowCapF != nil {
		dt := flowCapDT
		return flowSys{dt: func() float64 { return dt }, f: flowCapF}, true
	}
	return flowSys{}, false
}

// ── 4D flows ──────────────────────────────────────────────────────────────
// The general form: (x,y,z,w) → derivatives. 3D systems are the w≡0 special
// case, so flowFor4 is the ONE lookup every trajectory consumer (Model Out
// FLOW, the ring beam, the chaos guard) goes through — 4D equation modes
// (hyperrossler, custom) stopped being second-class citizens here.

type flowDeriv4 func(x, y, z, w float64) (dx, dy, dz, dw float64)

type flowSys4 struct {
	dt   func() float64 // the mode's BASE dt (live: reads the param var)
	f    flowDeriv4
	w    func() float64  // current hidden 4th state (seed for a new integrator)
	setW func(w float64) // write the hidden state back (beam → scan continuity)
	// scale is the display scale applied to the (x,y,z) projection when the
	// renderer stores vertices (hyperrossler shrinks its huge natural extent);
	// beam-style consumers writing vertBuf directly must apply it too.
	scale float32
	// w0 is the on-attractor SEED for the hidden state, as distinct from w,
	// which is wherever the running renderer has got to. A consumer starting
	// a fresh trajectory needs the seed: outside the browser w is still zero,
	// and the hyper-Rossler diverges from there.
	w0 float64
	// interpreted marks equation-engine systems (AST evaluation, ~10× the
	// per-step cost of a compiled deriv) so consumers pick the right budget.
	interpreted bool //nolint:unused // built but not wired up yet; kept deliberately
	// euler records that the mode's render loop steps forward Euler rather
	// than RK4. The integrator is part of the system the app actually runs —
	// measuring an Euler-stepped mode with RK4 (or the reverse) reports a
	// different system, which is how a chaotic Sprott read as periodic — so
	// anything reproducing a mode's dynamics has to know which it is.
	euler bool
}

var flowSystems4 = map[string]flowSys4{}

func registerFlow4(mode string, s flowSys4) {
	if s.scale == 0 {
		s.scale = 1
	}
	if s.w == nil {
		s.w = func() float64 { return 0 }
	}
	if s.setW == nil {
		s.setW = func(float64) {}
	}
	flowSystems4[mode] = s
}

// flowFor4 returns the 4D form of a mode's flow: native 4D systems first,
// then any 3D flow (registered or integrate3D-captured) lifted with dw≡0.
func flowFor4(mode string) (flowSys4, bool) {
	if s, ok := flowSystems4[mode]; ok {
		return s, true
	}
	if s3, ok := flowFor(mode); ok {
		return flowSys4{
			dt: s3.dt,
			f: func(x, y, z, _ float64) (float64, float64, float64, float64) {
				dx, dy, dz := s3.f(x, y, z)
				return dx, dy, dz, 0
			},
			w:     func() float64 { return 0 },
			setW:  func(float64) {},
			scale: 1,
		}, true
	}
	return flowSys4{}, false
}

// attractorInitCond seeds each mode's integrator (modes not listed use the
// generic fallback in resetAttractorState / defaultInitCond). Lives untagged
// with the registry so the native chaos test starts from the SAME state the
// app does.
var attractorInitCond = map[string][3]float32{
	"chua":       {0.1, 0.0, 0.0},
	"rabinovich": {-1.0, 0.0, 0.5},
	"burkeshaw":  {0.6, 0.0, 0.0},
	"chen":       {-3.0, 2.0, 20.0},
	"sprott":     {0.63, 0.47, -0.54},
	"thomas":     {1.0, 0.0, 0.0},
	"halvorsen":  {-1.48, -1.51, 2.04},
}

// defaultInitCond is the fallback for modes without an attractorInitCond row
// (must match resetAttractorState's else branch).
var defaultInitCond = [3]float32{0.1, 0.5, -0.6}

func initCondFor(mode string) [3]float32 {
	if ic, ok := attractorInitCond[mode]; ok {
		return ic
	}
	return defaultInitCond
}

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
	max := budget / points
	if max < 1 {
		max = 1
	}
	if requested > max {
		return max
	}
	return requested
}

const (
	frameBudgetCompiled    = 4_000_000 //nolint:unused // classic Euler / integrate3D RK4 steps per frame
	frameBudgetInterpreted = 400_000   //nolint:unused // equation-engine AST evaluations per frame (per expression set)
)

func init() {
	registerFlow("lorenz", &lorenzDT, lorenzDeriv)
	registerFlow("rossler", &rosslerDT, rosslerDeriv)
	registerFlow("chua", &chuaDT, chuaDeriv)
	registerFlow("aizawa", &aizawaDT, aizawaDeriv)
	registerFlow("sprott", &sprottDT, sprottDeriv)
	registerFlow("thomas", &thomasDT, thomasDeriv)
	registerFlow("halvorsen", &halvorsenDT, halvorsenDeriv)
	registerFlow("chen", &chenDT, chenDeriv)
	registerFlow("dadras", &dadrasDT, dadrasDeriv)
	registerFlow("burkeshaw", &burkeDT, burkeShawDeriv)
}
