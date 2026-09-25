package dynamics

import "sort"

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

// Deriv is a 3-D vector field.
type Deriv func(x, y, z float64) (float64, float64, float64)

type flowSys struct {
	dt func() float64 // the mode's BASE dt (live: reads the param var)
	f  Deriv
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
func registerFlow64(mode string, dt *float32, f Deriv) {
	flowSystems[mode] = flowSys{
		dt: func() float64 { return float64(*dt) },
		f:  f,
	}
}

// integrate3D capture — refreshed every frame the shared RK4 loop runs.
var (
	flowCapMode string
	flowCapDT   float64
	flowCapF    Deriv
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
// case, so FlowFor4 is the ONE lookup every trajectory consumer (Model Out
// FLOW, the ring beam, the chaos guard) goes through — 4D equation modes
// (hyperrossler, custom) stopped being second-class citizens here.

// Deriv4 is a 4-D vector field: (x,y,z,w) to its derivatives.
type Deriv4 func(x, y, z, W float64) (dx, dy, dz, dw float64)

// FlowSys4 is a registered four-dimensional flow: its vector field, its step,
// and the hidden fourth state the renderer carries between frames.
type FlowSys4 struct {
	Dt   func() float64 // the mode's BASE dt (live: reads the param var)
	F    Deriv4
	W    func() float64  // current hidden 4th state (seed for a new integrator)
	SetW func(W float64) // write the hidden state back (beam → scan continuity)
	// Scale is the display scale applied to the (x,y,z) projection when the
	// renderer stores vertices (hyperrossler shrinks its huge natural extent);
	// beam-style consumers writing vertBuf directly must apply it too.
	Scale float32
	// W0 is the on-attractor SEED for the hidden state, as distinct from w,
	// which is wherever the running renderer has got to. A consumer starting
	// a fresh trajectory needs the seed: outside the browser w is still zero,
	// and the hyper-Rossler diverges from there.
	W0 float64
	// Interpreted marks equation-engine systems (AST evaluation, ~10× the
	// per-step cost of a compiled deriv) so consumers pick the right budget.
	Interpreted bool
	// Euler records that the mode's render loop steps forward Euler rather
	// than RK4. The integrator is part of the system the app actually runs —
	// measuring an Euler-stepped mode with RK4 (or the reverse) reports a
	// different system, which is how a chaotic Sprott read as periodic — so
	// anything reproducing a mode's dynamics has to know which it is.
	Euler bool
}

var flowSystems4 = map[string]FlowSys4{}

// RegisterFlow4 records a mode's four-dimensional flow, filling in the
// defaults a system that has no hidden state leaves out.
func RegisterFlow4(mode string, s FlowSys4) {
	if s.Scale == 0 {
		s.Scale = 1
	}
	if s.W == nil {
		s.W = func() float64 { return 0 }
	}
	if s.SetW == nil {
		s.SetW = func(float64) {}
	}
	flowSystems4[mode] = s
}

// FlowFor4 returns the 4D form of a mode's flow: native 4D systems first,
// then any 3D flow (registered or integrate3D-captured) lifted with dw≡0.
func FlowFor4(mode string) (FlowSys4, bool) {
	if s, ok := flowSystems4[mode]; ok {
		return s, true
	}
	if s3, ok := flowFor(mode); ok {
		return FlowSys4{
			Dt: s3.dt,
			F: func(x, y, z, _ float64) (float64, float64, float64, float64) {
				dx, dy, dz := s3.f(x, y, z)
				return dx, dy, dz, 0
			},
			W:     func() float64 { return 0 },
			SetW:  func(float64) {},
			Scale: 1,
		}, true
	}
	return FlowSys4{}, false
}

// InitCond seeds each mode's integrator (modes not listed use the
// generic fallback in resetAttractorState / defaultInitCond). Lives untagged
// with the registry so the native chaos test starts from the SAME state the
// app does.
var InitCond = map[string][3]float32{
	"chua":       {0.1, 0.0, 0.0},
	"rabinovich": {-1.0, 0.0, 0.5},
	"burkeshaw":  {0.6, 0.0, 0.0},
	"chen":       {-3.0, 2.0, 20.0},
	"sprott":     {0.63, 0.47, -0.54},
	"thomas":     {1.0, 0.0, 0.0},
	"halvorsen":  {-1.48, -1.51, 2.04},
}

// defaultInitCond is the fallback for modes without an InitCond row
// (must match resetAttractorState's else branch).
var defaultInitCond = [3]float32{0.1, 0.5, -0.6}

// InitCondFor is the initial condition a mode starts from.
func InitCondFor(mode string) [3]float32 {
	if ic, ok := InitCond[mode]; ok {
		return ic
	}
	return defaultInitCond
}

func init() {
	registerFlow("lorenz", &LorenzDT, lorenzDeriv)
	registerFlow("rossler", &RosslerDT, rosslerDeriv)
	registerFlow("chua", &ChuaDT, chuaDeriv)
	registerFlow("aizawa", &AizawaDT, aizawaDeriv)
	registerFlow("sprott", &SprottDT, sprottDeriv)
	registerFlow("thomas", &ThomasDT, thomasDeriv)
	registerFlow("halvorsen", &HalvorsenDT, halvorsenDeriv)
	registerFlow("chen", &ChenDT, chenDeriv)
	registerFlow("dadras", &DadrasDT, dadrasDeriv)
	registerFlow("burkeshaw", &BurkeDT, burkeShawDeriv)
}

// Capture records the vector field the shared RK4 loop was last called with.
//
// A function rather than three exported variables, because it is one fact
// written atomically: the mode, its timestep and its field describe each
// other, and a caller that set two of the three would leave FlowFor handing
// out one system's equations at another's timestep.
//
// It exists because the integrating loop lives with the renderer — it writes
// into the vertex buffer as it goes — while the registry lives here. Every
// mode that goes through that loop (Sprott B–S, Rabinovich, Lü and the rest)
// therefore has no static entry, and this is how they become lookup-able
// without a per-mode registration each.
func Capture(mode string, dt float64, f Deriv) {
	flowCapMode, flowCapDT, flowCapF = mode, dt, f
}

// FlowFor is the vector field and timestep for a mode, if it has one.
func FlowFor(mode string) (dt float64, f Deriv, ok bool) {
	s, got := flowFor(mode)
	if !got {
		return 0, nil, false
	}
	return s.dt(), s.f, true
}

// The registries are not exported, and the accessors below are the reason.
//
// A map is a poor public surface: it invites a caller to write into it from
// anywhere, it hands out the zero value for a key that is not there instead
// of saying so, and — here — its element types have unexported fields, so an
// exported map of them could be ranged over and not much else. Each of these
// answers one question and says whether it could.

// IsClassic reports whether a mode's render loop steps forward Euler with a
// bespoke float32 field, rather than going through the shared RK4 loop.
//
// The distinction is not cosmetic and it is not an implementation detail: the
// integrator is part of the system the app actually runs. Measuring an
// Euler-stepped mode with RK4 measures a different system — Sprott M reads
// λ≈0.0005 that way and would be labeled periodic while visibly filling an
// attractor.
func IsClassic(mode string) bool {
	_, ok := classicSystems[mode]
	return ok
}

// Classic is a classic system's native float32 form: the timestep BY POINTER,
// so a knob turning the parameter is seen by the next step, and the field.
func Classic(mode string) (dt *float32, f func(x, y, z float32) (float32, float32, float32), ok bool) {
	s, got := classicSystems[mode]
	if !got {
		return nil, nil, false
	}
	return s.dt, s.f, true
}

// Registered4 reports whether a mode has its own 4-D entry, as opposed to a
// 3-D one that FlowFor4 would lift with dw = 0.
func Registered4(mode string) bool {
	_, ok := flowSystems4[mode]
	return ok
}

// Unregister4 removes a 4-D system.
//
// It exists for the custom equation mode, which is whichever system the user
// last typed: when the expression stops being a flow — an iterated map, or a
// parse error — leaving the old one registered would have Model Out and the
// ring beam quietly playing the previous equation.
func Unregister4(mode string) { delete(flowSystems4, mode) }

// ClassicKeys is every classic system, sorted — the ones whose render loop
// steps forward Euler with a bespoke float32 field.
func ClassicKeys() []string {
	out := make([]string, 0, len(classicSystems))
	for k := range classicSystems {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
