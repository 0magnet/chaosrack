//go:build js && wasm

package attractor

import (
	_ "embed"
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
	"turtle-seq":   turtleSeqNames(),
	"turtle-tint":  turtleTintNames(),
	"turtle-trail": turtleTrailNames(),
	"turtle-cam":   turtleCamNames(),
	"turtle-view":  turtleViewNames(),
	"spect-dft":    spectDFTNames(),
	"spect-win":    spectWinNames,
	"spect-col":    spectColNames,
	"spect-scale":  spectScaleNames,
	"globe-par":    {"rings", "spiral"},
	"globe-rev":    {"cw", "ccw"},
	// The Poincaré section's settings. dir takes its names from beside the
	// direction constants themselves rather than repeating them here, so a
	// direction cannot be added without a name or renamed in only one place.
	// All three are short enough to ring a dial as they stand, so none of them
	// needs a paramRingLabels entry — see ringLabelsFit.
	"sect-axis": sectAxisNames,
	"sect-dir":  poincareDirNames,
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
	"spect-dft": {"64", "128", "256", "512", "1k", "2k", "4k", "8k"},
	"spect-win": {"hann", "hamm", "bart", "rect"},
	// Six, because spectColNames is six. It was three, from before turbo,
	// viridis and magma were added beside them — and a ring that does not match
	// its options is DISCARDED whole by buildParamUnit, which then falls back to
	// the full names. That is why this dial came up reading "graysca…e" and
	// "…idis" with the knob over the top of them: adding three color maps
	// silently turned the one dial whose label ring IS its readout (the LED is
	// hidden for named settings) into an unreadable one.
	"spect-col":   {"heat", "blue", "gray", "turb", "viri", "magm"},
	"spect-scale": {"log", "lin"},
	"globe-par":   {"ring", "spir"},
	"globe-rev":   {"cw", "ccw"},
	"stereo-axes": stereoAxisRing,
}

// turtlePhysParams are the weight controls. They are not in attractorParams
// because they are not the Parameters module: they get their own, which appears
// with the Physics switch and goes away with it.
var turtlePhysParams = []paramDef{
	{"turtle-grav", "grav", &turtleGravF, 3, -8, 8, 0.1},
	{"turtle-fric", "fric", &turtleFricF, 0.6, 0, 2, 0.05},
	{"turtle-bounce", "bounce", &turtleBounceF, 0.2, 0, 1, 0.05},
	{"turtle-spin", "spin", &turtleSpinF, 1, 0.1, 8, 0.1},
}

var attractorParams = map[string][]paramDef{
	"lorenz": {
		{"lorenz-dt", "dt", &lorenzDT, 0.005, 0.001, 0.05, 0.001},
		{"lorenz-s", "σ", &lorenzS, 10.0, 1, 30, 0.1},
		{"lorenz-r", "ρ", &lorenzR, 28.0, 1, 60, 0.1},
		{"lorenz-b", "β", &lorenzB, 2.7, 0.1, 10, 0.1},
	},
	"rossler": {
		{"rossler-dt", "dt", &rosslerDT, 0.005, 0.001, 0.05, 0.001},
		{"rossler-a", "a", &rosslerA, 0.2, 0.01, 1, 0.01},
		{"rossler-b", "b", &rosslerB, 0.2, 0.01, 1, 0.01},
		{"rossler-c", "c", &rosslerC, 5.7, 1, 20, 0.1},
	},
	"chua": {
		{"chua-dt", "dt", &chuaDT, 0.005, 0.001, 0.05, 0.001},
		{"chua-alpha", "α", &chuaAlpha, 15.6, 5, 30, 0.1},
		{"chua-beta", "β", &chuaBeta, 28.0, 10, 50, 0.1},
		{"chua-m0", "m0", &chuaM0, -1.143, -2, 0, 0.001},
		{"chua-m1", "m1", &chuaM1, -0.714, -2, 0, 0.001},
	},
	"aizawa": {
		{"aizawa-dt", "dt", &aizawaDT, 0.0052, 0.001, 0.02, 0.0001},
		{"aizawa-a", "a", &aizawaA, 0.95, 0.1, 2, 0.01},
		{"aizawa-b", "b", &aizawaB, 0.7, 0.1, 2, 0.01},
		{"aizawa-c", "c", &aizawaC, 0.6, 0.1, 2, 0.01},
		{"aizawa-d", "d", &aizawaD, 3.5, 0.1, 8, 0.01},
		{"aizawa-e", "e", &aizawaE, 0.25, 0.01, 1, 0.01},
		{"aizawa-f", "f", &aizawaF, 0.1, 0.01, 1, 0.01},
	},
	"sprott": {
		{"sprott-dt", "dt", &sprottDT, 0.005, 0.001, 0.05, 0.001},
		{"sprott-a", "a", &sprottA, 1.6, 0.1, 5, 0.01},
		{"sprott-b", "b", &sprottB, 1.85, 0.1, 5, 0.01},
	},
	"lissajou": {
		{"lissajou-a", "a", &lissajouA, 3, 1, 20, 1},
		{"lissajou-b", "b", &lissajouB, 2, 1, 20, 1},
		{"lissajou-c", "c", &lissajouC, 5, 1, 20, 1},
	},
	// Graphic Artist: LEVEL A/B/D + HARMONIC B/C/D (integer). Waveform A/B/C/D
	// tri/square are separate switches (buildGraphicArtistControls).
	// Short labels (Lv/Hm + oscillator letter) so they fit to the left of the
	// centered LED without overlapping it.
	"graphicartist": {
		{"ga-la", "LvA", &gaLevelA, 0.55, 0, 1, 0.05},
		{"ga-lb", "LvB", &gaLevelB, 0.45, 0, 1, 0.05},
		{"ga-ld", "LvD", &gaLevelD, 0.55, 0, 1, 0.05},
		{"ga-hb", "HmB", &gaHarmB, 2, 1, 16, 1},
		{"ga-hc", "HmC", &gaHarmC, 12, 1, 32, 1},
		{"ga-hd", "HmD", &gaHarmD, 1, 1, 16, 1},
	},
	// Fourier Text: how many harmonics of the beam tour survive.
	"scopetext": {
		{"stext-harm", "harm", &scopeTextHarm, 24, 1, 64, 1},
	},
	// Sprott Morph: position in the A…S catalog cycle + self-step rate.
	"sprottmorph": {
		{"smorph-sys", "sys", &morphSysKnob, 3, 0, 19, 0.01},
		{"smorph-rate", "rate", &morphRate, 3, 0, 10, 0.1},
	},
	// Bouncing Ball: the analog-computer demo's three panel pots.
	"bounceball": {
		{"bounce-grav", "grav", &bounceGrav, 12, 2, 30, 0.5},
		{"bounce-rest", "bounce", &bounceRest, 0.88, 0.5, 0.99, 0.01},
		{"bounce-drift", "drift", &bounceDrift, 0.7, 0, 2, 0.05},
	},
	// Scope Pong: game feel — ball speed, paddle size, machine skill.
	"pong": {
		{"pong-speed", "speed", &pongBallSpeed, 1, 0.2, 3, 0.05},
		{"pong-paddle", "paddle", &pongPaddleH, 0.42, 0.1, 0.9, 0.01},
		{"pong-skill", "skill", &pongAISkill, 0.7, 0, 1, 0.05},
	},
	"thomas": {
		{"thomas-dt", "dt", &thomasDT, 0.05, 0.001, 0.1, 0.001},
		{"thomas-b", "b", &thomasB, 0.185, 0.01, 1.0, 0.001},
	},
	"halvorsen": {
		{"halvorsen-dt", "dt", &halvorsenDT, 0.003, 0.001, 0.05, 0.001},
		{"halvorsen-a", "a", &halvorsenA, 1.4, 0.1, 5, 0.01},
	},
	"chen": {
		{"chen-dt", "dt", &chenDT, 0.0005, 0.0001, 0.005, 0.0001},
		{"chen-a", "a", &chenA, 35.0, 10, 50, 0.1},
		{"chen-b", "b", &chenB, 3.0, 0.1, 10, 0.1},
		{"chen-c", "c", &chenC, 28.0, 10, 40, 0.1},
	},
	"dadras": {
		{"dadras-dt", "dt", &dadrasDT, 0.005, 0.001, 0.05, 0.001},
		{"dadras-p", "p", &dadrasP, 3.0, 0.1, 10, 0.1},
		{"dadras-q", "q", &dadrasQ, 2.7, 0.1, 10, 0.1},
		{"dadras-r", "r", &dadrasR, 1.7, 0.1, 10, 0.1},
		{"dadras-s", "s", &dadrasS, 2.0, 0.1, 10, 0.1},
		{"dadras-e", "e", &dadrasE, 9.0, 0.1, 20, 0.1},
	},
	"rabinovich": {
		{"rab-dt", "dt", &rabDT, 0.001, 0.0001, 0.01, 0.0001},
		{"rab-alpha", "α", &rabAlpha, 1.1, 0.01, 2, 0.01},
		{"rab-gamma", "γ", &rabGamma, 0.87, 0.01, 1, 0.01},
	},
	"burkeshaw": {
		{"burke-dt", "dt", &burkeDT, 0.005, 0.001, 0.05, 0.001},
		{"burke-s", "S", &burkeS, 10.0, 1, 20, 0.1},
		{"burke-v", "V", &burkeV, 4.272, 1, 10, 0.001},
	},
	// A turtle path has no continuous parameters; these are the arithmetic
	// itself, and they are pisano's flags — mod, seq, mul, cap, reps, tint,
	// trail, cycle. MOD 0 means no modulus at all: the sequence unreduced.
	// CAP 0 lets the modulus pick its own term limit.
	"turtle": {
		{"turtle-mod", "mod", &turtleModF, 25, 0, turtleModMax, 1},
		{"turtle-seq", "seq", &turtleSeqF, 0, 0, 4, 1},
		{"turtle-mul", "mul", &turtleMulF, 1, 1, 12, 1},
		{"turtle-cap", "cap", &turtleCapF, 0, 0, 20000, 100},
		{"turtle-dim", "dim", &turtleDimF, 3, 2, 3, 1},
		{"turtle-tint", "tint", &turtleTintF, 0, 0, 6, 1},
		{"turtle-trail", "trail", &turtleTrailF, 0, 0, 3, 1},
		{"turtle-cam", "cam", &turtleCamF, 0, 0, 3, 1},
		{"turtle-view", "view", &turtleViewF, 0, 0, 4, 1},
		{"turtle-cycle", "cycle", &turtleCycleF, 0, 0, 30, 1},
	},
	// The audio spectrogram's controls are the original audioprism's, defined
	// next to the code that applies them.
	"spectrogram": spectParams,
	"globe": {
		{"globe-lat", "lat", &globeLatF, 18, 0, 90, 1},
		{"globe-lon", "lon", &globeLonF, 36, 0, 180, 1},
		{"globe-par", "par", &globeSpiralF, 0, 0, 1, 1},
		{"globe-rev", "dir", &globeRevF, 0, 0, 1, 1},
		{"globe-twist", "twist", &globeTwistF, 0, -8, 8, 0.25},
	},
	"sphere": {
		{"sphere-r", "radius", &sphereRadius, 1.0, 0.1, 5, 0.1},
		{"sphere-stacks", "lat", &sphereStacksF, 30, 4, 100, 1},
		{"sphere-slices", "lon", &sphereSlicesF, 30, 4, 100, 1},
	},
	"torus": {
		{"torus-R", "R", &torusR, 1.5, 0.1, 5, 0.1},
		{"torus-r", "r", &torusr, 0.5, 0.1, 3, 0.1},
		{"torus-stacks", "stacks", &torusStacksF, 30, 3, 100, 1},
		{"torus-slices", "slices", &torusSlicesF, 30, 3, 100, 1},
		{"torus-roll", "roll", &torusRollF, 0, -8, 8, 0.1},
	},
}
