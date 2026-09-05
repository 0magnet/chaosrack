package attractor

// The mode descriptions — the prose the info overlay shows and the README's
// model reference is generated from. Untagged, next to the mode registry in
// modes.go, for the same reason the Sprott equations live untagged in
// sprottdata.go: a description that only exists in a js/wasm-tagged file is
// invisible to the native tools, and cmd/uitool writes the README from this
// map. Modes whose description is composed from data (the Sprott catalog)
// register in the init below.

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
	"scopeclock": "Scope Clock — the time, drawn the way a scope draws anything: as ONE beam tour. " +
		"The face, the twelve ticks and the three hands are a single path walked in order, because an " +
		"X/Y display has one beam and no way to lift it — so the jumps between the end of one stroke and " +
		"the start of the next are drawn too, and show as the faint diagonals an unblanked scope really " +
		"shows. Scope Pong and the Fourier banner are built the same way; this is the same instrument " +
		"told the time.\n\n" +
		"The beam is spaced evenly along the whole path rather than given a fixed number of points per " +
		"stroke: the face is a full circle and a tick is a few hundredths of one, so per-stroke spacing " +
		"would put most of the beam on the ticks and leave the circle dotted. Even spacing is also what a " +
		"real scope does, since the beam moves at a rate rather than at a number of samples.\n\n" +
		"The hands are fractional throughout — the hour hand creeps with the minutes, the minute hand " +
		"with the seconds — because a clock whose hands jump looks stopped in between. TRAIL and Persist " +
		"apply as they do to any trace, so a long trail smears the second hand into a comet.",
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
	"turtle":        "Turtle Path — Reduce an integer sequence modulo m and the remainders repeat; the length of the repeat is the Pisano period. Read each term as an instruction — odd turns left and steps forward, even turns right and steps forward, zero does neither — and the walk draws a figure. In three dimensions the parity of the NEXT term decides whether the turn is a yaw or a pitch, which is not arbitrary: the pair (F_n, F_n+1) mod m is the state of the recurrence, and the Pisano period is the period of that pair. One pass decides the rest, without walking further: the figure either closes, drifts in a straight line, or screws away along an axis. The walk never finishes and is never restarted — the turtle is held mid-stride and extended at the Speed knob's rate, forever, and TRAIL is how much stays behind it (whole, long, short, comet) as the oldest scrolls off the far end. CAM is where it is watched from. Only follow cares where the head is; every other setting places the figure by its DRIFT, the one direction it is going overall, known exactly from one pass of the period rather than guessed at from the last few frames — and for a screw it centers on the axis the classification locates, so the figure turns on the spot instead of swinging around it. fit then only decides how big it is drawn, lock holds that size too, and auto fits a closed figure and locks one that drifts. TINT is what a color means — step, pass, visits, heading, turn, term, age — and set COLORS SRC to trl to see it, or leave it on X/Y/Z to read position instead (a flat DIM 2 figure has no Z to follow). MUL multiplies the Fibonacci sequence, CAP limits the terms read for sequences that may not repeat, CYCLE steps to the next modulus every so many seconds, and MOD 0 draws the sequence unreduced. PHYS gives the figure weight: it becomes a rigid body in the plane of the screen — three degrees of freedom, the figure's own shape as its mass — inside a solid box the size of the frame, where GRAV (either way up), FRIC, BOUNCE and SPIN say how it behaves. Press on the figure and you can pick it up and throw it; press beside it and you are turning the view as usual. The walk keeps extruding while it lies there, new points arriving at the head and old ones dropping off the tail, so the figure slides through its own body and marches. PHYS off hands the placing back to CAM.",
	"globe":         "Globe — A wireframe sphere showing lines of latitude and longitude, similar to the graticule on a geographic globe. Latitude lines are horizontal circles parallel to the equator, longitude lines are great circles passing through the poles.",
	"sphere":        "Sphere — A perfectly round three-dimensional surface where every point is equidistant from the center. Generated as a UV sphere with configurable latitude and longitude subdivisions.",
	"torus":         "Torus — A doughnut-shaped surface of revolution generated by revolving a circle (radius r) around an axis at distance R from the center of the circle.",
	"magnetosphere": "Magnetosphere — A visualization of magnetic field lines surrounding a dipole, similar to Earth's magnetosphere that shields the planet from solar wind.",
	"stlfile":       "STL File — Load a stereolithograph (.stl, binary or ASCII) from disk with the Loader module's Load button and it renders as a rotating wireframe. Very large files are decimated to fit the 16-bit index pipeline.",
	"lu": "Lü Attractor — Discovered by Jinhu Lü and Guanrong Chen (2002), " +
		"the third member of the Lorenz–Chen–Lü family; it forms a bridge between the Lorenz and " +
		"Chen systems and produces a two-scroll butterfly.\n\ndx/dt = a(y − x)\ndy/dt = cy − xz\ndz/dt = xy − bz",
	"newtonleipnik": "Newton–Leipnik Attractor — Arises from a rigid-body " +
		"rotation model with a linear feedback torque; it has two coexisting scroll-shaped attractors.\n\n" +
		"dx/dt = −ax + y + 10yz\ndy/dt = −x − 0.4y + 5xz\ndz/dt = bz − 5xy",
	"sprotta": "Sprott A (Nosé–Hoover oscillator) — the first of J. C. Sprott's " +
		"1994 systems and the only conservative one: rather than settling onto a strange attractor it " +
		"fills a chaotic sea.\n\ndx/dt = y\ndy/dt = −x + yz\ndz/dt = 1 − y²",
	"hyperrossler": "Rössler Hyperchaos (1979) — Otto Rössler's" +
		" 4-dimensional extension of his attractor, with two positive Lyapunov" +
		" exponents (hyperchaos). A hidden fourth coordinate w feeds back into y;" +
		" the plot is the (x,y,z) projection.\n\n" +
		"dx/dt = −y − z\ndy/dt = x + ay + w\ndz/dt = b + xz\ndw/dt = −cz + dw",
	"takens": "Takens Delay Embedding — attractor reconstruction " +
		"from a single signal (F. Takens, \"Detecting strange attractors in turbulence\", 1981). " +
		"Each trail point is the delay vector (s(t), s(t−τ), s(t−2τ)) of the live audio: a pure " +
		"tone draws a closed loop, music and speech trace the geometry of whatever produced them. " +
		"τ is the embedding delay in samples, and MEAS measures it rather than guessing: the " +
		"first minimum of the signal's average mutual information (Fraser & Swinney 1986), " +
		"reported beside the false-nearest-neighbor embedding dimension m (Kennel et al. 1992). " +
		"It runs once, on the button — nothing here re-tunes itself per frame, because a knob " +
		"that moves with the music makes the figure move with it. An m above 3 means the trail " +
		"you are looking at is a projection of a higher-dimensional reconstruction. " +
		"WIN is how much time the figure spans, in milliseconds — short is live and legible, " +
		"long draws a denser tangle that turns over more slowly; GAIN sets how large a full-scale " +
		"sample draws. The scale is fixed — nothing auto-ranges, so quiet passages draw small and " +
		"loud ones large, and the view never moves under you; the camera is fitted once to what " +
		"full scale can reach, so peaks stay on screen. The trace is spline-smoothed between " +
		"samples the way a scope's beam is. " +
		"Audio comes from the active source — websocket stream, microphone, or the signal generators.",
	"stereo": "Stereo Embedding — the Takens trail built from the two channels instead " +
		"of one channel's past. Takens' theorem manufactures the missing axes out of a signal's own " +
		"history because there is only one signal; a stereo source has already measured two, so this " +
		"mode plots them against each other and what you see is the real relationship between the " +
		"channels — phase, polarity, correlation, width — rather than a reconstruction. Looked at " +
		"head-on it is the goniometer (vectorscope) of a mastering desk, which is the XY Scope's " +
		"figure; the third axis is what a scope with two deflection plates cannot give you. " +
		"AXES picks the assignment. L,R,L(t−τ) is the goniometer with a delay coordinate for depth: " +
		"a tone that draws one ellipse edge-on unrolls into a helix, and τ still means what it means " +
		"in the Takens mode. L,R,time sweeps the figure along a ribbon so successive cycles stack " +
		"instead of overwriting — the only way to see a slow phase drift, which on a flat display " +
		"just wobbles. The mid/side positions rotate the basis 45°: M=(L+R)/2 and S=(L−R)/2, so " +
		"center content lies along one axis and difference content along the other and width is an " +
		"extent rather than a tilt. τ is inert on the two time positions. " +
		"CORR is the correlation meter: +1.00 means the channels are identical and the figure is a " +
		"diagonal line, 0 means they are unrelated and it is a round cloud, −1.00 means one is the " +
		"other inverted (and the difference vanishes if the mix is summed to mono). " +
		"A mono source reads \"mono\" and draws the diagonal, which is the correct picture of a " +
		"signal with no stereo information in it — nothing here fakes a second channel out of a " +
		"delayed copy of the first, because that delayed copy is exactly what this mode exists to " +
		"stop pretending is a channel. The mid/side positions are the ones worth turning to then: " +
		"S is zero and what is left is an honest two-coordinate delay embedding. " +
		"WIN is how much time the figure spans, in milliseconds — a phase display is read over a " +
		"few tens of them, past a couple of hundred it is a filled blob; GAIN sets how large a " +
		"full-scale sample draws, and as in the Takens mode the scale is fixed and nothing " +
		"auto-ranges. " +
		"Audio comes from the active source — microphone or the signal generators for real stereo; " +
		"the websocket and WebTransport feeds carry one channel, so they read \"mono\".",
	"fvf": "FVF — Harmonic Wobbulator. A software analog of the " +
		"Frequency→Voltage→Frequency converter with balanced modulator designed at bunkerofdoom.com " +
		"(hardware built 1984). The live audio's " +
		"pitch is tracked, scaled/offset into a new carrier frequency, and ring- or AM-modulated back by " +
		"the original signal — the metallic, glitchy 'very strange' timbre. Shown here as the processed " +
		"spectrogram.",
	"graphicartist": "Graphic Artist — a digital re-creation of Mitchell Waite's " +
		"\"Oscilloscope Graphic Artist\" (Popular Electronics, November 1975), which drove a " +
		"scope's X/Y inputs with harmonically related signals to draw 3-D-looking wireframes. " +
		"Four relaxation oscillators A/B/C/D, each square or triangle: A is the fixed master and " +
		"B/C/D lock to it at integer harmonics — the sync the original circuit was meant to " +
		"enforce, and which a wiring error on real builds defeated, leaving them drawing an " +
		"unstable blur. Carrier C is split ±45° and the B envelope modulates each phase; the two " +
		"perpendicular components are what create the volume illusion on a flat scope, and here " +
		"that component is lifted onto a real Z so the figure genuinely has depth.\n\n" +
		"VERT = levelA·A + levelB·(B · C₊₄₅)\nHORIZ = levelD·D + levelB·(B · C₋₄₅)",
	"xy": "X/Y Scope — the classic two-channel oscilloscope figure, drawing the live " +
		"audio's (left, right) sample pairs as a line strip. Correlated channels lie on a " +
		"diagonal, anti-correlated on the other, and a phase difference opens the diagonal into " +
		"an ellipse — which is how the display doubles as a stereo phase meter. A mono source is " +
		"plotted against a lagged copy of itself, since a raw mono signal would otherwise be a " +
		"featureless diagonal.",
	"spectrogram": "Spectrogram — a scrolling short-time Fourier transform of the live " +
		"audio: frequency up the plane, time scrolling right to left, magnitude as color. It is " +
		"a display inspired by Vanya Sergeev's audioprism, written in Go " +
		"(github.com/0magnet/audioprism-go) and drawn here as a texture on a plane in the same " +
		"3-D pipeline as every other model, so it rotates and zooms like one — and the same " +
		"texture can be painted onto other geometry with the skin switch. The Spectrogram module " +
		"exposes the whole chain: transform size, overlap, window function, magnitude scale and " +
		"limits, and color scheme.",
	"custom": "Custom Equations — type your own system. The Equations module takes dx/dt, " +
		"dy/dt and dz/dt (plus dw/dt for a 4-D system) as text and integrates them with forward " +
		"Euler, the same way the classic attractors are stepped. The iterate switch reads the " +
		"same expressions as a discrete MAP instead — x = f(x,y,z) rather than x += dt·f(x,y,z) " +
		"— which has no dt and no path between iterates, so it draws as a cloud of points the way " +
		"Henon and Ikeda do. There is no eval: expressions are tokenized, " +
		"converted to RPN and interpreted over a small value stack, so nothing but arithmetic " +
		"can run. Grammar is + − × ÷ ^ with grouping and implicit multiplication (2x, 3(x+1)), " +
		"the functions sin cos tan asin acos atan exp log ln sqrt abs sign sinh cosh tanh floor, " +
		"the constants pi e tau, and the variables x y z w t. Any other name becomes a free " +
		"parameter and is auto-exposed as its own knob.",
	"recurrence": "Recurrence Plot — the picture of when a signal returns to " +
		"where it has already been (J.-P. Eckmann, S. O. Kamphorst and D. Ruelle, " +
		"\"Recurrence Plots of Dynamical Systems\", 1987). Time runs along both axes, and the " +
		"cell (i, j) is lit when the two samples are within ε of each other, so the main " +
		"diagonal is always lit and everything else says how the signal repeats. A steady tone " +
		"draws unbroken diagonals spaced by its period; a chaotic or noisy source breaks them " +
		"into short segments; a held sound is a solid block, and the instant the source changes " +
		"is an edge running across the square. SRC picks what is being plotted: audio is the raw " +
		"samples, embed is the Takens delay vector (s, s−τ, … s−(m−1)τ) of that same audio — the " +
		"reconstructed phase space a recurrence plot is properly defined on, which removes the " +
		"anti-diagonals a scalar signal shows because sin t equals sin(T/2 − t) — and traj is the " +
		"most recent attractor's own trajectory, the way the Bifurcation explorer picks its " +
		"system. τ is the Takens mode's own τ knob, so its MEAS button measures the delay for " +
		"this plot too, and m is the embedding dimension. WIN is how much history the square " +
		"covers: milliseconds for the audio sources, tenths of that in the system's own time " +
		"units for a trajectory. ε is the threshold as a fraction of the source's scale — full " +
		"scale for audio, the attractor's own diameter for a trajectory, which is what makes one " +
		"default readable across systems that differ in width by fifty times. It is never taken " +
		"from the current level, deliberately: a threshold that follows the music makes the " +
		"plot's density pulse with it instead of describing it. RQA reads the picture back as " +
		"three numbers — RR, the percentage lit and the number to turn ε by; DET, the share of " +
		"that lying on diagonals, which is what separates a system from noise; and LAM, the " +
		"share on verticals, which is states it sat in rather than passed through. Drawn as a " +
		"texture on a square plane in the same 3-D pipeline as every other model.",
	"bifurcation": "Bifurcation Explorer — the fig-tree diagram, computed " +
		"live. One parameter of the most recent flow mode sweeps its whole knob range across the x " +
		"axis; each column integrates the system fresh at that value and plots the local maxima of " +
		"z. Thin branches are periodic orbits, fan-outs are period-doubling cascades, filled bands " +
		"are chaos — the route between them is the route to chaos. Pick the swept parameter in the " +
		"Parameters module; visit an attractor and tune it to change the source system.",
	"poincare": "Poincaré Section — the continuous flow read as a discrete " +
		"point set. The most recent flow mode is integrated privately and sampled only where it " +
		"pierces a plane, going one way through it; what is left is the cross-section of the " +
		"attractor, and the sheets that are invisible in the tangle are the whole picture here. " +
		"AXIS and POS place the plane (POS is a fraction of the attractor's own reach along that " +
		"axis, so 0 is through the middle whatever the system's size); DIR chooses which way " +
		"through it counts. One way is the default, and it is not a preference: a bounded flow " +
		"that goes up through a plane has to come back down through it, so keeping both " +
		"superimposes two different sections and the return map stops being a function. VIEW " +
		"picks the picture — PLANE draws the crossings where they physically are, FLAT lays the " +
		"section out face on in the plane's own coordinates, and MAP is the FIRST-RETURN MAP: " +
		"each crossing plotted against the next one. That last is where the route to chaos is " +
		"legible — a periodic orbit is a handful of dots, a period-doubling is that set " +
		"doubling, and a chaotic attractor is a single-humped curve, which is the logistic map's " +
		"parabola surfacing inside a differential equation. The dotted 45° line is y = x, where " +
		"the map's fixed points are. The crossing point is INTERPOLATED between the two samples " +
		"that straddle the plane rather than snapped to the nearer of them — snapping smears the " +
		"section by up to half a step of arc, which on these attractors is the same size as the " +
		"gap between the sheets it is supposed to show. The same section is available as an " +
		"overlay on the live attractor: Trace > Sect.",
}

func init() {
	// Composed from the catalog data rather than written out nineteen times.
	for _, c := range sprottCases {
		attractorDescriptions[c.key] = c.name +
			" — one of J. C. Sprott's simple chaotic flows (1994), realized as an" +
			" analog circuit at glensstuff.com. Found by systematic search for the" +
			" algebraically simplest systems that still produce chaos.\n\n" + c.eq
	}
}

// Discrete maps. Each is stated as its recurrence rather than a derivative,
// because that is the whole difference: there is no dt and no trajectory
// between iterates, only where the point lands next.
func init() {
	for k, v := range map[string]string{
		"henon": "Hénon map (M. Hénon, \"A two-dimensional mapping with a strange attractor\", 1976) —" +
			" the example that showed a strange attractor needs neither a flow nor three dimensions," +
			" only a stretch and a fold. Zoom in anywhere on a filament and it resolves into more" +
			" filaments: locally a line crossed with a Cantor set. Its Jacobian determinant is −b, so" +
			" area contracts by exactly b every iterate no matter where you are.\n\n" +
			"x' = 1 − a·x² + y\ny' = b·x",
		"ikeda": "Ikeda map (K. Ikeda, 1979) — light circulating in a nonlinear optical cavity, where" +
			" each pass rotates the complex field by an amount that depends on its own intensity." +
			" That intensity-dependent rotation is the whole mechanism: the more energetic part of" +
			" the beam turns further, and the attractor's hook is the result.\n\n" +
			"t = 0.4 − 6/(1 + x² + y²)\nx' = 1 + u·(x·cos t − y·sin t)\ny' = u·(x·sin t + y·cos t)",
		"clifford": "Clifford attractor (Clifford Pickover) — a trigonometric map with no physical" +
			" derivation, kept because of what it draws. Every parameter change reshapes it entirely," +
			" which makes the four knobs worth turning slowly.\n\n" +
			"x' = sin(a·y) + c·cos(a·x)\ny' = sin(b·x) + d·cos(b·y)",
		"dejong": "Peter de Jong attractor — the same idea as Clifford's with the terms paired" +
			" differently, and a different family of figures for it. Four knobs, an enormous space of" +
			" shapes, and no way to predict which you will get.\n\n" +
			"x' = sin(a·y) − cos(b·x)\ny' = sin(c·x) − cos(d·y)",
		"mira": "Gumowski-Mira map (I. Gumowski and C. Mira, CERN, 1980) — written to study how a" +
			" particle beam drifts turn after turn around an accelerator ring, and it produces" +
			" organic, almost biological figures that look nothing like their origin. The shared" +
			" nonlinearity g is applied twice per iterate, to x and then to the NEW x." +
			" Unlike the others here it is QUASI-PERIODIC at these settings rather than chaotic:" +
			" the curves it draws are invariant curves the orbit winds around forever, not a" +
			" fractal dust. Two orbits started a hair apart drift apart slowly instead of" +
			" separating exponentially, which is why the figure looks woven rather than sprayed.\n\n" +
			"g(v) = μ·v + 2(1−μ)·v²/(1+v²)\nx' = y + a(1 − b·y²)·y + g(x)\ny' = −x + g(x')",
		"tinkerbell": "Tinkerbell map — a quadratic map of the plane whose orbit sweeps a curved," +
			" wing-like region. Bounded only for a narrow band of parameters; push the knobs and it" +
			" escapes to infinity, which the renderer catches and reseeds rather than drawing NaNs.\n\n" +
			"x' = x² − y² + a·x + b·y\ny' = 2·x·y + c·x + d·y",
		"standardmap": "Chirikov standard map (B. Chirikov, 1979) — the canonical" +
			" AREA-PRESERVING map, and the one system here with no attractor at all. A kicked rotor:" +
			" each iterate kicks the momentum by K·sin θ and lets the angle advance. Because area is" +
			" preserved nothing contracts onto anything, so it is drawn as an ensemble of orbits" +
			" rather than one — a single orbit would trace its own invariant curve and tell you" +
			" nothing about the rest. Turn K up and watch the intact curves break into a chaotic sea" +
			" with islands of stability stranded in it; near K ≈ 0.9716 the last curve spanning the" +
			" phase space goes, and orbits can wander in momentum without bound.\n\n" +
			"p' = p + K·sin θ\nθ' = θ + p'   (both mod 2π)",
	} {
		attractorDescriptions[k] = v
	}
}

// The terminal is a mode whose content is another program's rendering, which
// is unlike everything else here and worth saying plainly.
func init() {
	attractorDescriptions["terminal"] = "Terminal — a live terminal, drawn as a model. " +
		"It is the same texture-on-a-plane path the spectrogram and the recurrence plot use, so it " +
		"rotates, zooms and takes a gradient like any other model; what is on the texture is " +
		"[xterm-go](https://github.com/0magnet/xterm-go), a Go port of xterm.js, rendering a real " +
		"terminal grid with WebGL2. That last part is what makes this possible rather than merely " +
		"desirable: a terminal drawn as DOM could not be sampled into a texture at all, and " +
		"rasterizing DOM every frame is not a thing worth doing. xterm-go renders into a canvas, and " +
		"a canvas is a texture source. " +
		"Behind it runs [websh](https://github.com/0magnet/websh), a Bash interpreter compiled to " +
		"wasm over an in-memory filesystem, so this is a session you can work in and not a picture " +
		"of one: pipes, globs, redirection, `for` loops, all of it rotating with the model. " +
		"**Double-click** the canvas to type into it and **Esc** to give the keyboard back — a " +
		"double click because a single one is how you rotate the model, and the two must not be the " +
		"same gesture. Focusing it silences the app's own key bindings automatically, since Pong, " +
		"the Keys module and the hovered-knob arrows all already stand aside for a focused textarea, " +
		"which is what a terminal captures keys on. " +
		"It can also be the BACKDROP behind another model, the way the spectrogram can."

	attractorDescriptions["termanim"] = "Terminal Animation — the same terminal-on-a-plane, with a " +
		"drawing program on it instead of a shell. The catalog is " +
		"[tuiwasm](https://github.com/0magnet/tuiwasm): twenty-one animations — fire, plasma, a " +
		"matrix rain, an aquarium, a bonsai growing branch by branch, Langton's ant, falling sand — " +
		"plus charts, tables and styled text. Pick one from the **animation** selector on this " +
		"model's panel. " +
		"The animations draw at half-block resolution: every cell is an upper or lower block glyph " +
		"carrying its own foreground and background, which is two independently colored pixels per " +
		"cell and roughly square ones, since a terminal cell is about twice as tall as it is wide. " +
		"They run at the frame rate — the cells are written straight into xterm-go's buffer rather " +
		"than encoded as escape sequences and parsed back, which is the difference between sixty " +
		"frames a second and a wedged tab. " +
		"Nothing is wired to the keyboard here, unlike the Terminal model: these draw, they do not " +
		"read, and the animations quit on a keystroke. " +
		"Like the terminal it can also be the BACKDROP behind another model."

	// The other kind of terminal: not a shell compiled into the page, but the
	// one on the machine serving it. Same texture trick, different shell.
	attractorDescriptions["hostterm"] = "Host Shell — a real shell on the machine serving this page, drawn as a model. " +
		"The Terminal model beside it is [websh](https://github.com/0magnet/websh), a Bash interpreter " +
		"compiled into the wasm over a filesystem that exists only in the browser; this one is a pty on " +
		"the host, reached through the same agent the desk's host pane uses. " +
		"It needs the server to have been started with **--shell**, which also forces a loopback bind — " +
		"the server refuses that flag on a listener the network can reach rather than warning about it. " +
		"Without the flag the terminal still opens and says so, because an empty rectangle with no " +
		"explanation is the worse answer. " +
		"**Double-click** the canvas to type into it and **Esc** to give the keyboard back, exactly as " +
		"for the Terminal model. It reached the screen before this as a window inside the Desk; as a " +
		"model it is the shell without the window manager around it."

}

// The desk is the second mode whose content is another program's rendering,
// and the only one whose content is a whole window manager.
func init() {
	attractorDescriptions["desk"] = "Desk — a window manager, drawn as a model. " +
		"The same texture-on-a-plane path the spectrogram, the recurrence plot and the Terminal use, " +
		"with a whole [desk](https://github.com/0magnet/desk) on it: winbox windows, a " +
		"[websh](https://github.com/0magnet/websh) shell in each, a file manager, all of them running " +
		"while you rotate them. " +
		"It needed something from desk to be possible at all. A window is more than its pane — its " +
		"title, buttons and border are DOM, and DOM cannot be sampled into a texture — so texturing " +
		"the panes alone would give a desk of frameless rectangles. desk's WebGL compositor can now " +
		"REDRAW the frames instead: each title bar is rasterized with Canvas2D, text and buttons and " +
		"all, into the same canvas it draws the panes into, and that canvas is a complete picture of " +
		"the desk. " +
		"Nothing types into it while it is a model, and that is the arrangement rather than an " +
		"omission for the MOUSE: a click on a rotated quad would have to be cast through it to a " +
		"texture coordinate and synthesized back into a DOM event at a place nothing is. " +
		"What you get instead are two gestures. **Ctrl-drag** reaches the desk, so ctrl-dragging a " +
		"title bar moves a window while an ordinary drag still turns the model; the **Pass-thru** " +
		"switch in the Desk module swaps the two if you would rather drag windows directly and hold " +
		"ctrl to turn. And the KEYBOARD does reach it: **double-click** the canvas to type into the " +
		"focused window and **Esc** to give the keyboard back, the same pair the Terminal and Host " +
		"Shell models use. " +
		"Aiming is the honest limitation \u2014 you are pointing at a projection, so a title bar is not " +
		"where the pointer says it is. The **Desk** switch (Console \u2192 Window) puts the same windows " +
		"on the page as ordinary DOM, which is the one to use for arranging them; rearrange there, " +
		"then come back here to look at it. Flatten to work, rotate to admire. " +
		"It can also be the BACKDROP behind another model, the way the spectrogram and the terminal can."
}
