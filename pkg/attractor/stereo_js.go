//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"github.com/0magnet/chaosrack/pkg/takens"
	"math"
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
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
// into takensRing, and Drain is defined on the PRIMARY CHANNEL only — that
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

// stereoChan names the scalar signal an axis carries. The first four are
// functions of one (L, R) sample pair; chTime is the odd one out and is never
// read from a sample at all (see stereoChanValue).
type stereoChan uint8

const (
	chL stereoChan = iota
	chR
	chMid
	chSide
	chTime
)

// stereoPlan is one axis assignment: which signal each of x, y, z carries, and
// whether that axis reads the delayed sample rather than the current one.
type stereoPlan struct {
	ch    [3]stereoChan
	delay [3]bool
}

// stereoPlans is indexed by the axes knob. Positions are the knob's values, so
// the order here IS the order on the dial: the two L/R positions first (see
// the default's justification above), delay before time in each pair.
var stereoPlans = [...]stereoPlan{
	{ch: [3]stereoChan{chL, chR, chL}, delay: [3]bool{false, false, true}},
	{ch: [3]stereoChan{chL, chR, chTime}},
	{ch: [3]stereoChan{chMid, chSide, chMid}, delay: [3]bool{false, false, true}},
	{ch: [3]stereoChan{chMid, chSide, chTime}},
}

// stereoAxisNames are the dial's positions by name — the tooltip on each
// detent. paramLabels turns a numeric knob into a labeled rotary switch given
// these; paramdefs_js.go is where both tables are indexed.
// The two TIME positions are goniometers in the instrument sense — a two
// channel vector display — with the third axis sweeping like a scope
// timebase. The two DELAY positions are not: a delayed copy on the third
// axis makes them a delay embedding of the pair, which is a different
// figure answering a different question, so they are not called one.
var stereoAxisNames = []string{
	"L/R delay embedding — L, R, L(t−τ)",
	"L/R goniometer — L, R, time sweep",
	"mid/side delay embedding — M, S, M(t−τ)",
	"mid/side goniometer — M, S, time sweep",
}

// stereoAxisRing is what fits AROUND the dial: five runes per position, and for
// a named setting the ring IS the readout — the LED is hidden deliberately,
// because seven segments cannot spell a word — so these have to be told apart
// at a glance and not merely be short. Basis in caps, since L, R, M and S are
// what the channels are called everywhere else here; third axis lower case.
//
// It said "LRτ"/"LRt"/"MSτ"/"MSt" first, which is the notation the rest of this
// file uses, and it was UNREADABLE: captured off the real panel, τ and t at ring
// size are the same few pixels, so the dial showed what looked like two "LRt"
// and two "MSt" and there was no way to tell which of a pair was selected.
// "d" for delay is not the notation, and it is legible, which beats it.
// "g" for goniometer on the two TIME positions, "d" for delay on the other
// two. It said "t" for time, which described the axis correctly and said
// nothing about what the figure IS — and the pair a reader wants to tell
// apart at a glance is goniometer against embedding, not time against delay.
var stereoAxisRing = []string{"LRd", "LRg", "MSd", "MSg"}

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
	axesF float32 // axes knob: an index into stereoPlans
	tau   float32 // delay τ, reference samples (delay positions only)
	win   float32 // display window, milliseconds
	gain  float32 // world units a full-scale (±1) sample maps to

	l, r []float32 // this frame's snapshot, oldest first

	// fitGain is the GAIN the camera was last fitted to, 0 for not yet.
	// A gain rather than a bool for takensFitGain's reason: the bound the fit
	// is made against is a function of the gain, so a fit made at one gain is
	// not a fit at another, and raising GAIN under a bool pushed the figure
	// off the screen with only Zoom to bring it back.
	fitGain float32

	// align is an INTER-CHANNEL delay: right is read this many reference
	// samples later than left, signed, so either channel can be the late one.
	// A time offset between channels is the fault a goniometer gets reached
	// for — a spaced pair of microphones, a mis-clocked converter, a plugin
	// reporting its latency wrong. It draws as a figure that opens into an
	// ellipse and rotates as the frequency moves, and the way to CONFIRM it
	// is to dial the offset out and watch the figure collapse back onto the
	// diagonal, at which point the knob reads how far apart they were.
	// ±96 reference samples is ±2 ms: a 68 cm path difference in air.
	align float32

	// width scales SIDE against MID: at 1 the figure is what was recorded,
	// below it the difference content shrinks toward mono, above it the
	// figure spreads. It acts on the DRAWN figure only — nothing is written
	// back to the audio, so the correlation meter goes on reading the source
	// as it actually is rather than as the knob is pretending.
	width float32

	// vgain is the VERTICAL GAIN a scope has and gain here is not. gain
	// scales the geometry and the camera fit is a function of it, so the two
	// cancel and the figure is the same size on screen at 0.5 as at 50. This
	// multiplies the two SIGNAL axes and the fit does not know about it, so
	// turning it up can push the trace off the frame — which is the point.
	vgain float32

	// span stretches the TIME axis, and only the time axis. Stretching one
	// axis of a 3D model is a distortion and is offered nowhere else here;
	// time is the exception, because it is a ramp this code synthesizes from
	// the vertex index rather than a measured dimension. Scaling it is a
	// TIMEBASE. Inert on the two delay positions, where all three
	// coordinates are signal.
	span float32

	// trig selects the trigger edge, and lvl the level it looks for. See the
	// TRIGGER section below: together they fix the START of the window to a
	// repeatable feature of the signal instead of to the newest sample, which
	// is what makes a periodic figure stand still.
	trig float32
	lvl  float32
	tsrc float32 // trigger source: mid, L or R
	tcpl float32 // trigger coupling: DC, LF reject, HF reject
	trun float32 // run mode: auto, normal, single
	hold float32 // holdoff, milliseconds
	tpos float32 // where the trigger sits in the window, 0..1
	grat float32 // graticule: off, axes, full

	// frozen is SINGLE having caught its frame; lastRun notices the dial
	// moving, which is what re-arms it.
	frozen  bool
	lastRun int

	hyst float32 // noise reject: how far past the level a crossing must go

	// trigBuf is the trigger path for this frame, kept so a filter run does
	// not allocate sixty times a second.
	trigBuf []float32

	// lockWave is the stretch of trigger signal the last frame drew, kept
	// for the waveform lock to match against.
	lockWave []float32

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
	return &stereoInst{
		axesF: 0,
		tau:   takens.TauDef,
		win:   85,
		gain:  10,
		width: 1,
		vgain: 1,
		span:  1,
		trig:  stereoTrigOff,
		lvl:   0,
		tsrc:  trigSrcMid,
		tcpl:  trigCplDC,
		hyst:  0.02,
		trun:  trigRunAuto,
		hold:  0,
		tpos:  0,
		grat:  gratOff,
	}
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

// stereoWinMax is the WIN knob's top, in milliseconds, and it is lower than
// the Takens mode's 500 for two unrelated reasons that happen to agree.
//
// The hard one: this mode SNAPSHOTS its window (audiosrc.DefaultRingSize and
// the comment on it), and a snapshot longer than the source's ring comes back
// silently wrapped. 250 ms is 12000 samples at 48 kHz, which leaves room for
// the largest τ under the 16384-sample ring. stereoWindow clamps as well, so
// an unusual sample rate cannot walk past the bound — the knob's max is what
// keeps the clamp from ever being the thing the user is fighting.
//
// The soft one: a goniometer is a PHASE display, and phase displays are read
// over a few tens of milliseconds. Past a couple of hundred the figure is a
// filled blob whatever the source is doing, so the range the knob does not
// cover is range nobody would turn it to. The Takens mode wants the long
// window because a reconstructed manifold fills in as it accumulates; a
// Lissajous just gets darker.
const stereoWinMax = 250

// stereoAlignMax is the ALIGN knob's reach either way, in reference samples:
// 96 of them is 2 ms, which is a 68 cm path difference in air. The snapshot has
// to hold the offset on top of the window and the delay, and stereoWindow's
// clamp reserves it — see the invariant there.
const stereoAlignMax = 96

// stereoSpanMax is the most samples one frame may ask the source for. Named
// against the source's own constant rather than repeating the number, so the
// two cannot drift apart.
const stereoSpanMax = audiosrc.DefaultRingSize

// stereoChanValue derives an axis's scalar from one (L, R) sample pair.
//
// mid and side carry the ½ so that a mono signal draws mid at exactly the size
// L would have drawn: switching basis must not resize the figure, or the
// switch reads as a zoom. Both stay bounded by 1 for inputs bounded by 1,
// which is what lets takensFitExtent's fixed camera fit apply unchanged.
//
// chTime returns 0 and is never called in anger: a time axis is a function of
// where you are in the window, not of the sample there, so generateStereo
// fills it from the vertex index. It is in the enum because it is an axis
// ASSIGNMENT, and returning 0 rather than panicking keeps a plan built wrong
// in future to a flat figure instead of a dead page.
func stereoChanValue(c stereoChan, l, r float32) float32 {
	switch c {
	case chR:
		return r
	case chMid:
		return (l + r) * 0.5
	case chSide:
		return (l - r) * 0.5
	case chTime:
		return 0
	default: // chL
		return l
	}
}

// stereoAxisSel is the axes knob as a plan index, clamped. Audio modulation can
// drive any registered parameter, this one included, so the value arriving here
// is not necessarily one of the detents — and a modulator riding a feature that
// has gone to zero or to infinity can hand over an out-of-range float or a NaN.
//
// The range is checked BEFORE the conversion, not after. A float-to-int
// conversion whose value does not fit is implementation-defined in Go, so
// clamping the RESULT of int(±Inf + 0.5) is clamping whatever the runtime
// happened to produce. NaN falls out of the same comparison, since every
// comparison against a NaN is false.
func (s *stereoInst) axisSel() int {
	last := len(stereoPlans) - 1
	v := s.axesF
	if !(v > 0) { // false for NaN too
		return 0
	}
	if v > float32(last) {
		return last
	}
	return int(v + 0.5)
}

// stereoWindow is takensWindow with the snapshot bound applied first: same
// duration-not-a-point-count arithmetic, same stride, same vertex budget.
//
// The clamp is on the DURATION rather than on the returned n, because reducing
// n after the fact would silently deliver a shorter window than the stride was
// computed for. Clamping first means the numbers that come back are internally
// consistent and the only casualty is milliseconds the user asked for and
// cannot have.
//
// The invariant the caller depends on: (n−1)·stride + tau + 1 ≤ stereoSpanMax,
// for every sample rate and every τ the knob can reach. It holds because
// takensWindow's window never exceeds the requested duration in samples, and
// the clamp reserves τ+1 of the budget before asking. (It would fail if τ could
// approach stereoSpanMax itself, leaving less than takensWindow's 64-sample
// floor; the knob tops out at 512, five bits short of that.)
func stereoWindow(winMS float32, sampleRate, budget, tau, align int) (n, stride int) {
	sr := sampleRate
	if sr <= 0 {
		sr = 24000
	}
	if align < 0 {
		align = -align
	}
	// The ALIGN offset costs snapshot on top of τ, because the two channels are
	// then read from indices that far apart and the snapshot has to cover both
	// runs. Reserved here with τ for the same reason τ is: clamping the
	// DURATION keeps the numbers that come back internally consistent, where
	// trimming n afterwards would deliver a shorter window than the stride was
	// computed for.
	if maxMS := float32(stereoSpanMax-tau-align-1) / float32(sr) * 1000; winMS > maxMS {
		winMS = maxMS
	}
	return takensWindow(winMS, sr, budget)
}

// stereoAlignSamples is the ALIGN knob in source samples, signed, clamped to
// the knob's own reach — the snapshot bound is computed from that reach, so a
// value past it (a hand-edited permalink, a modulator) would ask for samples
// outside the buffer.
func stereoAlignSamples(align float32, sr int) int {
	if align > stereoAlignMax {
		align = stereoAlignMax
	} else if align < -stereoAlignMax {
		align = -stereoAlignMax
	} else if align != align { // NaN
		return 0
	}
	neg := align < 0
	if neg {
		align = -align
	}
	n := takens.TauSamples(align, sr)
	if align < 0.5 {
		n = 0 // takens.TauSamples floors at 1; a zero offset has to stay zero
	}
	if neg {
		return -n
	}
	return n
}

// stereoWiden applies the WIDTH knob to one (L, R) pair: side is scaled against
// mid and the pair rebuilt from them. At 1 it is the identity.
//
// Done to the PAIR rather than to the side axis alone, so every axis assignment
// sees the same widened signal — the L/R positions show what widening does to
// the channels, which is the question, and the mid/side positions show it as an
// extent along one axis. Scaling only the side axis would make the L/R
// positions silently ignore the knob.
func stereoWiden(l, r, width float32) (float32, float32) {
	if width == 1 || width != width { // identity, and NaN leaves the pair alone
		return l, r
	}
	if width < 0 {
		width = 0
	}
	m := (l + r) * 0.5
	s := (l - r) * 0.5 * width
	return m + s, m - s
}

// generateStereo snapshots both channels and draws the newest window as a
// trail through the normal 3D pipeline.
func (s *stereoInst) generate() {
	src := ensureAudioSource()
	sr := 24000
	if src != nil && src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	tau := takens.TauSamples(s.tau, sr)
	align := stereoAlignSamples(s.align, sr)
	n, stride := stereoWindow(s.win, sr, steps, tau, align)
	span := (n-1)*stride + tau
	// The two channels are read from indices `align` apart, so the snapshot has
	// to cover both runs: |align| more samples, with the earlier channel
	// starting at 0 and the later one offset into them. baseL and baseR are
	// those two starts, and choosing them as max(±align, 0) keeps both inside
	// [0, |align|] whichever way round the offset goes.
	absA := align
	if absA < 0 {
		absA = -absA
	}
	baseL, baseR := 0, 0
	if align > 0 {
		baseL = align // right is read EARLIER, i.e. delayed into line with left
	} else {
		baseR = -align
	}
	// The trigger searches BEHIND the window, so the snapshot carries a margin
	// of older audio for it to look in. Zero when the trigger is off, which
	// leaves the snapshot exactly the size it always was.
	margin := s.trigMargin(span)
	// The waveform lock compares a whole window at every candidate offset,
	// so the snapshot has to reach further back than the search margin
	// alone would need.
	lockPad := 0
	if s.trigMode() == stereoTrigLock {
		lockPad = lockPoints*lockDecim(span) + 1
	}
	snap := span + absA + 1 + margin + lockPad
	if len(s.l) < snap {
		// Grown by half again, as the Takens ring is, so that turning the WIN
		// knob does not reallocate on every step of the dial.
		s.l = make([]float32, snap+snap/2)
		s.r = make([]float32, len(s.l))
	}
	nv := takensVerts(n)
	vertices := vertBuf[:nv*4]
	if src == nil || !src.Ready() {
		// Re-upload the previous frame rather than a cleared buffer, so the
		// model does not flicker while the source spins up — and refit when
		// audio arrives, since the mode-entry fit saw whatever was here.
		s.fitGain = 0
		s.noteState(false, false, 0)
		uploadVerticesOnly(vertices, attractorDrawMode, nv)
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

	// Where in the margin this frame starts. margin means "the newest
	// possible window", which is what an untriggered draw has always used.
	s.rearmIfModeMoved()
	toff := margin
	if m := s.trigMode(); m != stereoTrigOff {
		if s.frozen {
			// SINGLE has its frame. Re-upload it rather than recomputing:
			// the point of a single shot is that what is on screen stops
			// changing, including in the ways a redraw would change it.
			uploadVerticesOnly(vertices, attractorDrawMode, nv)
			return
		}
		// LOCK compares a whole window, so it needs the window behind every
		// candidate offset, not just the offsets themselves.
		decim := lockDecim(span)
		need := margin + 1
		if m == stereoTrigLock {
			need = margin + lockPoints*decim + 1
		}
		trig := s.buildTrigSignal(l, r, baseL, baseR, tau, need, sr)
		var off int
		var found bool
		if m == stereoTrigLock {
			off, found = lockOffset(trig, s.lockWave, margin, decim)
		} else {
			off, found = triggerOffset(trig, margin, s.trigHoldSamples(sr),
				s.lvl, s.hyst, m == stereoTrigRising)
		}
		switch {
		case found:
			// The trigger point sits trigPos of the way into the window, so
			// a position past zero shows what LED UP TO the edge. Clamped to
			// the margin: there is only so much older audio to give back.
			pre := int(s.trigPos() * float32(span))
			if off+pre > margin {
				pre = margin - off
			}
			toff = off + pre
			if s.trigRun() == trigRunSingle {
				s.frozen = true
			}
		case s.trigRun() == trigRunNormal, s.trigRun() == trigRunSingle:
			// NORMAL holds the last frame rather than showing an untriggered
			// one, which is what makes a figure that DID hold stay up to be
			// read. SINGLE waits the same way until it catches something.
			uploadVerticesOnly(vertices, attractorDrawMode, nv)
			return
		}
	}
	if s.trigMode() == stereoTrigLock {
		// Record what this frame actually DREW — after the trigger position
		// has moved it — so next frame matches the window on screen rather
		// than one beside it, and the two do not walk apart. Recorded
		// whether it locked or free-ran, which is how it recovers after a
		// cut: one free-run frame becomes the next frame's reference.
		s.sampleLockWave(s.trigBuf, toff, lockDecim(span))
	}
	baseL += toff
	baseR += toff

	plan := stereoPlans[s.axisSel()]
	g := s.gain
	width := s.width
	// Display-only scaling: neither is in the camera fit, which is what
	// makes them scope controls rather than more of GAIN. See their
	// declarations.
	vg := s.vgain
	tspan := s.span

	// at reads axis c at source point k, clamping k to the window so the
	// spline's outer control points at either end are defined — the Takens
	// mode's `at`, with the delay moved from the coordinate index into the
	// plan, because here only some axes are delayed.
	//
	// The two channels are indexed from their own bases, which is where the
	// ALIGN offset lives, and the pair is widened before the axis is taken from
	// it so that every assignment sees the same signal.
	at := func(c, k int) float32 {
		if k < 0 {
			k = 0
		} else if k > n-1 {
			k = n - 1
		}
		i := tau + k*stride
		if plan.delay[c] {
			i -= tau
		}
		lv, rv := stereoWiden(l[baseL+i], r[baseR+i], width)
		return stereoChanValue(plan.ch[c], lv, rv)
	}
	invN := float32(1) / float32(nv-1)
	sm := takensSmooth()
	for m := 0; m < nv; m++ {
		i := m / sm
		f := float32(m%sm) / float32(sm)
		j := m * 4
		w := float32(m) * invN
		for c := 0; c < 3; c++ {
			if plan.ch[c] == chTime {
				// Straight from the vertex index, not through the spline. A
				// Catmull-Rom through equally spaced collinear points returns
				// the line — except at the two ends, where clamping makes
				// p0 == p1 and the first and last segments bow. On a signal
				// axis that is the right price for defined endpoints; on a
				// ramp it would be a visible kink in a straight edge, for a
				// value that is exactly computable.
				vertices[j+c] = (2*w - 1) * g * tspan
				continue
			}
			p0, p1, p2, p3 := at(c, i-1), at(c, i), at(c, i+1), at(c, i+2)
			// Catmull-Rom through p1..p2, as takensSmooth documents.
			vertices[j+c] = 0.5 * (2*p1 + (-p0+p2)*f +
				(2*p0-5*p1+4*p2-p3)*f*f +
				(-p0+3*p1-3*p2+p3)*f*f*f) * g * vg
		}
		vertices[j+3] = w
	}
	uploadVerticesOnly(vertices, attractorDrawMode, nv)
	// The reference lines go up after the trace, as the Poincaré overlay
	// does: a separate draw call over the finished figure.
	s.drawGraticule()

	// The RAW pair, not the widened or realigned one: the meter reports the
	// source as it is, so that ALIGN and WIDE can be turned to ask what-if
	// questions without the number moving to agree with the answer.
	corr, ok := stereoCorrelation(l, r)
	s.noteState(src.Channels() < 2, ok, corr)

	if s.fitGain != s.gain && !paramIsModulated("stereo-gain") {
		// Fitted to the FIXED scale's worst case, not to this window — see the
		// same block in generateTakens for why fitting the instantaneous
		// figure is what put loud passages off the screen. Every coordinate
		// here is bounded by gain (samples are bounded to ±1; mid and side by
		// construction; the time ramp by its own mapping), so the Takens
		// mode's √3 cube-corner extent is the right bound unchanged.
		s.fitGain = s.gain
		view.fitOverride = takensFitExtent(s.gain)
		autoFitCamera()
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
	for i := 0; i < n; i++ {
		sl += float64(l[i])
		sr += float64(r[i])
	}
	ml, mr := sl/float64(n), sr/float64(n)
	var sll, srr, slr float64
	for i := 0; i < n; i++ {
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

// stereoReadout is the LED text for a measured state. Six characters at most,
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

// stereoNoteState records this frame's measurement, updates the readout, and
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
		showAudioStatus("Mono source — both axes carry the same signal, so the figure lies on the diagonal. " +
			"The MS positions draw it as a delay embedding instead.")
	} else {
		showAudioStatus("The two channels are identical — the figure is a diagonal line. " +
			"Nothing is wrong with the display; there is no stereo information in this source.")
	}
}

// showStereoReadout writes the LED, but only when the text actually changes.
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
		s.readEl.Set("textContent", text)
	}
}

// appendStereoReadout adds the correlation cell to the Stereo parameter grid.
// Into the grid, not #params, for the reason appendTakensEstimate is: #params
// stacks below the height-bounded grid and gets clipped.
func (s *stereoInst) appendReadout(grid js.Value) {
	card, top := newPunitCard("corr")

	s.readEl = dom.Doc.Call("createElement", "span")
	s.readEl.Set("className", "led counter-led")
	s.readEl.Set("title", "Correlation between the two channels over the display window, as a goniometer's "+
		"correlation meter reads it: +1.00 means the channels are identical and the figure is the diagonal "+
		"line, 0 means they are unrelated and the figure is a round cloud, −1.00 means one is the other's "+
		"polarity inverted (and the difference disappears if the mix is summed to mono). "+
		"\"mono\" means the source has only one channel, so there is no stereo relationship to draw. "+
		"\"r --\" means silence, or one dead channel: nothing to correlate.")
	// Seeded from the last measurement, not from a placeholder: the panel is
	// rebuilt on every mode change and every module toggle, and a cell that
	// came back reading "r --" over a live stereo source would be reporting
	// silence that is not there. s.readText is cleared so the next frame
	// writes into the NEW element rather than skipping it as unchanged.
	s.readText = ""
	s.readEl.Set("textContent", stereoReadout(s.monoSrc, s.corrOK, s.corr))
	top.Call("appendChild", s.readEl)

	grid.Call("appendChild", card)
}

// ── TRIGGER ─────────────────────────────────────────────────────────────
//
// The oldest control on the instrument this resembles, and the one that
// turns a moving picture into something that can be read.
//
// Every window here ends at the newest sample, so the figure slides: a
// steady tone is redrawn each frame starting at a different phase of itself,
// and what stands still on a scope crawls here. A trigger fixes the START of
// the window to a repeatable feature of the signal instead of to "now" — the
// moment it crosses a level going up — so successive frames begin at the
// same phase and a periodic signal is drawn in the same place every time.
//
// It works on audio for exactly the reason it works on a bench: audio is
// periodic over the few milliseconds a window covers, far more often than
// not. A held note, a bass line, a drum's body, feedback, a test tone — all
// stand still. Noise and speech do not, because there is no period to lock
// to, and that is information rather than a failure: a figure that will not
// hold under a trigger is telling you it is not periodic.
//
// AUTO rather than NORMAL, in scope terms: when no crossing is found in the
// search window the draw free-runs from the newest sample instead of holding
// the last frame. A blank screen is the correct behavior on a bench, where
// the operator is hunting a fault; here it would just look broken.

// stereoTrigNames are the dial's positions.
var stereoTrigNames = []string{
	"off — the window ends at the newest sample and the figure slides",
	"rising — start where the signal crosses the level going up",
	"falling — start where it crosses going down",
	"lock — match the shape of the last frame, with no level or edge involved",
}

// stereoTrigRing is what fits around the dial.
var stereoTrigRing = []string{"off", "rise", "fall", "lock"}

const (
	stereoTrigOff = iota
	stereoTrigRising
	stereoTrigFalling
	stereoTrigLock // match the previous frame's waveform; see WAVEFORM LOCK
)

// stereoTrigMaxMargin caps how far back a trigger will look, in samples.
//
// The search costs a comparison per sample per frame and buys nothing past
// one period of the lowest thing worth locking to: 4096 samples is 85 ms at
// 48 kHz, which is a period of 11 Hz. Below that there is no pitch to hold
// still anyway.
const stereoTrigMaxMargin = 4096

// trigMode reads the dial, clamped the way axisSel is.
func (s *stereoInst) trigMode() int {
	v := s.trig
	if !(v > 0) { // false for NaN too
		return stereoTrigOff
	}
	if v > stereoTrigLock {
		return stereoTrigLock
	}
	return int(v + 0.5)
}

// trigMargin is how many samples of search the current mode wants behind
// the window. Zero when the trigger is off, which is what keeps the
// snapshot — and so the work — exactly as it was.
func (s *stereoInst) trigMargin(span int) int {
	if s.trigMode() == stereoTrigOff {
		return 0
	}
	// One window to search in, plus however much pre-trigger is asked for:
	// showing what led up to the edge means holding that much older audio,
	// and without it the clamp in generate would quietly eat the position.
	m := span + int(s.trigPos()*float32(span))
	if m > stereoTrigMaxMargin {
		return stereoTrigMaxMargin
	}
	return m
}

// triggerOffset picks where in the search margin the window starts, and
// reports whether it found a trigger at all.
//
// Offsets are added to the window base, so a LARGER offset is a NEWER
// window: 0 is the oldest start the margin allows and margin the newest,
// which is where an untriggered draw begins and where a failed search falls
// back to. The scan therefore runs DOWN from the newest and takes the first
// crossing, which is the most recent one.
//
// hyst is the noise-reject band: a crossing only counts once the signal has
// been the far side of level by at least this much, so a waveform that
// dithers across the level does not produce a trigger per dither.
//
// hold is HOLDOFF, in samples, and it is what locks onto a pattern rather
// than onto a cycle within one. A crossing qualifies only when there is no
// other crossing in the hold samples BEFORE it — so the trigger lands on the
// first edge after a quiet-enough gap rather than on whichever edge happens
// to be newest. Set near a bar and a loop stands still; set near a period
// and single cycles do. Zero is off, and is the plain most-recent-edge rule.
//
// The found flag is what separates AUTO from NORMAL upstream: auto draws
// anyway from the newest sample, normal keeps the previous frame.
func triggerOffset(trig []float32, margin, hold int, level, hyst float32, rising bool) (int, bool) {
	if margin <= 0 || len(trig) <= margin {
		return margin, false
	}
	crosses := func(off int) bool {
		if off < 1 || off > margin {
			return false
		}
		newer, older := trig[off], trig[off-1]
		if rising {
			return older < level && newer >= level
		}
		return older > level && newer <= level
	}
	armed := false
	for off := margin; off > 0; off-- {
		if rising {
			if trig[off] >= level+hyst {
				armed = true
			}
		} else if trig[off] <= level-hyst {
			armed = true
		}
		if !armed || !crosses(off) {
			continue
		}
		if hold > 0 {
			// Look back over the holdoff for another crossing. One there
			// means this edge is mid-pattern rather than the start of one.
			//
			// BACK is toward SMALLER offsets: larger is newer here, so the
			// samples before this edge in time are the ones below it. Looking
			// the other way asks whether anything follows, which every edge in
			// a burst answers the same way and which disqualifies nothing.
			quiet := true
			for b := off - 1; b >= off-hold && b >= 1; b-- {
				if crosses(b) {
					quiet = false
					break
				}
			}
			if !quiet {
				continue
			}
		}
		return off, true
	}
	return margin, false // nothing to lock to
}

// trigCoupling filters the trigger signal in place, oldest sample first.
//
// This is the "what frequency do I trigger on" control, and on a scope it is
// spelled as coupling rather than as a frequency: the trigger has its own
// path, and narrowing what reaches it is how a stable lock is found in
// material that has too much going on.
//
//   - DC passes everything, which is right for a clean tone.
//   - LF REJECT high-passes, so a bass line or DC offset stops dragging the
//     crossing around and the trigger locks to the part you are looking at.
//   - HF REJECT low-passes, so cymbals and hiss stop producing crossings of
//     their own and the trigger follows the fundamental.
//
// One-pole each, which is what a scope's trigger path is: the point is to
// tilt the balance, not to build a filter anyone would listen through.
func trigCoupling(buf []float32, mode int, sr int) {
	if len(buf) == 0 || sr <= 0 || mode == trigCplDC {
		return
	}
	switch mode {
	case trigCplLFRej:
		// ~100 Hz high-pass: below a bass note's fundamental.
		a := rcAlpha(100, sr)
		var prevIn, prevOut float32
		for i, x := range buf {
			y := a * (prevOut + x - prevIn)
			prevIn, prevOut = x, y
			buf[i] = y
		}
	case trigCplHFRej:
		// ~2 kHz low-pass: above most fundamentals, below most hiss.
		a := rcAlpha(2000, sr)
		var prevOut float32
		for i, x := range buf {
			prevOut += (1 - a) * (x - prevOut)
			buf[i] = prevOut
		}
	}
}

// rcAlpha is the one-pole coefficient for a corner at hz.
func rcAlpha(hz float32, sr int) float32 {
	rc := 1 / (2 * math.Pi * float64(hz))
	dt := 1 / float64(sr)
	return float32(rc / (rc + dt))
}

// The rest of the trigger section: the controls a bench scope puts beside
// LEVEL and SLOPE, which are the ones that decide whether a lock can be
// found at all in material with more than one thing in it.

const (
	trigSrcMid = iota // the sum: what both axes are built from
	trigSrcL
	trigSrcR
)

var trigSrcNames = []string{
	"mid — the sum of the pair, which is what both axes are built from",
	"left — lock to the left channel alone",
	"right — lock to the right channel alone",
}

var trigSrcRing = []string{"mid", "L", "R"}

const (
	trigCplDC = iota // everything reaches the trigger
	trigCplLFRej
	trigCplHFRej
)

var trigCplNames = []string{
	"DC — the whole signal reaches the trigger",
	"LF reject — high-passed, so bass and offset stop dragging the crossing",
	"HF reject — low-passed, so hiss and cymbals stop triggering on themselves",
}

var trigCplRing = []string{"DC", "LFr", "HFr"}

// clampSel reads a knob that is a rotary switch, clamped to 0..last.
func clampSel(v float32, last int) int {
	if !(v > 0) { // false for NaN too
		return 0
	}
	if v > float32(last) {
		return last
	}
	return int(v + 0.5)
}

func (s *stereoInst) trigSrc() int { return clampSel(s.tsrc, trigSrcR) }
func (s *stereoInst) trigCpl() int { return clampSel(s.tcpl, trigCplHFRej) }

// buildTrigSignal fills the trigger path for this frame: the chosen source
// over the search margin, oldest sample first, coupled.
//
// Its own buffer and its own filter run, rather than reading the drawn axes:
// the trigger is allowed to look at a different signal from the one on
// screen, and on a scope that is the whole point of having a trigger source
// and a coupling at all.
func (s *stereoInst) buildTrigSignal(l, r []float32, baseL, baseR, tau, n, sr int) []float32 {
	if n <= 0 {
		return nil
	}
	if cap(s.trigBuf) < n {
		s.trigBuf = make([]float32, n+n/2)
	}
	buf := s.trigBuf[:n]
	src := s.trigSrc()
	// Clamped: the lock asks for a window beyond the search margin, and a
	// snapshot one sample short of it must repeat its oldest sample rather
	// than index past the end.
	last := len(l) - 1
	if m := len(r) - 1; m < last {
		last = m
	}
	for off := 0; off < n; off++ {
		i := tau + off
		if baseL+i > last {
			i = last - baseL
		}
		if baseR+i > last {
			i = last - baseR
		}
		if i < 0 {
			i = 0
		}
		switch src {
		case trigSrcL:
			buf[off] = l[baseL+i]
		case trigSrcR:
			buf[off] = r[baseR+i]
		default:
			buf[off] = (l[baseL+i] + r[baseR+i]) * 0.5
		}
	}
	trigCoupling(buf, s.trigCpl(), sr)
	return buf
}

// ── the rest of a trigger section ───────────────────────────────────────

const (
	trigRunAuto   = iota // draw anyway when nothing triggers
	trigRunNormal        // hold the last frame until something does
	trigRunSingle        // catch one and freeze; move the knob to re-arm
)

var trigRunNames = []string{
	"auto — draw anyway when nothing triggers, so the display never goes blank",
	"normal — hold the last triggered frame until the next trigger",
	"single — catch the next trigger and freeze; move this knob to re-arm",
}

var trigRunRing = []string{"auto", "norm", "sgl"}

// trigRun reads the run-mode dial.
//
// AUTO, NORMAL and SINGLE are the three a bench scope has, and they differ
// only in what happens when there is NO trigger — which is the whole
// question on program material, where a lock comes and goes with the
// music. Auto keeps drawing so the display is never blank; normal keeps
// the last locked frame, so a figure that did hold stays up to be read
// instead of dissolving the moment the signal changes; single catches one
// and stops, which is the only way to look at a transient properly.
func (s *stereoInst) trigRun() int { return clampSel(s.trun, trigRunSingle) }

// trigHoldSamples is the holdoff knob in samples at the live rate. The knob
// is in milliseconds, for takens.TauSamples' reason: a sample is not a duration,
// and a holdoff that meant different things on the microphone and the feed
// would lock to different things on each.
func (s *stereoInst) trigHoldSamples(sr int) int {
	if !(s.hold > 0) {
		return 0
	}
	n := int(s.hold * float32(sr) / 1000)
	if n < 1 {
		return 1
	}
	return n
}

// trigPos is where the trigger point sits IN the window, 0 at the left edge
// and 1 at the right.
//
// A scope calls this the horizontal position, and it is how you see what
// LED UP TO an event rather than only what followed it: put the trigger at
// 0.5 and half the window is pre-trigger. Here it is a shift of the window
// start, which is why it costs another half window of margin at 1.
func (s *stereoInst) trigPos() float32 {
	if s.tpos < 0 {
		return 0
	}
	if s.tpos > 1 {
		return 1
	}
	return s.tpos
}

// rearmIfModeMoved re-arms SINGLE when the run dial is touched, which is
// how a scope's single-shot is re-armed: there is no separate button here,
// and adding one for a mode that is not the usual one is a cell of panel
// for a click a year.
func (s *stereoInst) rearmIfModeMoved() {
	if m := s.trigRun(); m != s.lastRun {
		s.lastRun = m
		s.frozen = false
	}
}

// ── WAVEFORM LOCK ───────────────────────────────────────────────────────
//
// An edge trigger locks to ONE feature: the moment the signal crosses a
// level. That is the right thing on a bench, where the signal is a square
// wave and every cycle is the same. On music it is the weak link — a
// polyphonic passage crosses any level many times per period, at moments
// that are not the same feature from one cycle to the next, and the figure
// settles onto whichever crossing happened to be newest.
//
// This locks to the SHAPE instead. Keep the stretch of signal the last
// frame drew, slide it along the search margin, and start where it matches
// best. Nothing has to cross anything, there is no level to set, and what
// holds still is the whole waveform rather than one point on it.
//
// It is what a correlation-based pitch detector does with the answer thrown
// away: the offset IS the answer, and a figure drawn from it stands where
// the previous one did. On material an edge trigger cannot hold — chords,
// speech, anything with several partials — this is the one that works.
//
// The cost is a search, so both sides are decimated: the comparison runs on
// every lockDecim'th sample and the candidate offsets step by the same, for
// roughly (margin/decim) × (span/decim) multiplies a frame. At the default
// window that is a few hundred thousand, which is small beside the spline
// the frame is about to run anyway.

// lockPoints is how many samples the comparison uses. 192 is enough to
// identify a waveform and few enough that the search stays cheap; past it
// the extra points agree with the ones already counted.
const lockPoints = 192

// lockBias is how much better an OLDER candidate must score before it is
// preferred to a newer one, on a scale where 1 is an exact match.
//
// 0.02 is enough to reject the identical-content match that would freeze
// the display and small enough that a genuinely better alignment still
// wins. Zero reproduces the freeze; large values stop it locking at all.
const lockBias = 0.02

// lockStep is how far apart the candidate offsets are, in samples. Every
// offset would be exact and four times the work for a shift nobody can
// see: a quarter-sample of jitter at 48 kHz is 20 microseconds.
const lockStep = 4

// lockOffset finds where in the margin the current signal best matches what
// was drawn last frame.
//
// The score is a normalized dot product, so it compares SHAPE and not
// loudness: a passage that swells must not drag the lock along with it.
// Returns the free-run offset and false when there is no previous frame to
// match, which is what the first frame after a mode change gets.
func lockOffset(cur, prev []float32, margin, decim int) (int, bool) {
	if len(prev) == 0 || margin <= 0 || decim < 1 {
		return margin, false
	}
	n := len(prev)
	if len(cur) < margin+n*decim {
		return margin, false
	}
	var prevNorm float64
	for _, v := range prev {
		prevNorm += float64(v) * float64(v)
	}
	if prevNorm < 1e-9 {
		return margin, false // silence last frame: nothing to match
	}
	prevNorm = math.Sqrt(prevNorm)

	// Scanned NEWEST first, and an older candidate has to be clearly better
	// to displace a newer one. That bias is not a refinement, it is the
	// difference between a lock and a freeze.
	//
	// Without it the match is on CONTENT rather than on PHASE. The audio
	// advances between frames, so last frame's window is now sitting further
	// back in the buffer, and the best possible match is that same content
	// exactly — score 1.0, every frame, until it scrolls out of the margin
	// and the lock jumps to the next one. The display then holds still for a
	// while and stutters, and what it is showing is steadily older audio
	// rather than what is playing.
	//
	// A periodic signal matches nearly as well one period from the newest
	// sample as it does against its own past self, so requiring a clear
	// improvement makes the NEWEST acceptable match win. The figure is then
	// phase-aligned and at most a period old, which is what a trigger is
	// supposed to give.
	best, bestOff := -2.0, margin
	for off := margin; off >= 0; off -= lockStep {
		var dot, curNorm float64
		for i := 0; i < n; i++ {
			c := float64(cur[off+i*decim])
			dot += c * float64(prev[i])
			curNorm += c * c
		}
		if curNorm < 1e-9 {
			continue
		}
		score := dot / (math.Sqrt(curNorm) * prevNorm)
		if score > best+lockBias {
			best, bestOff = score, off
		}
	}
	if best < 0 {
		// Nothing resembles the last frame — a cut, a new section. Free-run
		// rather than locking to the least bad of a bad set.
		return margin, false
	}
	return bestOff, true
}

// sampleLockWave records the stretch the window just drew, decimated, as
// next frame's thing to match.
func (s *stereoInst) sampleLockWave(trig []float32, off, decim int) {
	if cap(s.lockWave) < lockPoints {
		s.lockWave = make([]float32, lockPoints)
	}
	w := s.lockWave[:lockPoints]
	for i := range w {
		j := off + i*decim
		if j >= len(trig) {
			j = len(trig) - 1
		}
		w[i] = trig[j]
	}
	s.lockWave = w
}

// lockDecim spreads lockPoints over the window, so the comparison covers
// what is drawn rather than a sliver at the start of it.
func lockDecim(span int) int {
	d := span / lockPoints
	if d < 1 {
		d = 1
	}
	return d
}

// ── GRATICULE ───────────────────────────────────────────────────────────
//
// The lines a goniometer is read against. Without them the figure is a
// shape in the dark and its angle has to be judged by eye, which is the
// one thing the instrument is for.
//
// Which lines mean what depends on the axes, so this follows the dial:
//
//	L/R positions — x is L and y is R. The DIAGONALS carry the meaning:
//	  x=y is in-phase material, which is what a centered mono signal draws,
//	  and x=−y is out of phase, the content that vanishes when summed. The
//	  axes themselves are hard left and hard right.
//
//	mid/side positions — x is mid and y is side. Now the AXES carry it:
//	  side=0 is mono, and mid=0 is entirely out of phase. The diagonals are
//	  hard left and hard right, the two being swapped relative to L/R.
//
// Drawn at z=0, in the plane the vector part of the figure lives in, and
// scaled to the same extent the signal axes are: gain times the vertical
// gain, so the lines stay with the trace when either is turned.
const (
	gratOff = iota
	gratAxes
	gratFull
)

var gratNames = []string{
	"off — no reference lines",
	"axes — the two coordinate axes only",
	"full — axes, diagonals and the unit box",
}

var gratRing = []string{"off", "ax", "full"}

func (s *stereoInst) gratMode() int { return clampSel(s.grat, gratFull) }

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
	m := s.gratMode()
	if m == gratOff {
		return
	}
	e := s.gain * s.vgain
	if !(e > 0) {
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
	line(-e, 0, e, 0)
	line(0, -e, 0, e)
	if m == gratFull {
		line(-e, -e, e, e)
		line(-e, e, e, -e)
		line(-e, -e, e, -e)
		line(e, -e, e, e)
		line(e, e, -e, e)
		line(-e, e, -e, -e)
	}
	n := len(v) / 4

	glctx.GL.Call("uniform1i", uGradientColorsLoc, 1)
	glctx.GL.Call("uniform3f", uBaseColorLoc, 0.22, 0.26, 0.32)
	uploadVerticesOnly(v, glctx.Types.Lines, n)
	if phosphorActive() {
		// The phosphor owns these two while it is on; handing them to the
		// palette here would hand them to the wrong owner. sectTick says
		// the same thing about the same pair.
		applyPhosphorColor()
		return
	}
	glctx.GL.Call("uniform1i", uGradientColorsLoc, gradientColorsUniform())
	glctx.GL.Call("uniform3f", uBaseColorLoc, baseColor[0], baseColor[1], baseColor[2])
}

// field resolves a parameter id to the field it names ON THIS INSTANCE.
//
// The parameter table binds a knob to a field address when the row is
// built, which is right for a knob and wrong for a sweep: a sweep has to
// write the same parameter on a different cell's instance every pass, and
// the table's pointer names only the focused one. This is the indirection
// that makes "the same parameter, on that cell" expressible.
//
// Returns nil for an id this mode does not have, which is how a sweep
// pointed at another mode's parameter does nothing rather than something
// wrong.
func (s *stereoInst) field(id string) *float32 {
	switch id {
	case "stereo-axes":
		return &s.axesF
	case "stereo-tau":
		return &s.tau
	case "stereo-win":
		return &s.win
	case "stereo-gain":
		return &s.gain
	case "stereo-align":
		return &s.align
	case "stereo-width":
		return &s.width
	case "stereo-vg":
		return &s.vgain
	case "stereo-span":
		return &s.span
	case "stereo-trig":
		return &s.trig
	case "stereo-lvl":
		return &s.lvl
	case "stereo-hyst":
		return &s.hyst
	case "stereo-hold":
		return &s.hold
	case "stereo-tpos":
		return &s.tpos
	case "stereo-grat":
		return &s.grat
	}
	return nil
}
