//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/acoustics"
	"github.com/0magnet/chaosrack/pkg/analysis"
	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// ── Parameter definitions with slider ranges ─────────────────────────────────

type paramDef struct {
	ID    string
	Label string
	Value *float32
	Def   float32
	Min   float32
	Max   float32
	Step  float32
}

// paramLabels names the positions of parameters whose values are a list of
// settings rather than a quantity. Given them, the cell is built as a labeled
// rotary switch reading its setting by name instead of a knob reading a number
// — a modulus is a number, but "comet" is not the fourth of anything.
//
// The value is still the same float behind the same hidden slider, so reset,
// permalinks and audio modulation are untouched. Positions are the values
// themselves, so a labeled parameter runs 0..n-1 in steps of one.
var paramLabels = map[string][]string{
	"turtle-seq":    turtleSeqNames(),
	"turtle-tint":   turtleTintNames(),
	"turtle-trail":  turtleTrailNames(),
	"turtle-cam":    turtleCamNames(),
	"turtle-view":   turtleViewNames(),
	"spect-dft":     spectDFTNames(),
	"spect-win":     spectWinNames,
	"spect-chan":    spectChanNames,
	"spect-scale":   spectScaleNames,
	"polyhedron-op": polyOpNames(),
	"globe-par":     {"rings", "spiral"},
	"globe-rev":     {"cw", "ccw"},
	"ga-wa":         {"tri", "sqr"},
	"ga-wb":         {"tri", "sqr"},
	"ga-wc":         {"tri", "sqr"},
	"ga-wd":         {"tri", "sqr"},
	// The Poincaré section's settings. dir takes its names from beside the
	// direction constants themselves rather than repeating them here, so a
	// direction cannot be added without a name or renamed in only one place.
	// All three are short enough to ring a dial as they stand, so none of them
	// needs a paramRingLabels entry — see ringLabelsFit.
	"sect-axis": sectAxisNames,
	"sect-dir":  analysis.PoincareDirNames,
	"sect-view": sectViewNames,
	// What the recurrence plot is a plot OF — the raw audio, the delay
	// embedding of that same audio, or the running attractor's own trajectory.
	// A setting rather than a quantity: "traj" is not the third of anything.
	"rec-src": {"audio", "embed", "traj"},
	// The stereo embedding's axis assignment. Defined next to the plans it
	// indexes (stereo_js.go) rather than spelled out here, because the two
	// have to stay the same length and the same order — a list of names that
	// disagreed with the list of plans would put a dial position on a figure
	// it does not draw.
	"stereo-axes": stereoAxisNames,
	// The trigger edge, named the same way and for the same reason.
	"stereo-trig": stereoTrigNames,
	"stereo-tsrc": trigSrcNames,
	"stereo-tcpl": trigCplNames,
	"stereo-trun": trigRunNames,
	"stereo-grat": gratNames,
	// The polar embedding's radius map. Same arrangement and the same reason:
	// the names live next to the maps they index (polar_js.go), so a dial
	// position cannot come to name a curve it does not draw.
	"polar-map": polarMapNames,
	// The xy scope's basis, next to the pair it selects between for the same
	// reason as the two above.
	"xy-basis": xyBasisNames,
	// Which signal of the live pair the two accumulating embeddings reconstruct.
	// Defined next to the fold they index (audiotap_js.go) for the same reason
	// the two above are: a name that disagreed with the fold would put a dial
	// position on a signal it does not read.
	"takens-chan": tapChanNames,
	"polar-chan":  tapChanNames,
	"rta-chan":    tapChanNames,
	// The RTA's band width, next to the fractions it indexes.
	"rta-frac": acoustics.RTAFractionNames,
	"xf-frac":  acoustics.RTAFractionNames,
	// Which of the three curves the transfer display draws.
	"xf-show": xfShowNames,
	// Which channel is the reference — what went out — and which is what came
	// back. A swap rather than a rewire, because half the time the cabling makes
	// it the other way round.
	"xf-swap":    {"left is reference", "right is reference"},
	"wfall-swap": {"left is reference", "right is reference"},
	// Which surface: the cumulative spectral decay of a measured impulse, which
	// holds still between sweeps, or a live stack of spectra of whatever is
	// playing, which is the other thing the word waterfall means.
	"wfall-src": {"decay from a sweep", "live spectra"},
	// Which channel the live surface analyses. The decay surface has no such
	// choice — it needs both, and REF says which of the two is the reference.
	"wfall-chan": tapChanNames,
	"wfall-fft":  wfallFFTLabels,
}

// paramRingLabels is what actually fits around the dial. A cell is a third of
// the module wide, so a label much past four characters runs into its
// neighbors — the full name from paramLabels stays on as the position's
// tooltip, and the highlighted label is the readout (a seven-segment LED can
// spell "whole" only as "bhoL").
var paramRingLabels = map[string][]string{
	"turtle-seq":   {"fib", "luc", "tri", "nat", "prm"},
	"turtle-tint":  {"step", "pass", "vis", "head", "turn", "term", "age"},
	"turtle-trail": {"whol", "long", "shrt", "comt"},
	"turtle-cam":   {"auto", "fit", "lock", "head"},
	"turtle-view":  {"free", "end", "back", "acrs", "up"},
	// Transform sizes abbreviated past 512, where the digits stop fitting.
	"spect-dft":   {"64", "128", "256", "512", "1k", "2k", "4k", "8k"},
	"spect-win":   {"hann", "hamm", "bart", "rect"},
	"spect-scale": {"log", "lin"},
	"globe-par":   {"ring", "spir"},
	"globe-rev":   {"cw", "ccw"},
	"stereo-axes": stereoAxisRing,
	"stereo-trig": stereoTrigRing,
	"stereo-tsrc": trigSrcRing,
	"stereo-tcpl": trigCplRing,
	"stereo-trun": trigRunRing,
	"stereo-grat": gratRing,
	"polar-map":   polarMapRing,
	"xy-basis":    xyBasisRing,
	"takens-chan": tapChanRing,
	"polar-chan":  tapChanRing,
	"rta-chan":    tapChanRing,
	"rta-frac":    acoustics.RTAFractionRing,
	"xf-frac":     acoustics.RTAFractionRing,
	"xf-show":     xfShowRing,
	"xf-swap":     {"L", "R"},
	"wfall-swap":  {"L", "R"},
	"wfall-src":   {"dcay", "live"},
	"wfall-chan":  tapChanRing,
	"wfall-fft":   wfallFFTRing,
}

// turtlePhysParams are the weight controls. They are not in attractorParams
// because they are not the Parameters module: they get their own, which appears
// with the Physics switch and goes away with it.
var turtlePhysParams = []paramDef{
	{"turtle-grav", "grav", &turtle.gravF, 3, -8, 8, 0.1},
	{"turtle-fric", "fric", &turtle.fricF, 0.6, 0, 2, 0.05},
	{"turtle-bounce", "bounce", &turtle.bounceF, 0.2, 0, 1, 0.05},
	{"turtle-spin", "spin", &turtle.spinF, 1, 0.1, 8, 0.1},
}

// quietParams are parameters the model turns itself — Pong's motorized
// paddle pots, written back twenty times a second while the machine plays.
// Turning one reseeds nothing and redraws nothing, and a link does not carry
// where a paddle happened to be.
var quietParams = map[string]bool{"pong-pad-l": true, "pong-pad-r": true}

var attractorParams = map[string][]paramDef{
	"lissajou": {
		{"lissajou-a", "a", &liss.a, 3, 1, 20, 1},
		{"lissajou-b", "b", &liss.b, 2, 1, 20, 1},
		{"lissajou-c", "c", &liss.c, 5, 1, 20, 1},
	},
	// Graphic Artist: LEVEL A/B/D + HARMONIC B/C/D (integer) + WAVEFORM A/B/C/D,
	// triangle or square (the article's S1–S4, positions of the bank).
	// Short labels (Lv/Hm + oscillator letter) so they fit to the left of the
	// centered LED without overlapping it.
	"graphicartist": {
		{"ga-la", "LvA", &ga.levelA, 0.55, 0, 1, 0.05},
		{"ga-lb", "LvB", &ga.levelB, 0.45, 0, 1, 0.05},
		{"ga-ld", "LvD", &ga.levelD, 0.55, 0, 1, 0.05},
		{"ga-hb", "HmB", &ga.harmB, 2, 1, 16, 1},
		{"ga-hc", "HmC", &ga.harmC, 12, 1, 32, 1},
		{"ga-hd", "HmD", &ga.harmD, 1, 1, 16, 1},
		{"ga-wa", "WvA", &ga.waveA, 0, 0, 1, 1},
		{"ga-wb", "WvB", &ga.waveB, 0, 0, 1, 1},
		{"ga-wc", "WvC", &ga.waveC, 0, 0, 1, 1},
		{"ga-wd", "WvD", &ga.waveD, 0, 0, 1, 1},
	},
	// Fourier Text: how many harmonics of the beam tour survive.
	"scopetext": {
		{"stext-harm", "harm", &ftext.harm, 24, 1, 64, 1},
	},
	// Sprott Morph: position in the A…S catalog cycle + self-step rate.
	"sprottmorph": {
		{"smorph-sys", "sys", &morph.sysKnob, 3, 0, 19, 0.01},
		{"smorph-rate", "rate", &morph.rate, 3, 0, 10, 0.1},
	},
	// Bouncing Ball: the analog-computer demo's panel pots, and the drop
	// height its Drop switch releases from.
	"bounceball": {
		{"bounce-grav", "grav", &ball.grav, 12, 2, 30, 0.5},
		{"bounce-rest", "bounce", &ball.rest, 0.88, 0.5, 0.99, 0.01},
		{"bounce-drift", "drift", &ball.drift, 0.7, 0, 2, 0.05},
		{"bounce-height", "height", &ball.height, 0.9, 0.2, 1, 0.05},
	},
	// Scope Pong: game feel — ball speed, paddle size, machine skill — and
	// the two paddle pots (motorized: quietParams).
	"pong": {
		{"pong-speed", "speed", &pong.ballSpeed, 1, 0.2, 3, 0.05},
		{"pong-paddle", "paddle", &pong.paddleH, 0.42, 0.1, 0.9, 0.01},
		{"pong-skill", "skill", &pong.aiSkill, 0.7, 0, 1, 0.05},
		{"pong-pad-l", "left", &pong.potL, 0, -1, 1, 0.01},
		{"pong-pad-r", "right", &pong.potR, 0, -1, 1, 0.01},
	},
	// A turtle path has no continuous parameters; these are the arithmetic
	// itself, and they are pisano's flags — mod, seq, mul, cap, reps, tint,
	// trail, cycle. MOD 0 means no modulus at all: the sequence unreduced.
	// CAP 0 lets the modulus pick its own term limit.
	"turtle": {
		{"turtle-mod", "mod", &turtle.modF, 25, 0, turtleModMax, 1},
		{"turtle-seq", "seq", &turtle.seqF, 0, 0, 4, 1},
		{"turtle-mul", "mul", &turtle.mulF, 1, 1, 12, 1},
		{"turtle-cap", "cap", &turtle.capF, 0, 0, 20000, 100},
		{"turtle-dim", "dim", &turtle.dimF, 3, 2, 3, 1},
		{"turtle-tint", "tint", &turtle.tintF, 0, 0, 6, 1},
		{"turtle-trail", "trail", &turtle.trailF, 0, 0, 3, 1},
		{"turtle-cam", "cam", &turtle.camF, 0, 0, 3, 1},
		{"turtle-view", "view", &turtle.viewF, 0, 0, 4, 1},
		{"turtle-cycle", "cycle", &turtle.cycleF, 0, 0, 30, 1},
	},
	// The audio spectrogram's controls are the original audioprism's, defined
	// next to the code that applies them.
	"spectrogram": spectParams,
	// The Polyhedron: {p,q} on two knobs, then what is done to the solid.
	// See conway_js.go. The operator keeps the id the five seeds shared.
	"polyhedron": {
		{"polyhedron-p", "sides", &poly.pF, 3, 3, 6, 1},
		{"polyhedron-q", "meet", &poly.qF, 3, 3, 6, 1},
		{"polyhedron-morph", "morph", &poly.morphF, 0, 0, 2, 0.01},
		{"polyhedron-op", "op", &polyOpF, 0, 0, 6, 1},
		{"polyhedron-kis", "kis", &poly.kisF, 0, -0.9, 1, 0.01},
	},

	"globe": {
		{"globe-lat", "lat", &globe.latF, 18, 0, 90, 1},
		{"globe-lon", "lon", &globe.lonF, 36, 0, 180, 1},
		{"globe-par", "par", &globe.spiralF, 0, 0, 1, 1},
		{"globe-rev", "dir", &globe.revF, 0, 0, 1, 1},
		{"globe-twist", "twist", &globe.twistF, 0, -8, 8, 0.25},
	},
	"torus": {
		{"torus-R", "R", &torus.major, 1.5, 0.1, 5, 0.1},
		{"torus-r", "r", &torus.minor, 0.5, 0.1, 3, 0.1},
		{"torus-stacks", "stacks", &torus.stacksF, 30, 3, 100, 1},
		{"torus-slices", "slices", &torus.slicesF, 30, 3, 100, 1},
		{"torus-roll", "roll", &torus.rollF, 0, -8, 8, 0.1},
	},
}

// helpFor is what a parameter's knob DOES, keyed by its id: its entry in the
// manual (parameters.md, as p.<id>), or else the generated sentence for one of
// a system's own constants.
//
// The tooltip machinery names a control by where it sits — "Stereo ▸ smth ▸
// knob" — which answers "what am I hovering" and not "what is this for". On a
// panel whose labels are four characters because that is what fits under a
// dial, the second question is the one that goes unanswered: "smth" is
// unguessable, and so are algn, drv, τ and vg until someone reads the source.
// So a label that is an abbreviation spells the word out, and every entry says
// what turning it does rather than restating the name.
func helpFor(id string) string {
	if h := doc("p." + id); h != "" {
		return h
	}
	return systemConstantHelp[id]
}

// The systems' own constants come from pkg/dynamics rather than being listed
// again here.
//
// They used to be 43 rows in this file, pointing at variables in that package
// — the table and the values it described on opposite sides of a build tag,
// so nothing without a browser could read the ranges of the very systems that
// integrate perfectly well without one. dynamics.Params is that table, and
// this fills the same map from it, so the panel still builds the same knobs
// and there is one list to be wrong.
//
// The modes below this line keep their rows here, because their values are
// package variables of the front end (turtle's physics, the scope's harmonics,
// the globe's winding) and a vector field package has no business owning them.
func init() {
	for _, mode := range dynamics.ParamModes() {
		ps := dynamics.Params(mode)
		rows := make([]paramDef, 0, len(ps))
		for _, p := range ps {
			rows = append(rows, paramDef{p.ID, p.Label, p.Value, p.Def, p.Min, p.Max, p.Step})
		}
		attractorParams[mode] = rows
	}
}
