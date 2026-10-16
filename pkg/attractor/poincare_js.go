//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/analysis"
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
//   - LINE S (Display), on any flow mode: the crossings drawn in gold WHERE THEY
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

//
// The plane, the integration and the points are PoincareSection
// (poincaresection.go), which a native rack draws too; this is the page's
// side: the overlay switch, the vertex buffers and the camera.

// poincareSection is the Poincaré section on the page: the section, and the
// overlay and the camera that show it.
type poincareSection struct {
	PoincareSection
	on   bool      // the LINE S (Display) switch
	draw []float32 // per-frame vertex scratch
	fit  bool      // the model has fitted its camera to a section worth fitting
}

var sect = poincareSection{PoincareSection: *NewPoincareSection()}

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

// buf returns the vertex scratch, grown on demand and bounded by the
// vertex budget the trail knob set. vertBuf is sized from the trail length, so
// a short trail is a small buffer and the section has to fit inside it.
func (p *poincareSection) buf(n int) []float32 {
	if hi := cap(sim.vertBuf) / 4; n > hi {
		n = hi
	}
	if n < 0 {
		n = 0
	}
	if cap(p.draw) < n*4 {
		p.draw = make([]float32, n*4)
	}
	return p.draw[:n*4]
}

// points fills the scratch with the section as view draws it (Points), and
// reports it and how many points it holds.
func (p *poincareSection) points(view, room int) ([]float32, int) {
	v := p.buf(room)
	i := 0
	n := p.Points(view, len(v)/4, func(x, y, z, age float32) {
		v[i], v[i+1], v[i+2], v[i+3] = x, y, z, age
		i += 4
	})
	return v[:n*4], n
}

// run is Run for a frame of the page's.
func (p *poincareSection) run(mode string) bool {
	before := p.sig
	ok := p.Run(mode, sim.speedScale, 1)
	if p.sig != before {
		p.fit = false // a new section is framed anew
	}
	return ok
}

// ── The overlay (LINE S (Display)) ───────────────────────────────────────────

// tick advances the private integrator and draws the crossings in place,
// over the finished trail. Called at the end of every attractor-path frame —
// scan, ring and start sweep alike — because it is an overlay and not a replacement.
func (p *poincareSection) tick(mode string) {
	if !p.on || mode == "poincare" {
		return
	}
	if !p.run(mode) {
		return
	}
	v, n := p.points(sectViewPlane, p.log.Len())
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
	restoreTraceColor()
}

func (p *poincareSection) wireSectSwitch() {
	wireSwitch("sect-sw", func(on bool) {
		p.on = on
		p.invalidate()
		syncHomedModules(run.selectedMode)
	})
}

// ── The model (Analysis → Poincaré) ──────────────────────────────────────

// generatePoincare draws the section as its own model: in place, face on in
// the plane's own basis, or as its return map (PoincareSection.Points).
//
// Face on, it is in WORLD UNITS, not stretched to fill the frame: both axes
// are lengths in the same space, and scaling one against the other shears the
// fractal — the banding's spacing relative to the section's extent is the
// thing being looked at. The camera fits around it once instead. In place, it
// is the same picture the overlay draws, with the flow that made it absent —
// the section in the attractor's own coordinates, so the shape learned while
// watching the overlay is the shape here.
func (p *poincareSection) generatePoincare() {
	if !p.run(bif.lastFlowMode) {
		gpu.uploadVerticesOnly(sim.vertBuf[:0], glctx.Types.Points, 0)
		return
	}
	room := p.log.Len()
	if p.View() == sectViewMap {
		room += sectMapGuides
	}
	v, n := p.points(p.View(), room)
	gpu.uploadVerticesOnly(v, glctx.Types.Points, n)
	// Fit once, and only once there is a section to fit to. Fitting every frame
	// would rescale the picture as it fills, which is the mistake the Takens
	// mode made with its gain and undid: a figure that resizes as it
	// accumulates reads as the structure itself moving.
	if !p.fit && p.log.Len() > 256 {
		p.fit = true
		view.autoFitCamera()
	}
}

func init() {
	registerGenerate("poincare", sect.generatePoincare)
	attractorParams["poincare"] = append(append([]paramDef{}, sectPlaneParams...), sectViewParams...)
}

// invalidate throws the accumulated section away. Called from
// sim.reseed, so any parameter edit — which changes the system, and
// therefore changes where its trajectory crosses anything — starts a fresh
// one rather than mixing two systems' crossings in one scatter.
func (p *poincareSection) invalidate() { p.sig = "" }
