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
	"globe": {
		{"globe-lat", "lat", &globeLatF, 18, 4, 90, 1},
		{"globe-lon", "lon", &globeLonF, 36, 4, 180, 1},
	},
	"sphere": {
		{"sphere-r", "radius", &sphereRadius, 1.0, 0.1, 5, 0.1},
		{"sphere-stacks", "lat", &sphereStacksF, 30, 4, 100, 1},
		{"sphere-slices", "lon", &sphereSlicesF, 30, 4, 100, 1},
	},
	"torus": {
		{"torus-R", "R", &torusR, 1.5, 0.1, 5, 0.1},
		{"torus-r", "r", &torusr, 0.5, 0.1, 3, 0.1},
		{"torus-stacks", "stacks", &torusStacksF, 30, 4, 100, 1},
		{"torus-slices", "slices", &torusSlicesF, 30, 4, 100, 1},
	},
}

var attractorDescriptions = map[string]string{
	"lorenz": "Lorenz Attractor — Discovered by Edward Lorenz in 1963 while modeling atmospheric convection. " +
		"The butterfly-shaped trajectory arises from a simplified system of three coupled differential equations. " +
		"It was one of the first systems shown to exhibit deterministic chaos, where tiny differences in initial conditions lead to vastly different outcomes.\n\n" +
		"dx/dt = σ(y − x)\ndy/dt = x(ρ − z) − y\ndz/dt = xy − βz",
	"rossler": "Rössler Attractor — Proposed by Otto Rössler in 1976 as a simpler system that produces chaotic behavior. " +
		"Unlike the Lorenz system's two-lobed shape, the Rössler attractor has a single folded-band structure with an outward spiral that occasionally makes a large excursion in the z-direction.\n\n" +
		"dx/dt = −(y + z)\ndy/dt = x + ay\ndz/dt = b + z(x − c)",
	"chua": "Chua's Circuit (Double Scroll Attractor) — Invented by Leon Chua in 1983, this is the first electronic circuit proven to exhibit chaos. " +
		"The system features a piecewise-linear nonlinearity (the Chua diode) that creates the characteristic double-scroll pattern. " +
		"It is also the basis for multi-scroll attractor generalizations.\n\n" +
		"dx/dt = α(y − x − h(x))\ndy/dt = x − y + z\ndz/dt = −βy\nh(x) = m₁x + ½(m₀ − m₁)(|x+1| − |x−1|)",
	"aizawa": "Aizawa Attractor — A chaotic system that produces a toroidal structure with a tendril extending from the center. " +
		"The attractor has a visually striking shape that resembles a sphere with a tail, exhibiting both rotational symmetry and chaotic wandering.\n\n" +
		"dx/dt = (z − b)x − dy\ndy/dt = dx + (z − b)y\ndz/dt = c + az − z³/3 − (x² + y²)(1 + ez) + fzx³",
	"sprott": "Sprott Attractor — One of many simple chaotic systems cataloged by Julien Clinton Sprott. " +
		"These systems were discovered through systematic computer searches for chaotic flows with minimal terms, demonstrating that chaos can arise from remarkably simple equations.\n\n" +
		"dx/dt = y + Axy + xz\ndy/dt = 1 − Bx² + yz\ndz/dt = x − x² − y²",
	"lissajou": "Lissajous Curve — Named after Jules Antoine Lissajous (1822–1880), these are parametric curves formed by combining sinusoidal motions along each axis. " +
		"Not a chaotic system — the curves are periodic and their shape depends on the frequency ratios and phase relationships between the three oscillations.\n\n" +
		"x(t) = sin(at)\ny(t) = sin(bt)\nz(t) = sin(ct)",
	"scopetext": "Fourier Text — An homage to the glensstuff.com Fourier Synthesis Character Generator, which built alphanumerics on a scope from summed harmonics. " +
		"The banner's whole beam tour (strokes and retrace jumps alike) is one complex periodic signal x(t)+i·y(t); what's drawn is its reconstruction from only the first N harmonics — real harmonic synthesis, not a blur. " +
		"One harmonic is an ellipse, so at low N the letters melt into loops; raise the harm knob and overtones sharpen them into legibility. " +
		"Type the banner in the Console's TEXT field; Model Out (CAM) plays the actual harmonic stack.",
	"sprottmorph": "Sprott Morph — The faithful version of the glensstuff.com Self-Programming Analog Computer, which stepped itself through the Sprott catalog by re-patching. " +
		"Every Sprott A–S system is a quadratic 3-D flow — a point in the same 30-dimensional coefficient space Sprott's 1994 computer search ran in — so the machine morphs by sliding linearly between coefficient vectors while ONE trajectory keeps integrating: watch D melt into E. " +
		"The sys knob parks the patch anywhere in the cycle (fractional = between systems, often chaotic in its own right); the rate knob makes it step itself, in systems per minute; the Console's PATCH readout shows the current wiring. " +
		"Coefficients are extracted numerically from the catalog's own equations, and a divergence guard reseeds the trajectory when a blend flings it away.",
	"bounceball": "Bouncing Ball — The classic analog-computer demo (Telefunken shipped it to sell integrators): two integrator chains under constant gravity, " +
		"a comparator that flips the vertical velocity at the floor with a restitution loss, and wall reflections for the drift. " +
		"The scope draws the familiar train of shrinking parabolic arcs; when the bounces decay away the machine re-kicks the ball, as the unattended trade-show loop did. " +
		"Floor hits blip at a pitch set by impact speed (once audio is unlocked by any interaction). Knobs: gravity, restitution, drift. Try Persist to paint the full arc family.\n\n" +
		"y'' = −g, bounce: v ← −e·v at the floor",
	"pong": "Scope Pong — An homage to the glensstuff.com analog Oscilloscope Pong: the whole game is one multiplexed beam tour driving the X/Y inputs, " +
		"so the court, net, score ticks, paddles and ball share a single stroke — the faint diagonal beams are the retrace an unblanked scope really shows.\n\n" +
		"W / S move the left paddle, ↑ / ↓ the right; a side left alone for ~10 s returns to the machine player (it boots as a self-playing demo). " +
		"First to 9 resets the match. Knobs: ball speed, paddle size, machine skill.",
	"thomas": "Thomas' Cyclically Symmetric Attractor — Introduced by René Thomas, this system has the elegant property of cyclic symmetry: each variable is damped and driven by the sine of the next variable in the cycle. " +
		"The parameter b controls dissipation; as b decreases the system transitions from stable points through limit cycles to chaos.\n\n" +
		"dx/dt = −bx + sin(y)\ndy/dt = −by + sin(z)\ndz/dt = −bz + sin(x)",
	"halvorsen": "Halvorsen Attractor — A chaotic system with three-fold rotational symmetry, producing a distinctive pinwheel-like shape. " +
		"The attractor consists of three intertwined lobes that spiral around each other, creating a visually complex but structurally symmetric trajectory.\n\n" +
		"dx/dt = −ax − 4y − 4z − y²\ndy/dt = −ay − 4z − 4x − z²\ndz/dt = −az − 4x − 4y − x²",
	"chen": "Chen Attractor — Discovered by Guanrong Chen in 1999, this system was found as a dual of the Lorenz system in a specific mathematical sense. " +
		"It exhibits chaotic behavior with a distinctive two-scroll structure that differs from both the Lorenz and Rössler attractors.\n\n" +
		"dx/dt = a(y − x)\ndy/dt = (c − a)x − xz + cy\ndz/dt = xy − bz",
	"dadras": "Dadras Attractor — A three-dimensional autonomous chaotic system with five parameters, introduced by Sara Dadras and Hamid Reza Momeni. " +
		"The system exhibits rich dynamical behavior including period-doubling routes to chaos.\n\n" +
		"dx/dt = y − px + qyz\ndy/dt = ry − xz + z\ndz/dt = sxy − ez",
	"rabinovich": "Rabinovich-Fabrikant Attractor — Derived by Mikhail Rabinovich and Anatoly Fabrikant from physical equations modeling the stochasticity of three interacting waves. " +
		"The system is known for its complex topology and extreme sensitivity to parameters, producing intricate folded structures.\n\n" +
		"dx/dt = y(z − 1 + x²) + γx\ndy/dt = x(3z + 1 − x²) + γy\ndz/dt = −2z(α + xy)",
	"burkeshaw": "Burke-Shaw Attractor — Introduced by Bill Burke and Robert Shaw, this system exhibits chaotic behavior with a distinctive two-winged structure. " +
		"It arises from the study of nonlinear dynamics and produces complex trajectories confined to a compact region of phase space.\n\n" +
		"dx/dt = −S(x + y)\ndy/dt = −y − Sxz\ndz/dt = Sxy + V",
	"tetrahedron":   "Tetrahedron — The simplest Platonic solid, with 4 triangular faces, 6 edges, and 4 vertices. It is its own dual.",
	"cube":          "Cube (Hexahedron) — A Platonic solid with 6 square faces, 12 edges, and 8 vertices. Its dual is the octahedron.",
	"octahedron":    "Octahedron — A Platonic solid with 8 triangular faces, 12 edges, and 6 vertices. Its dual is the cube.",
	"dodecahedron":  "Dodecahedron — A Platonic solid with 12 pentagonal faces, 30 edges, and 20 vertices. Its dual is the icosahedron.",
	"icosahedron":   "Icosahedron — A Platonic solid with 20 triangular faces, 30 edges, and 12 vertices. Its dual is the dodecahedron.",
	"nestedcube":    "Nested Cube — A cube within a cube, connected at the vertices, illustrating the relationship between inner and outer geometric structures.",
	"globe":         "Globe — A wireframe sphere showing lines of latitude and longitude, similar to the graticule on a geographic globe. Latitude lines are horizontal circles parallel to the equator, longitude lines are great circles passing through the poles.",
	"sphere":        "Sphere — A perfectly round three-dimensional surface where every point is equidistant from the center. Generated as a UV sphere with configurable latitude and longitude subdivisions.",
	"torus":         "Torus — A doughnut-shaped surface of revolution generated by revolving a circle (radius r) around an axis at distance R from the center of the circle.",
	"magnetosphere": "Magnetosphere — A visualization of magnetic field lines surrounding a dipole, similar to Earth's magnetosphere that shields the planet from solar wind.",
	"stlfile":       "STL File — Load a stereolithograph (.stl, binary or ASCII) from disk with the Loader module's Load button and it renders as a rotating wireframe. Very large files are decimated to fit the 16-bit index pipeline.",
}
