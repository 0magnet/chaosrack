//go:build js && wasm

package attractor

import (
	"strconv"

	"github.com/0magnet/chaosrack/pkg/analysis"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/glctx"
)

// The Poincaré section, as a thing on screen. The arithmetic is next door in
// poincare.go, untagged and tested; this file integrates a trajectory, feeds
// consecutive states to it, and draws what comes back.
//
// A section turns a continuous 3-D flow into a discrete 2-D point set by
// keeping only the states where the trajectory pierces a plane. That is the
// oldest trick in the subject and still the one that shows the most: the
// sheets a strange attractor is made of are invisible in the tangle and
// obvious in the section, and the ROUTE to chaos — period 1, then 2, then 4,
// then a smear — is a story about how many points the section has, which is
// unreadable in the flow and unmissable here.
//
// TWO PLACES SHOW IT, and they answer different questions.
//
//   - Trace > Sect, on any flow mode: the crossings drawn in gold WHERE THEY
//     PHYSICALLY ARE, on top of the running attractor. This answers "what is
//     the section OF" — you watch the trail thread through a scatter that is
//     its own record. It is an overlay, so it never replaces the flow.
//
//   - The Poincaré model, in Analysis beside the bifurcation explorer: the
//     section as its own picture, face on, plus the RETURN MAP. This answers
//     "what does the section SAY", which needs the flow out of the way.
//
// Both read the same knobs and build the same plane, so the section you tuned
// against the attractor is the section the model draws. The model's source
// system is lastFlowMode, the same "most recent flow mode you visited" the
// bifurcation explorer sweeps — visit an attractor, tune it, come back.

// ── The plane, and where the knobs put it ────────────────────────────────

// The section plane is axis-aligned. A fully free normal is two more knobs for
// a gain that is mostly imaginary: the interesting sections of the published
// systems are through a coordinate plane (Lorenz through z, Rössler through x
// or y), a tilted plane's section is a sheared version of the same picture,
// and an arbitrary normal makes the 2-D axes unnameable — "the section's first
// in-plane coordinate" is not something to put on a panel. poincare.go carries
// the general case anyway, because the general case is what the basis
// construction has to be correct for; this is the part of it the panel offers.
const (
	sectAxisX = 0 //nolint:unused // named for the axis knob's positions; sectNormal indexes them
	sectAxisY = 1 //nolint:unused // named for the axis knob's positions; sectNormal indexes them
	sectAxisZ = 2
)

// The view the Poincaré MODEL draws. The overlay ignores this — see
// sectViewParams below for why the knob is not offered there.
const (
	sectViewPlane = 0 // crossings in place, in 3-D, where they physically are
	sectViewFlat  = 1 // the section face on, in the plane's own 2-D basis
	sectViewMap   = 2 // the first-return map: crossing n+1 against crossing n
)

// poincareSection is the Poincaré section overlay: the plane, the crossings
// found, and its knobs.
type poincareSection struct {
	// Default z, because that is the plane the section was hardcoded to before
	// it had a knob, and because it is the classic section of the Lorenz
	// system that opens the app's Attractors category.
	axisF float32

	// Where the plane sits along that axis, as a fraction of the attractor's
	// own reach: 0 is through the middle, ±1 is at the extremes.
	//
	// A fraction rather than a coordinate, and this is borrowed deliberately
	// from splitFrac in split_js.go, which says the same thing about the depth
	// partition: a knob calibrated in world units would need re-learning for
	// every system, since a Lorenz attractor and a Rössler attractor are
	// different sizes. In fractions, the middle is always the middle.
	//
	// What is NOT borrowed is view.fitExtent, which splitFrac measures against.
	// That is one scalar for all three axes (max|coordinate| over the whole
	// model), so ±1 on a thin axis would put the plane well outside the
	// attractor and the last part of the knob's travel would do nothing. The
	// reach here is measured PER AXIS from the trajectory itself, during the
	// same warm-up that skips the run-in — see sectSeed.
	posF float32

	// Direction. crossRising is the default and poincare.go says at length
	// why: keeping both directions superimposes two different sections and
	// stops the return map being a function.
	dirF  float32
	viewF float32
	on    bool   // the Trace > Sect switch
	sig   string // system + plane the accumulated crossings belong to
	state [4]float64
	plane analysis.PoincarePlane
	log   analysis.PoincareLog
	draw  []float32 // per-frame vertex scratch
	fit   bool      // the model has fitted its camera to a section worth fitting
}

var sect = poincareSection{
	axisF: sectAxisZ,
	dirF:  analysis.CrossRising,
	viewF: sectViewPlane,
}

// sectAxisNames / sectViewNames are the panel's names for those positions.
// They and poincareDirNames are what paramLabels turns into rotary switches.
var (
	sectAxisNames = []string{"x", "y", "z"}
	sectViewNames = []string{"plane", "flat", "map"}
)

// sectPlaneParams are the three controls that define the section itself, and
// they are the ones the OVERLAY gets.
//
// The view knob is not among them. On a flow mode the section is drawn where
// the crossings are, because that is what an overlay is for — a flat 2-D plot
// superimposed on the tumbling attractor that produced it is two pictures in
// one frame and neither is readable. A knob that appeared there and did
// nothing would be worse than no knob, so it appears only on the model, where
// it means something.
var sectPlaneParams = []paramDef{
	{"sect-axis", "axis", &sect.axisF, sectAxisZ, 0, 2, 1},
	{"sect-pos", "pos", &sect.posF, 0, -1, 1, 0.01},
	{"sect-dir", "dir", &sect.dirF, analysis.CrossRising, 0, 2, 1},
}

// sectViewParams is the model's extra control, appended to the three above.
var sectViewParams = []paramDef{
	{"sect-view", "view", &sect.viewF, sectViewPlane, 0, 2, 1},
}

// ── State ────────────────────────────────────────────────────────────────

// sectCap is how many crossings are kept. Eight thousand is a few minutes of
// Lorenz at the default speed and enough for the section's banding to be solid
// rather than dotted; past that the ring recycles, so the picture keeps
// refreshing instead of freezing into whatever the first pass happened to
// find.
const sectCap = 8192

// sectTransient is how many steps the private integrator runs before anything
// is recorded. The path from the initial condition ONTO the attractor crosses
// the plane too, and those crossings are not part of the section — they are a
// handful of outliers sitting outside the structure, and on a return map an
// outlier is a point that appears to contradict the map.
const sectTransient = 4000

// sectInvalidate throws the accumulated section away. Called from
// reseedAttractorState, so any parameter edit — which changes the system, and
// therefore changes where its trajectory crosses anything — starts a fresh
// one rather than mixing two systems' crossings in one scatter.
func (p *poincareSection) invalidate() { p.sig = "" }

// sectSignature is what the accumulated points belong to. Following bifSig
// rather than wiring a listener onto every control: the knobs reach these
// variables by half a dozen routes (the dial, the LED field, a permalink, Reset
// All, a patch recall, MIDI), and a signature compared once a frame cannot miss
// one of them the way a listener bound to the dial would.
func (p *poincareSection) signature(mode string) string {
	return mode + "|" + strconv.Itoa(p.axis()) +
		"|" + strconv.FormatFloat(float64(p.posF), 'g', 6, 32) +
		"|" + strconv.Itoa(p.direction())
}

// sectDirection is the direction knob as one of poincare.go's constants.
func (p *poincareSection) direction() int {
	d := int(p.dirF + 0.5)
	if d < 0 || d > analysis.CrossEither {
		return analysis.CrossRising
	}
	return d
}

// sectAxis is the axis knob as an index, and sectNormal the unit normal it
// names. Positive, always: "which way through the plane counts" is the
// direction knob's job, and having two controls that can both reverse it makes
// two settings mean the same thing and neither of them mean anything.
func (p *poincareSection) axis() int {
	a := int(p.axisF + 0.5)
	if a < 0 || a > sectAxisZ {
		return sectAxisZ
	}
	return a
}

func sectNormal() [3]float64 {
	var n [3]float64
	n[sect.axis()] = 1
	return n
}

// sectAdvancer returns the one-step integrator for a system, and it is not one
// scheme for all of them.
//
// The classics run a bespoke forward-Euler render loop; everything else goes
// through the shared RK4 loop. A section is a statement about where THE
// TRAJECTORY THE APP DRAWS crosses a plane, so integrating it with a different
// scheme sections a different system — lyapunov.go learned the same lesson the
// hard way and mislabeled a third of the Sprott catalog until it stopped
// pretending one integrator fits all. The dynamics.ClassicSystems lookup is how that
// file decides too; dynamics.FlowSys4.euler carries the same fact for the systems that
// never had a float32 form.
func sectAdvancer(mode string, sys dynamics.FlowSys4, dt float64) func(s *[4]float64) {
	classic := dynamics.IsClassic(mode)
	if classic || sys.Euler {
		return func(s *[4]float64) { twinStep(sys, s, dt) }
	}
	return func(s *[4]float64) { *s = dynamics.RK4x4(sys.F, dt, *s) }
}

// sectField evaluates the vector field at a state, scaled by dt: the "velocity
// per step" poincareCross wants for its cubic.
//
// Two of these are spent per CROSSING, not per step — a crossing happens about
// once per orbit, so on the order of one percent of the integration cost even
// for the interpreted equation engine, where a field evaluation is an AST
// walk. That is why the section always takes the cubic rather than dropping to
// the straight line on the expensive systems: the accuracy is worth three and
// a half orders of magnitude and the cost does not show up.
func sectField(sys dynamics.FlowSys4, s [4]float64, dt float64) [3]float64 {
	dx, dy, dz, _ := sys.F(s[0], s[1], s[2], s[3])
	return [3]float64{dx * dt, dy * dt, dz * dt}
}

// sectSeed restarts the private integrator, measures where the attractor
// actually reaches along the section axis, and builds the plane from the pos
// knob against that reach.
//
// The measurement is why this is not free and why it is cached behind
// sectSignature. It is also why the pos knob means something: without it the
// only thing available is view.fitExtent, which is the camera fit of whatever
// model was last DRAWN — on the Poincaré model that is not the source system
// at all, so the plane would be positioned against the size of a dodecahedron
// somebody looked at earlier.
func (p *poincareSection) seed(mode string, sys dynamics.FlowSys4, dt float64) {
	ic := dynamics.InitCondFor(mode)
	p.state = [4]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), sys.W0}
	adv := sectAdvancer(mode, sys, dt)
	ax := p.axis()
	lo, hi := 0.0, 0.0
	measured := false
	for i := 0; i < sectTransient; i++ {
		adv(&p.state)
		if twinDiverged(p.state) {
			p.state = [4]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), sys.W0}
			lo, hi, measured = 0, 0, false
			continue
		}
		// Measure over the second half only: the first half is still the
		// run-in, and a run-in that starts far from the attractor widens the
		// measured reach to include a path the section is not about.
		if i < sectTransient/2 {
			continue
		}
		v := p.state[ax]
		if !measured {
			lo, hi, measured = v, v, true
			continue
		}
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	mid, half := (lo+hi)/2, (hi-lo)/2
	p.plane = analysis.NewPoincarePlane(sectNormal(), mid+float64(p.posF)*half)
	p.log.Reset(sectCap)
	p.fit = false
}

// sectAdvance integrates n steps, recording every crossing.
func (p *poincareSection) advance(mode string, sys dynamics.FlowSys4, dt float64, n int) {
	adv := sectAdvancer(mode, sys, dt)
	dir := p.direction()
	ic := dynamics.InitCondFor(mode)
	sc := sys.Scale
	for i := 0; i < n; i++ {
		prev := p.state
		adv(&p.state)
		if twinDiverged(p.state) {
			// A reseeded trajectory's first crossing does not follow the last
			// one in time, and the return map must not join them.
			p.state = [4]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), sys.W0}
			p.log.BreakChain()
			continue
		}
		a := [3]float64{prev[0], prev[1], prev[2]}
		b := [3]float64{p.state[0], p.state[1], p.state[2]}
		// The cheap test first: a sign change in the wanted direction, or
		// nothing. Only a step that actually crosses pays for the two field
		// evaluations below, which is what makes the cubic affordable.
		if !analysis.PoincareAccepts(p.plane.Signed(a), p.plane.Signed(b), dir) {
			continue
		}
		hit, ok := analysis.PoincareCross(p.plane, a, b,
			sectField(sys, prev, dt), sectField(sys, p.state, dt), dir)
		if !ok {
			continue
		}
		s, t := p.plane.Project(hit)
		p.log.Add(analysis.PoincareHit{
			P: [3]float32{float32(hit[0]) * sc, float32(hit[1]) * sc, float32(hit[2]) * sc},
			S: float32(s) * sc,
			T: float32(t) * sc,
		})
	}
}

// sectBudget is how many steps a frame integrates. The interpreted engine is
// roughly ten times the per-step cost of a compiled field, and gets a tenth of
// the steps for it — the same split flowregistry.go's frame budgets make.
func sectBudget(sys dynamics.FlowSys4) int {
	if sys.Interpreted {
		return 512
	}
	return 4096
}

// sectRun brings the accumulated section up to date for a system: reseed if
// anything that defines it has changed, then integrate a frame's worth.
// Reports false when there is nothing to draw yet.
func (p *poincareSection) run(mode string) bool {
	sys, ok := dynamics.FlowFor4(mode)
	if !ok {
		return false
	}
	dt := sys.Dt() * float64(sim.speedScale)
	if dt <= 0 {
		return false
	}
	if sig := p.signature(mode); p.sig != sig {
		p.sig = sig
		p.seed(mode, sys, dt)
	}
	p.advance(mode, sys, dt, sectBudget(sys))
	return p.log.Len() >= 2
}

// sectBuf returns the vertex scratch, grown on demand and bounded by the
// vertex budget the trail knob set. vertBuf is sized from the trail length, so
// a short trail is a small buffer and the section has to fit inside it.
func (p *poincareSection) buf(n int) []float32 {
	if max := cap(sim.vertBuf) / 4; n > max {
		n = max
	}
	if n < 0 {
		n = 0
	}
	if cap(p.draw) < n*4 {
		p.draw = make([]float32, n*4)
	}
	return p.draw[:n*4]
}

// sectFillNewest writes the newest hits that fit into v, oldest of them first,
// with the gradient parameter running 0..1 over them so age reads the way it
// does on the trail. get pulls the coordinates out of a hit — which is the
// only difference between drawing the section in place and drawing it flat.
func (p *poincareSection) fillNewest(v []float32, get func(analysis.PoincareHit) (float32, float32, float32)) int {
	n := len(v) / 4
	base := p.log.Len() - n
	inv := float32(1)
	if n > 1 {
		inv = 1 / float32(n-1)
	}
	for i := 0; i < n; i++ {
		x, y, z := get(p.log.At(base + i))
		j := i * 4
		v[j], v[j+1], v[j+2] = x, y, z
		v[j+3] = float32(i) * inv
	}
	return n
}

func sectHitInPlace(h analysis.PoincareHit) (float32, float32, float32) {
	return h.P[0], h.P[1], h.P[2]
}
func sectHitFlat(h analysis.PoincareHit) (float32, float32, float32) { return h.S, h.T, 0 }

// ── The overlay (Trace > Sect) ───────────────────────────────────────────

// sectTick advances the private integrator and draws the crossings in place,
// over the finished trail. Called at the end of every attractor-path frame —
// scan, ring and twin alike — because it is an overlay and not a replacement.
func (p *poincareSection) tick(mode string) {
	if !p.on || mode == "poincare" {
		return
	}
	if !p.run(mode) {
		return
	}
	v := p.buf(p.log.Len())
	n := p.fillNewest(v, sectHitInPlace)
	// Gold, via the monochrome override, and put back afterwards: the section
	// has to be visibly not-the-trail, and the trail's own gradient is whatever
	// the color knobs say.
	//
	// Putting the BASE COLOR back is the part that was missing. renderFrame
	// re-uploads the gradient's source, count, frequency, phase and reverse
	// every frame, but NOT uBaseColor — that is written only when a color knob
	// moves — so the gold this overlay set stayed set, and the trail drew its
	// gradient starting from gold until something touched the palette.
	// Restoring gradientColors alone was enough to hide it from the two- and
	// three-color schemes, where the start color is one end of a mix and reads
	// as a palette choice rather than as a bug.
	glctx.GL.Call("uniform1i", gpu.u.gradientColors, 1)
	glctx.GL.Call("uniform3f", gpu.u.baseColor, 1.0, 0.8, 0.15)
	gpu.uploadVerticesOnly(v, glctx.Types.Points, n)
	if phos.active() {
		// The phosphor owns both of those uniforms while it is on, and
		// renderFrame set them from it earlier this frame. Handing them to the
		// palette here would be handing them to the wrong owner.
		phos.applyPhosphorColor()
		return
	}
	glctx.GL.Call("uniform1i", gpu.u.gradientColors, gradientColorsUniform())
	glctx.GL.Call("uniform3f", gpu.u.baseColor, style.baseColor[0], style.baseColor[1], style.baseColor[2])
}

// wireSectSwitch hooks up the Trace > Sect checkbox. It rebuilds the panel,
// because the Section module comes and goes with it — the same thing the
// Patchbay switch does.
func (p *poincareSection) wireSectSwitch() {
	wireSwitch("sect-sw", func(on bool) {
		p.on = on
		p.invalidate()
		buildParamPanel(run.selectedMode)
	})
}

// ── The model (Analysis → Poincaré) ──────────────────────────────────────

// sectMapGuides is how many dots the return map's y=x diagonal is drawn with.
//
// The diagonal is not decoration. A first-return map's FIXED POINTS are where
// it meets y=x — a period-1 orbit is one point on the diagonal, a period-2
// orbit is a pair straddling it — and the whole reading of the picture is
// against that line. Without it a return map is a parabola floating in space
// with no way to tell which part of it matters.
//
// Drawn as sparse dots rather than as a line for one reason: the scatter is a
// single POINTS draw against one buffer, and a line would be a second draw
// call. uploadVerticesOnly subtracts a running center offset in place and warms
// that offset from what it is handed, so calling it twice a frame would feed
// the centering two different point sets and drift the whole picture. Sixty-
// four dots read as a guide, which is what a guide should look like anyway.
const sectMapGuides = 64

// generatePoincare draws the section as its own model.
func (p *poincareSection) generatePoincare() {
	if !p.run(bif.lastFlowMode) {
		gpu.uploadVerticesOnly(sim.vertBuf[:0], glctx.Types.Points, 0)
		return
	}
	switch int(p.viewF + 0.5) {
	case sectViewFlat:
		// The section laid out in the plane's own basis, face on. In WORLD
		// UNITS, not stretched to fill the frame: both axes are lengths in the
		// same space, and scaling one against the other shears the fractal —
		// the banding's spacing relative to the section's extent is the thing
		// being looked at. The camera fits around it once instead.
		v := p.buf(p.log.Len())
		gpu.uploadVerticesOnly(v, glctx.Types.Points, p.fillNewest(v, sectHitFlat))
	case sectViewMap:
		p.drawReturnMap()
	default:
		// The same picture the overlay draws, with the flow that made it
		// absent — the section in the attractor's own coordinates, so the
		// shape learned while watching the overlay is the shape here.
		v := p.buf(p.log.Len())
		gpu.uploadVerticesOnly(v, glctx.Types.Points, p.fillNewest(v, sectHitInPlace))
	}
	// Fit once, and only once there is a section to fit to. Fitting every frame
	// would rescale the picture as it fills, which is the mistake the Takens
	// mode made with its gain and undid: a figure that resizes as it
	// accumulates reads as the structure itself moving.
	if !p.fit && p.log.Len() > 256 {
		p.fit = true
		view.autoFitCamera()
	}
}

// sectDrawReturnMap plots each crossing's first in-plane coordinate against
// the NEXT crossing's — the first-return map, and the reason to build a
// section at all.
//
// A continuous flow's chaos is hard to see and its return map is not: a
// periodic orbit is a finite set of dots, a period-doubling is that set
// doubling, and a chaotic attractor is a curve — usually a single-humped one,
// which is the logistic map's parabola turning up inside a differential
// equation. That last is the whole content of the phrase "route to chaos", and
// it is one projection away from the section already being computed.
//
// WHICH coordinate: S, the first in-plane axis, which for the three
// axis-aligned planes is the obvious one (y for an x-plane, z for a y-plane, x
// for a z-plane — see poincareBasis). This is a choice and it is not free. The
// return map of a 2-D section is a map OF THE PLANE, and plotting one
// coordinate of it against itself is a 1-D shadow that comes out as a clean
// curve only when the section is nearly 1-D — which is exactly when a strange
// attractor's section is thin, and is why the trick works at all on Lorenz and
// Rössler. On a thick section the plot is a band rather than a curve. That is
// not a failure of the plot; it is the plot saying the section is thick, and
// the flat view next door is where to go and look at it.
//
// NOT normalized to fill the frame, and this matters more here than anywhere.
// Both axes are the same quantity in the same units, so the diagonal is at 45°
// and a point's distance from it is a real distance. Scaling the two axes
// independently — which is what filling the frame means — tilts the diagonal
// to some other angle and destroys the one reading the picture is for. The
// bifurcation explorer does normalize, correctly, because ITS axes are a swept
// parameter against a value and have no common scale.
func (p *poincareSection) drawReturnMap() {
	total := p.log.Len()
	if total < 2 {
		gpu.uploadVerticesOnly(sim.vertBuf[:0], glctx.Types.Points, 0)
		return
	}
	v := p.buf(total - 1 + sectMapGuides)
	room := len(v) / 4
	n := 0
	lo, hi := float32(0), float32(0)
	for i := 1; i < total && n < room-sectMapGuides; i++ {
		next := p.log.At(i)
		if next.Gap {
			// The predecessor is on a different trajectory: joining them plots
			// a point that is about nothing, and one stray dot in open space
			// on an otherwise clean parabola reads as structure.
			continue
		}
		cur := p.log.At(i - 1)
		j := n * 4
		v[j], v[j+1], v[j+2] = cur.S, next.S, 0
		v[j+3] = float32(i) / float32(total)
		if n == 0 {
			lo, hi = cur.S, cur.S
		}
		for _, s := range [2]float32{cur.S, next.S} {
			if s < lo {
				lo = s
			}
			if s > hi {
				hi = s
			}
		}
		n++
	}
	if n == 0 {
		gpu.uploadVerticesOnly(sim.vertBuf[:0], glctx.Types.Points, 0)
		return
	}
	// The y=x guide, spanning the range the data spans.
	for i := 0; i < sectMapGuides && n < room; i++ {
		s := lo + (hi-lo)*float32(i)/float32(sectMapGuides-1)
		j := n * 4
		v[j], v[j+1], v[j+2] = s, s, 0
		v[j+3] = 0
		n++
	}
	gpu.uploadVerticesOnly(v[:n*4], glctx.Types.Points, n)
}

func init() {
	registerGenerate("poincare", sect.generatePoincare)
	// The model's own controls are the section's three plus the view, so the
	// generic parameter grid builds them and there is nothing mode-specific in
	// panelbuild for this. The overlay gets the same three from
	// buildSectionModule, out of the same paramDefs pointing at the same
	// variables — one section, configured in one vocabulary, wherever you
	// happen to be standing when you configure it.
	attractorParams["poincare"] = append(append([]paramDef{}, sectPlaneParams...), sectViewParams...)
}
