//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/glctx"
	"github.com/0magnet/chaosrack/pkg/takens"
	"math"
	"strconv"
	"syscall/js"
)

// Stereo embedding — the same 3-D trail as the Takens mode next door, built
// from a DIFFERENT source of independent coordinates.
//
// Takens' theorem exists because you are given one observable and want a
// manifold: the delay vector (s(t), s(t−τ), s(t−2τ)) manufactures the missing
// axes out of the signal's own past, and the reconstruction is diffeomorphic
// to the real attractor. It is a reconstruction because there is nothing else
// to work with.
//
// A stereo signal is TWO observables of one system, already measured. Spending
// a delay to invent a second axis when a second axis was recorded is throwing
// away the measurement and then approximating it. So this mode plots the
// channels against each other: what you see is the actual relationship between
// them — phase, polarity, correlation, width — rather than a reconstruction of
// one of them. A pure tone panned center draws a line at 45°; the same tone
// with the channels 90° apart draws a circle; a wide mix draws a fat cloud;
// a channel flipped in polarity swings the whole figure onto the other
// diagonal. That is the goniometer (vectorscope) every mastering desk has, and
// it is a 2-D display everywhere else because a scope has two deflection
// axes. This one has three, so the third can carry something.
//
// Everything else is deliberately the Takens mode's: takensWindow's window /
// stride / budget arithmetic, takensSmooth's Catmull-Rom beam, takensVerts,
// takensFitExtent's fixed-scale camera fit, and the same fixed GAIN with
// nothing auto-ranging. Those were argued out over there and the arguments do
// not change because the coordinates came from two channels instead of one;
// see takens_js.go for why the scale is fixed and why the window is a duration.
//
// WHAT IS NOT SHARED, and why: the samples. takens_js.go drains the source
// into emb.ring, and Drain is defined on the PRIMARY CHANNEL only — that
// ring is mono by construction and no amount of reading it produces a right
// channel. Real per-channel samples come from Source.TimeDomainStereo, which
// is a snapshot (latest N, oldest first) rather than a stream, so this mode
// asks for its whole window every frame instead of accumulating one. That is
// also why there is no ring here at all: the snapshot IS the window.
//
// The mono case is not faked. The xy scope, when handed a one-channel source,
// substitutes a lagged copy of the left channel for the right so the trace is
// not a straight diagonal — reasonable for a scope whose job is to draw
// something, and wrong here, because a lagged copy is precisely the delay
// coordinate this mode exists to stop pretending is a second channel. A mono
// source collapses the L/R pair onto the diagonal L=R and it is SAID SO (see
// stereoReadout and the one-shot notice below), because that is the true
// picture of a signal with no stereo information in it.

// ── Axis assignment ──────────────────────────────────────────────────────
//
// Two independent choices, so four positions rather than one guess:
//
//   BASIS — L/R or mid/side. M=(L+R)/2 and S=(L−R)/2 are the same plane
//   rotated 45°: correlated (center) content lies along M and difference
//   content along S, so "how wide is this" becomes an extent along one axis
//   instead of an eccentricity of a tilted ellipse. Broadcast goniometers
//   ship rotated for exactly this reason. Mid/side is also the position that
//   degrades WELL on a mono source: S ≡ 0, and (M, 0, M(t−τ)) is a legitimate
//   two-coordinate delay embedding drawn in the x–z plane rather than a
//   diagonal streak.
//
//   THIRD AXIS — a delay of the first coordinate, or time. The delay keeps the
//   figure an embedding (τ means what it means in takens_js.go, and the same
//   MEAS estimator answers it) and gives the flat Lissajous depth: a tone that
//   draws one ellipse edge-on unrolls into a helix. Time instead sweeps the
//   figure along a ribbon so successive cycles stack rather than overwrite,
//   which is the only way to SEE a slow phase drift — on the flat display a
//   drifting figure just wobbles.
//
// The default is position 0, (L, R, L(t−τ)):
//
//   - Looked at down the third axis it is EXACTLY the xy scope's figure, which
//     is the display everyone arriving here already knows. Rotating then ADDS
//     information to something familiar instead of showing something new that
//     has to be learned before it can be read.
//   - Its third axis is a genuine delay coordinate, so the mode is still an
//     embedding and τ is still τ. Defaulting to a time axis would make the
//     first thing anyone sees the one position that is not an embedding at all.
//   - L/R over mid/side because the axes are then the things the source
//     actually has — a listener knows which speaker is which; nobody's
//     intuition starts at "side content".
//
// Rejected: (L, R, L−R). It looks like a third axis and is not one — L−R is a
// linear combination of the first two, so every point lies in a fixed plane
// and rotating the model reveals that the "solid" is a sheet. The same
// objection kills (M, S, L) and any other triple whose three coordinates are
// three linear functions of two samples: a third axis has to come from another
// time or another measurement, and there are only those two ways to get one.
//
// Also rejected: making τ mean an inter-channel delay on the time positions,
// so the knob would do something in all four. One knob with two meanings that
// swap under another knob is worse than a knob that is plainly inert; on the
// time positions τ does nothing and the tooltip says so.

// stereoInst is ONE stereo embedding: everything the mode reads or writes
// while drawing, in a struct rather than in package variables.
//
// It was eighteen globals, which is the same thing said in a way that
// permits exactly one of them. Two of these side by side — the same audio
// under different axes, or one held while the other is turned — is not a
// feature that could be added on top of globals; it is what having a struct
// means. The pure helpers around it (stereoWiden, stereoWindow,
// stereoCorrelation, stereoReadout) already took their arguments and so are
// already instance-free; only the state lived in one place, and now it does
// not.
//
// The knob table binds to the fields of an instance, so a second instance
// gets a second row of knobs by construction rather than by a parallel set
// of ids someone has to keep in step.
type stereoInst struct {
	StereoScope

	l, r []float32 // this frame's snapshot, oldest first

	// fitGain is the GAIN the camera was last fitted to, 0 for not yet.
	// A gain rather than a bool for emb.fitGain's reason: the bound the fit
	// is made against is a function of the gain, so a fit made at one gain is
	// not a fit at another, and raising GAIN under a bool pushed the figure
	// off the screen with only Zoom to bring it back.
	fitGain float32

	readEl   js.Value // the readout in the parameter grid
	readText string   // last text written to it (DOM write only on change)

	monoSrc bool    // the source itself has one channel
	corrOK  bool    // there is enough signal for a correlation to mean anything
	corr    float32 // Pearson r between the channels, −1..+1

	// collapsed counts CONSECUTIVE frames in which the two axes carry the
	// same signal. A count rather than a flag because music is mono for a bar
	// at a time all the time — a solo instrument panned center, a fade to a
	// single voice — and a notice that fired on that would be noise. A second
	// of it is a property of the source, not of the passage.
	collapsed int
	noticed   bool // the notice has been shown once since audio started
}

// newStereoInst returns an instance at the defaults the knobs reset to.
func newStereoInst() *stereoInst {
	return &stereoInst{StereoScope: *NewStereoScope()}
}

// stereo is the instance the single on-screen stereo mode draws. A second
// viewInsts are the stereo mode's per-view instances. Two, because Views
// viewInsts is one instance per grid cell, allocated for the largest grid
// the dial offers: sixteen small structs, and only the cells actually
// drawn ever allocate their sample buffers.
var viewInsts = newViewInsts()

func newViewInsts() [viewMax]*stereoInst {
	var out [viewMax]*stereoInst
	for i := range out {
		out[i] = newStereoInst()
	}
	return out
}

// stereo is the instance the PANEL drives and the one anything outside the
// draw passes means: the focused view's, or view A's when the views are
// linked or there is only one.
//
// It is a pointer rather than a value because the draw swaps it per pass —
// see drawViewPasses — and because the parameter rows bind to the fields of
// whatever it points at WHEN THE ROW IS BUILT. That is why changing focus
// rebuilds the panel rather than merely moving this: a row already built
// holds &viewInsts[0].tau directly and would go on writing there.
var stereo = viewInsts[0]

func init() {
	registerGenerate("stereo", stereo.generate)
	attractorParams["stereo"] = []paramDef{
		{"stereo-axes", "axes", &stereo.axesF, 0, 0, float32(len(stereoPlans) - 1), 1},
		{"stereo-tau", "τ", &stereo.tau, takens.TauDef, 1, takens.TauMax, 1},
		{"stereo-win", "win", &stereo.win, 85, 5, stereoWinMax, 5},
		{"stereo-gain", "gain", &stereo.gain, 10, 0.5, 50, 0.5},
		{"stereo-align", "algn", &stereo.align, 0, -stereoAlignMax, stereoAlignMax, 1},
		{"stereo-width", "wide", &stereo.width, 1, 0, 3, 0.05},
		{"stereo-vg", "vg", &stereo.vgain, 1, 0.1, 8, 0.1},
		{"stereo-span", "span", &stereo.span, 1, 0.25, 8, 0.25},
		{"stereo-trig", "trig", &stereo.trig, stereoTrigOff, stereoTrigOff, stereoTrigLock, 1},
		{"stereo-lvl", "lvl", &stereo.lvl, 0, -1, 1, 0.01},
		{"stereo-tsrc", "tsrc", &stereo.tsrc, trigSrcMid, trigSrcMid, trigSrcR, 1},
		{"stereo-tcpl", "tcpl", &stereo.tcpl, trigCplDC, trigCplDC, trigCplHFRej, 1},
		{"stereo-hyst", "hyst", &stereo.hyst, 0.02, 0, 0.5, 0.01},
		{"stereo-trun", "run", &stereo.trun, trigRunAuto, trigRunAuto, trigRunSingle, 1},
		{"stereo-hold", "hold", &stereo.hold, 0, 0, 500, 5},
		{"stereo-tpos", "tpos", &stereo.tpos, 0, 0, 1, 0.05},
		{"stereo-grat", "grid", &stereo.grat, gratOff, gratOff, gratFull, 1},
		{"takens-smooth", "smth", &takensSmoothF, 4, 1, 16, 1},
	}
}

// generate snapshots both channels and draws the newest window as a
// trail through the normal 3D pipeline.
func (s *stereoInst) generate() {
	src := aud.ensureAudioSource()
	sr := 24000
	if src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	smooth := takensSmooth()
	snap, nv := s.Window(sr, sim.steps, smooth)
	if len(s.l) < snap {
		// Grown by half again, as the Takens ring is, so that turning the WIN
		// knob does not reallocate on every step of the dial.
		s.l = make([]float32, snap+snap/2)
		s.r = make([]float32, len(s.l))
	}
	vertices := sim.vertBuf[:nv*4]
	if !src.Ready() {
		// Re-upload the previous frame rather than a cleared buffer, so the
		// model does not flicker while the source spins up — and refit when
		// audio arrives, since the mode-entry fit saw whatever was here.
		s.fitGain = 0
		s.noteState(false, false, 0)
		gpu.uploadVerticesOnly(vertices, gpu.drawMode, nv)
		return
	}
	// The whole window, every frame. There is no accumulation to get wrong and
	// no ring to keep — but for the first fraction of a second after a source
	// starts, the front of the snapshot is the zeros ring.latest writes where
	// there is no data yet, so the figure trails a straight run into the origin
	// and the correlation reads low. It clears itself as the source's ring
	// fills (a third of a second at 48 kHz on a 16384-sample ring) and it
	// cannot cause a false report: the collapse notice wants a solid second.
	l, r := s.l[:snap], s.r[:snap]
	src.TimeDomainStereo(l, r)
	if !s.Frame(l, r, sr, sim.steps, smooth, vertices) {
		// The trigger holds the last frame: re-upload it as it was.
		gpu.uploadVerticesOnly(vertices, gpu.drawMode, nv)
		return
	}
	gpu.uploadVerticesOnly(vertices, gpu.drawMode, nv)
	// The reference lines go up after the trace, as the Poincaré overlay
	// does: a separate draw call over the finished figure.
	s.drawGraticule()

	// The RAW pair, not the widened or realigned one: the meter reports the
	// source as it is, so that ALIGN and WIDE can be turned to ask what-if
	// questions without the number moving to agree with the answer.
	corr, ok := stereoCorrelation(l, r)
	s.noteState(src.Channels() < 2, ok, corr)

	if s.fitGain != s.gain && !pmod.paramIsModulated("stereo-gain") {
		// Fitted to the FIXED scale's worst case, not to this window — see the
		// same block in generateTakens for why fitting the instantaneous
		// figure is what put loud passages off the screen. Every coordinate
		// here is bounded by gain (samples are bounded to ±1; mid and side by
		// construction; the time ramp by its own mapping), so the Takens
		// mode's √3 cube-corner extent is the right bound unchanged.
		s.fitGain = s.gain
		view.fitOverride = takensFitExtent(s.gain)
		view.autoFitCamera()
	}
}

// ── Saying what the figure is of ─────────────────────────────────────────
//
// A mono source draws a straight diagonal streak and a broken renderer draws a
// straight diagonal streak, so the picture alone cannot tell you which you
// have. Two things say it instead: a readout that is always there, and a
// one-shot notice for the case where the readout is not enough because you
// were not looking at the panel.

// stereoNoticeFrames is a second at 60 Hz.
const stereoNoticeFrames = 60

// stereoCollapseR is how correlated two channels have to be before the figure
// is a line rather than a thin ellipse. At r = 0.999 the minor axis is about
// 2% of the major: at any usable gain that is under a pixel wide.
const stereoCollapseR = 0.999

// stereoCorrelation is the Pearson correlation of the two channels over the
// window — the number a goniometer's correlation meter shows. +1 is mono
// (the diagonal), 0 is uncorrelated (a round cloud), −1 is a polarity flip
// (the other diagonal, and the thing that vanishes when someone sums the mix
// to mono).
//
// The means are subtracted rather than assumed zero. Audio is nominally
// AC-coupled and in practice is not: a cheap ADC's DC offset, or a window
// short enough to sit inside one cycle of something very low, both move the
// mean, and a raw Σlr/√(Σl²Σr²) then reports the offset's correlation instead
// of the signal's — which is +1 for any two channels that happen to share a
// rail.
//
// ok is false when there is nothing to correlate: silence, or one dead
// channel, where the denominator is zero and any answer would be invented. The
// floor is a variance of 1e-9, an RMS around −90 dBFS, which is below the
// noise floor of anything real and above exact digital zero.
func stereoCorrelation(l, r []float32) (float32, bool) {
	n := len(l)
	if n != len(r) || n < 2 {
		return 0, false
	}
	var sl, sr float64
	for i := range n {
		sl += float64(l[i])
		sr += float64(r[i])
	}
	ml, mr := sl/float64(n), sr/float64(n)
	var sll, srr, slr float64
	for i := range n {
		a := float64(l[i]) - ml
		b := float64(r[i]) - mr
		sll += a * a
		srr += b * b
		slr += a * b
	}
	fn := float64(n)
	if sll/fn < 1e-9 || srr/fn < 1e-9 {
		return 0, false
	}
	c := slr / math.Sqrt(sll*srr)
	// Rounding can put an exactly-identical pair a hair outside the range, and
	// a readout of "r+1.00" that came from 1.0000000002 is fine while a
	// downstream acos of it would not be.
	if c > 1 {
		c = 1
	} else if c < -1 {
		c = -1
	}
	return float32(c), true
}

// stereoReadout is the readout text for a measured state: six characters at
// most, the size of its display (appendReadout).
// matching the Takens mode's "τ32 m4" — the cell is a third of a module wide.
func stereoReadout(monoSrc, ok bool, corr float32) string {
	if monoSrc {
		return "mono"
	}
	if !ok {
		return "r --"
	}
	sign := "+"
	if corr < 0 {
		sign = "-"
		corr = -corr
	}
	return "r" + sign + strconv.FormatFloat(float64(corr), 'f', 2, 32)
}

// stereoIsCollapsed reports whether the two axes are carrying the same signal,
// so that the figure is a line and not a figure. Both causes count: a source
// with one channel (TimeDomainStereo copies left into right), and a stereo
// source whose channels happen to be identical — a mono file in a stereo
// container, a mono capture device, a synth patched to both outputs.
func stereoIsCollapsed(monoSrc, ok bool, corr float32) bool {
	if monoSrc {
		return true
	}
	return ok && float64(corr) >= stereoCollapseR
}

// noteState records this frame's measurement, updates the readout, and
// raises the notice once the collapse has persisted.
func (s *stereoInst) noteState(monoSrc, ok bool, corr float32) {
	s.monoSrc, s.corrOK, s.corr = monoSrc, ok, corr
	s.showReadout(stereoReadout(monoSrc, ok, corr))

	if !stereoIsCollapsed(monoSrc, ok, corr) {
		// Re-armed, so a source that is swapped for a mono one later in the
		// session is reported too. It cannot become chatter: re-arming costs a
		// non-collapsed frame and the notice costs a further second of
		// collapse, and showAudioStatus suppresses a message identical to the
		// one it last showed anyway.
		s.collapsed, s.noticed = 0, false
		return
	}
	if s.collapsed < stereoNoticeFrames {
		// Stops at the threshold rather than counting on: this runs every
		// frame for as long as the mode is up, and the number past the
		// threshold means nothing to anyone.
		s.collapsed++
	}
	if s.noticed || s.collapsed < stereoNoticeFrames {
		return
	}
	s.noticed = true
	// Reusing the audio status overlay rather than inventing a second one: it
	// already shows once per change, auto-hides, and can be tapped away, and a
	// message about the audio belongs where the messages about the audio go.
	if monoSrc {
		aud.showAudioStatus("Mono source — both axes carry the same signal, so the figure lies on the diagonal. " +
			"The MS positions draw it as a delay embedding instead.")
	} else {
		aud.showAudioStatus("The two channels are identical — the figure is a diagonal line. " +
			"Nothing is wrong with the display; there is no stereo information in this source.")
	}
}

// showReadout writes the LED, but only when the text actually changes.
// The correlation moves continuously and the DOM does not need to hear about
// every frame of it; more to the point, a two-decimal readout that re-renders
// sixty times a second is unreadable, which is the same complaint that keeps
// the Takens mode's τ on a button.
func (s *stereoInst) showReadout(text string) {
	if text == s.readText {
		return
	}
	s.readText = text
	if s.readEl.Truthy() {
		setDotText(s.readEl, text)
	}
}

// appendReadout adds the correlation readout to the model's readout line
// (liveReadoutHost).
func (s *stereoInst) appendReadout(host js.Value) {
	// Seeded from the last measurement, not from a placeholder: the panel is
	// rebuilt on every mode change and every module toggle, and a cell that
	// came back reading "r --" over a live stereo source would be reporting
	// silence that is not there. s.readText is cleared so the next frame
	// writes into the NEW element rather than skipping it as unchanged.
	s.readText = ""
	s.readEl = liveReadout(host, "corr", dispFullChars, stereoReadout(s.monoSrc, s.corrOK, s.corr),
		"Correlation between the two channels over the display window, as a goniometer's "+
			"correlation meter reads it: +1.00 means the channels are identical and the figure is the diagonal "+
			"line, 0 means they are unrelated and the figure is a round cloud, −1.00 means one is the other's "+
			"polarity inverted (and the difference disappears if the mix is summed to mono). "+
			"\"mono\" means the source has only one channel, so there is no stereo relationship to draw. "+
			"\"r --\" means silence, or one dead channel: nothing to correlate.")
}

// gratBuf is the graticule's own vertex buffer: a handful of lines, rebuilt
// only when the extent or the mode changes.
var gratBuf []float32

// drawGraticule puts the reference lines up behind the trace.
//
// A separate draw call with a flat color, the way the Poincaré overlay does
// it, and the uniforms are put back afterwards for the same reason: the
// trail's own gradient is whatever the color knobs say, and leaving the
// override set paints the next thing drawn in graticule gray.
func (s *stereoInst) drawGraticule() {
	if s.gratMode() == gratOff || !(s.gain*s.vgain > 0) {
		return
	}
	// 2 axes, and in full 2 diagonals plus a 4-segment box: 16 vertices at
	// most, 4 floats each.
	if cap(gratBuf) < 16*4 {
		gratBuf = make([]float32, 16*4)
	}
	v := gratBuf[:0]
	line := func(x0, y0, x1, y1 float32) {
		// The fourth float is the color coordinate; the flat override
		// ignores it, and a constant keeps it out of any gradient that is
		// somehow still bound.
		v = append(v, x0, y0, 0, 0, x1, y1, 0, 0)
	}
	s.gratLines(line)
	n := len(v) / 4

	glctx.GL.Call("uniform1i", gpu.u.gradientColors, 1)
	glctx.GL.Call("uniform3f", gpu.u.baseColor, GraticuleColor[0], GraticuleColor[1], GraticuleColor[2])
	gpu.uploadVerticesOnly(v, glctx.Types.Lines, n)
	if phos.active() {
		// The phosphor owns these two while it is on; handing them to the
		// palette here would hand them to the wrong owner. sect.tick says
		// the same thing about the same pair.
		phos.applyPhosphorColor()
		return
	}
	glctx.GL.Call("uniform1i", gpu.u.gradientColors, gradientColorsUniform())
	glctx.GL.Call("uniform3f", gpu.u.baseColor, style.baseColor[0], style.baseColor[1], style.baseColor[2])
}
