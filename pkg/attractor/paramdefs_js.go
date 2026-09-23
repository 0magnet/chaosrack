//go:build js && wasm

package attractor

import (
	_ "embed"

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
	"turtle-seq":   turtleSeqNames(),
	"turtle-tint":  turtleTintNames(),
	"turtle-trail": turtleTrailNames(),
	"turtle-cam":   turtleCamNames(),
	"turtle-view":  turtleViewNames(),
	"spect-dft":    spectDFTNames(),
	"spect-win":    spectWinNames,
	"spect-chan":   spectChanNames,
	"spect-scale":  spectScaleNames,
	"poly-op":      polyOpNames(),
	"globe-par":    {"rings", "spiral"},
	"globe-rev":    {"cw", "ccw"},
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
	"rta-frac": rtaFractionNames,
	"xf-frac":  rtaFractionNames,
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
	"rta-frac":    rtaFractionRing,
	"xf-frac":     rtaFractionRing,
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
	{"turtle-grav", "grav", &turtleGravF, 3, -8, 8, 0.1},
	{"turtle-fric", "fric", &turtleFricF, 0.6, 0, 2, 0.05},
	{"turtle-bounce", "bounce", &turtleBounceF, 0.2, 0, 1, 0.05},
	{"turtle-spin", "spin", &turtleSpinF, 1, 0.1, 8, 0.1},
}

var attractorParams = map[string][]paramDef{
	"lorenz": {
		{"lorenz-dt", "dt", &dynamics.LorenzDT, 0.005, 0.001, 0.05, 0.001},
		{"lorenz-s", "σ", &dynamics.LorenzS, 10.0, 1, 30, 0.1},
		{"lorenz-r", "ρ", &dynamics.LorenzR, 28.0, 1, 60, 0.1},
		{"lorenz-b", "β", &dynamics.LorenzB, 2.7, 0.1, 10, 0.1},
	},
	"rossler": {
		{"rossler-dt", "dt", &dynamics.RosslerDT, 0.005, 0.001, 0.05, 0.001},
		{"rossler-a", "a", &dynamics.RosslerA, 0.2, 0.01, 1, 0.01},
		{"rossler-b", "b", &dynamics.RosslerB, 0.2, 0.01, 1, 0.01},
		{"rossler-c", "c", &dynamics.RosslerC, 5.7, 1, 20, 0.1},
	},
	"chua": {
		{"chua-dt", "dt", &dynamics.ChuaDT, 0.005, 0.001, 0.05, 0.001},
		{"chua-alpha", "α", &dynamics.ChuaAlpha, 15.6, 5, 30, 0.1},
		{"chua-beta", "β", &dynamics.ChuaBeta, 28.0, 10, 50, 0.1},
		{"chua-m0", "m0", &dynamics.ChuaM0, -1.143, -2, 0, 0.001},
		{"chua-m1", "m1", &dynamics.ChuaM1, -0.714, -2, 0, 0.001},
	},
	"aizawa": {
		{"aizawa-dt", "dt", &dynamics.AizawaDT, 0.0052, 0.001, 0.02, 0.0001},
		{"aizawa-a", "a", &dynamics.AizawaA, 0.95, 0.1, 2, 0.01},
		{"aizawa-b", "b", &dynamics.AizawaB, 0.7, 0.1, 2, 0.01},
		{"aizawa-c", "c", &dynamics.AizawaC, 0.6, 0.1, 2, 0.01},
		{"aizawa-d", "d", &dynamics.AizawaD, 3.5, 0.1, 8, 0.01},
		{"aizawa-e", "e", &dynamics.AizawaE, 0.25, 0.01, 1, 0.01},
		{"aizawa-f", "f", &dynamics.AizawaF, 0.1, 0.01, 1, 0.01},
	},
	"sprott": {
		{"sprott-dt", "dt", &dynamics.SprottDT, 0.005, 0.001, 0.05, 0.001},
		{"sprott-a", "a", &dynamics.SprottA, 1.6, 0.1, 5, 0.01},
		{"sprott-b", "b", &dynamics.SprottB, 1.85, 0.1, 5, 0.01},
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
		{"thomas-dt", "dt", &dynamics.ThomasDT, 0.05, 0.001, 0.1, 0.001},
		{"thomas-b", "b", &dynamics.ThomasB, 0.185, 0.01, 1.0, 0.001},
	},
	"halvorsen": {
		{"halvorsen-dt", "dt", &dynamics.HalvorsenDT, 0.003, 0.001, 0.05, 0.001},
		{"halvorsen-a", "a", &dynamics.HalvorsenA, 1.4, 0.1, 5, 0.01},
	},
	"chen": {
		{"chen-dt", "dt", &dynamics.ChenDT, 0.0005, 0.0001, 0.005, 0.0001},
		{"chen-a", "a", &dynamics.ChenA, 35.0, 10, 50, 0.1},
		{"chen-b", "b", &dynamics.ChenB, 3.0, 0.1, 10, 0.1},
		{"chen-c", "c", &dynamics.ChenC, 28.0, 10, 40, 0.1},
	},
	"dadras": {
		{"dadras-dt", "dt", &dynamics.DadrasDT, 0.005, 0.001, 0.05, 0.001},
		{"dadras-p", "p", &dynamics.DadrasP, 3.0, 0.1, 10, 0.1},
		{"dadras-q", "q", &dynamics.DadrasQ, 2.7, 0.1, 10, 0.1},
		{"dadras-r", "r", &dynamics.DadrasR, 1.7, 0.1, 10, 0.1},
		{"dadras-s", "s", &dynamics.DadrasS, 2.0, 0.1, 10, 0.1},
		{"dadras-e", "e", &dynamics.DadrasE, 9.0, 0.1, 20, 0.1},
	},
	"rabinovich": {
		{"rab-dt", "dt", &dynamics.RabDT, 0.001, 0.0001, 0.01, 0.0001},
		{"rab-alpha", "α", &dynamics.RabAlpha, 1.1, 0.01, 2, 0.01},
		{"rab-gamma", "γ", &dynamics.RabGamma, 0.87, 0.01, 1, 0.01},
	},
	"burkeshaw": {
		{"burke-dt", "dt", &dynamics.BurkeDT, 0.005, 0.001, 0.05, 0.001},
		{"burke-s", "S", &dynamics.BurkeS, 10.0, 1, 20, 0.1},
		{"burke-v", "V", &dynamics.BurkeV, 4.272, 1, 10, 0.001},
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
	// The Platonic solids: one knob each, the Conway operator applied to
	// the seed. Same variable on all five, because it is the same question
	// and the setting should survive changing which solid it is asked of.
	"tetrahedron":  {{"poly-op", "op", &polyOpF, 0, 0, 6, 1}},
	"cube":         {{"poly-op", "op", &polyOpF, 0, 0, 6, 1}},
	"octahedron":   {{"poly-op", "op", &polyOpF, 0, 0, 6, 1}},
	"dodecahedron": {{"poly-op", "op", &polyOpF, 0, 0, 6, 1}},
	"icosahedron":  {{"poly-op", "op", &polyOpF, 0, 0, 6, 1}},

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

// paramHelp says what a knob DOES, in one sentence, keyed by parameter id.
//
// The tooltip machinery names a control by where it sits — "Stereo ▸ smth ▸
// knob" — which answers "what am I hovering" and not "what is this for". On a
// panel whose labels are four characters because that is what fits under a
// dial, the second question is the one that goes unanswered: "smth" is
// unguessable, and so are algn, drv, τ and vg until someone reads the source.
//
// So a label that is an abbreviation spells the word out, and every entry says
// what turning it does rather than restating the name. Anything without an
// entry keeps the plain hierarchy tooltip, which is fine for a knob whose
// label is already a word.
var paramHelp = map[string]string{
	// ── the delay embedding, shared by three modes ──────────────────────
	"takens-tau": "tau — the delay between the coordinates, in samples at 48 kHz, " +
		"so one position is the same duration on any source. Too short and the " +
		"axes are nearly the same sample and the figure collapses onto the " +
		"diagonal; too long and it folds back on itself.",
	"takens-win": "window — how much recent audio is on screen, in milliseconds. " +
		"The length of the trace, not its shape.",
	"takens-gain": "gain — world units a full-scale sample maps to. The camera fit " +
		"tracks this, so the figure stays the same size on screen; it sets the " +
		"scale the geometry is built at, not how big it looks.",
	"takens-smooth": "smooth — Catmull-Rom upsampling, in drawn points per source " +
		"sample. A straight line between samples draws chords that are an " +
		"artifact of the drawing; this curves the beam through them instead. " +
		"Higher is smoother and costs vertices.",
	"takens-chan": "source — which channel of the live pair is reconstructed.",

	// ── stereo ──────────────────────────────────────────────────────────
	"stereo-axes": "axes — what the three coordinates are. The two 't' positions " +
		"put TIME on the third axis and draw a goniometer sweeping like a scope " +
		"trace; the two 'd' positions put a delayed copy there and draw a delay " +
		"embedding of the pair.",
	"stereo-tau": "tau — the delay used by the two 'd' axis positions. Inert on the " +
		"time positions, which have no delayed coordinate.",
	"stereo-win": "window — how much recent audio is on screen, in milliseconds.",
	"stereo-gain": "gain — the scale the geometry is built at. The camera fit tracks " +
		"it, so this does NOT change how big the figure looks; vg is the one " +
		"that does.",
	"stereo-align": "align — an inter-channel delay, signed, in samples at 48 kHz: " +
		"right is read this many later than left. A spaced pair of microphones or " +
		"a mis-clocked converter opens the figure into a rotating ellipse; dial " +
		"the offset out until it collapses back onto the diagonal and the knob " +
		"reads how far apart they were.",
	"stereo-width": "width — mid/side width on the DRAWN figure only: below 1 the " +
		"difference content shrinks toward mono, above 1 it spreads. Nothing is " +
		"written back to the audio, so the correlation meter goes on reading the " +
		"source as it is and this asks 'what would widening do'.",
	"stereo-vg": "vertical gain — scales the two signal axes against the frame, the " +
		"way a scope's vertical knob does. The camera fit ignores it, so a loud " +
		"passage can be turned down to fit or a quiet one driven off the top.",
	"stereo-trig": "trigger — what fixes the START of the window. OFF ends it at " +
		"the newest sample, so a steady tone is redrawn at a different phase each " +
		"frame and the figure slides. RISE and FALL start it where the signal " +
		"crosses the level, so successive frames begin at the same phase and a " +
		"periodic signal stands still. LOCK is the one for music: it matches the " +
		"SHAPE of the last frame instead of looking for an edge, so nothing has " +
		"to cross a level and what holds still is the whole waveform rather than " +
		"one point on it — which is what works on chords and speech, where an " +
		"edge trigger cannot. Nothing to lock to and it free-runs rather than " +
		"blanking, which is the AUTO behavior of a bench scope.",
	"stereo-tsrc": "trigger source — which signal the trigger watches. It need not " +
		"be what is drawn: locking to MID holds the whole figure, and locking to " +
		"one channel is how to hold a figure whose other channel is the busy one.",
	"stereo-tcpl": "trigger coupling — what reaches the trigger. DC passes " +
		"everything, LF reject high-passes so bass and offset stop dragging the " +
		"crossing around, HF reject low-passes so hiss and cymbals stop producing " +
		"crossings of their own. This is the \"which frequency do I trigger on\" " +
		"control, spelled the way a scope spells it.",
	"stereo-trun": "run mode — what happens when NOTHING triggers, which is the " +
		"whole question on music, where a lock comes and goes. AUTO draws anyway " +
		"so the display is never blank. NORMAL holds the last triggered frame, so " +
		"a figure that did hold stays up to be read instead of dissolving the " +
		"moment the signal changes. SINGLE catches the next trigger and freezes; " +
		"move this knob to re-arm.",
	"stereo-hold": "holdoff, milliseconds — the minimum quiet before an edge " +
		"counts. Zero takes whichever edge is newest, so a waveform with several " +
		"crossings per pattern locks to a different one each frame. Set near the " +
		"length of a bar and a loop stands still; near a period and single cycles " +
		"do. This is the control that locks onto a PATTERN rather than a cycle " +
		"inside one.",
	"stereo-grat": "graticule — the reference lines a goniometer is read " +
		"against. Which lines carry the meaning follows the AXES dial: on the " +
		"L/R positions the DIAGONALS do (x=y is in phase, x=-y is the content " +
		"that vanishes when summed to mono) and on mid/side the AXES do " +
		"(side=0 is mono, mid=0 is entirely out of phase). Scaled with gain and " +
		"vg, so the lines stay with the trace.",
	"stereo-tpos": "trigger position — where the trigger point sits in the " +
		"window, 0 at the left edge and 1 at the right. Past zero the window " +
		"holds audio from BEFORE the edge, which is how to see what led up to a " +
		"transient rather than only what followed it. A scope calls this the " +
		"horizontal position; it costs that much more older audio to keep.",
	"stereo-hyst": "noise reject — how far past the level the signal must go " +
		"before a crossing counts. Zero triggers on every dither across the level, " +
		"which on program material means the figure flickers between two phases a " +
		"sample apart. A few hundredths is usually enough.",
	"stereo-lvl": "level — where the trigger looks for its crossing, in units of " +
		"full scale. 0 is the zero crossing and is what to use unless the signal " +
		"has an offset or the interesting edge is part way up it.",
	"stereo-span": "span — timebase. Stretches the TIME axis only, so the trace can " +
		"sweep across a wide window instead of sitting in a small square. Inert " +
		"on the two 'd' positions, where all three axes are signal and stretching " +
		"one would be a distortion.",

	// ── polar ───────────────────────────────────────────────────────────
	"polar-map": "map — how sample magnitude becomes radius: tanh and soft squash " +
		"gently, dB is logarithmic, unit discards magnitude and keeps direction " +
		"only, drawing on the unit sphere.",
	"polar-drive": "drive — how hard the signal is pushed into the map before it " +
		"squashes. Low leaves quiet material near the center; high pushes " +
		"everything out toward the surface.",
	"polar-tau":  "tau — the delay between the three coordinates, in samples at 48 kHz.",
	"polar-win":  "window — how much recent audio is on screen, in milliseconds.",
	"polar-gain": "gain — the scale the geometry is built at; the camera fit tracks it.",
	"polar-chan": "source — which channel of the live pair is reconstructed.",
}

// helpFor returns the sentence for a parameter id, or "".
func helpFor(id string) string { return paramHelp[id] }
