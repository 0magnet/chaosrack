//go:build js && wasm

package attractor

import (
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
var stereoAxisNames = []string{
	"L, R, L(t−τ)",
	"L, R, time",
	"mid, side, mid(t−τ)",
	"mid, side, time",
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
var stereoAxisRing = []string{"LRd", "LRt", "MSd", "MSt"}

var (
	stereoAxesF float32 = 0  // axes knob: an index into stereoPlans
	stereoTau   float32 = 32 // delay τ, in source samples (delay positions only)
	stereoWin   float32 = 85 // display window, milliseconds
	stereoGain  float32 = 10 // world units a full-scale (±1) sample maps to

	stereoL, stereoR []float32 // this frame's snapshot, oldest first
	stereoFitted     bool      // camera fitted since real audio arrived
)

func init() {
	registerGenerate("stereo", generateStereo)
	attractorParams["stereo"] = []paramDef{
		{"stereo-axes", "axes", &stereoAxesF, 0, 0, float32(len(stereoPlans) - 1), 1},
		{"stereo-tau", "τ", &stereoTau, 32, 1, 512, 1},
		{"stereo-win", "win", &stereoWin, 85, 5, stereoWinMax, 5},
		{"stereo-gain", "gain", &stereoGain, 10, 0.5, 50, 0.5},
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
func stereoAxisSel() int {
	last := len(stereoPlans) - 1
	v := stereoAxesF
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
func stereoWindow(winMS float32, sampleRate, budget, tau int) (n, stride int) {
	sr := sampleRate
	if sr <= 0 {
		sr = 24000
	}
	if maxMS := float32(stereoSpanMax-tau-1) / float32(sr) * 1000; winMS > maxMS {
		winMS = maxMS
	}
	return takensWindow(winMS, sr, budget)
}

// generateStereo snapshots both channels and draws the newest window as a
// trail through the normal 3D pipeline.
func generateStereo() {
	src := ensureAudioSource()
	tau := int(stereoTau)
	if tau < 1 {
		tau = 1
	}
	sr := 24000
	if src != nil && src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	n, stride := stereoWindow(stereoWin, sr, steps, tau)
	span := (n-1)*stride + tau
	if need := span + 1; len(stereoL) < need {
		// Grown by half again, as the Takens ring is, so that turning the WIN
		// knob does not reallocate on every step of the dial.
		stereoL = make([]float32, need+need/2)
		stereoR = make([]float32, len(stereoL))
	}
	nv := takensVerts(n)
	vertices := vertBuf[:nv*4]
	if src == nil || !src.Ready() {
		// Re-upload the previous frame rather than a cleared buffer, so the
		// model does not flicker while the source spins up — and refit when
		// audio arrives, since the mode-entry fit saw whatever was here.
		stereoFitted = false
		stereoNoteState(false, false, 0)
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
	l, r := stereoL[:span+1], stereoR[:span+1]
	src.TimeDomainStereo(l, r)

	plan := stereoPlans[stereoAxisSel()]
	g := stereoGain

	// at reads axis c at source point k, clamping k to the window so the
	// spline's outer control points at either end are defined — the Takens
	// mode's `at`, with the delay moved from the coordinate index into the
	// plan, because here only some axes are delayed.
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
		return stereoChanValue(plan.ch[c], l[i], r[i])
	}
	invN := float32(1) / float32(nv-1)
	for m := 0; m < nv; m++ {
		i := m / takensSmooth
		f := float32(m%takensSmooth) / takensSmooth
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
				vertices[j+c] = (2*w - 1) * g
				continue
			}
			p0, p1, p2, p3 := at(c, i-1), at(c, i), at(c, i+1), at(c, i+2)
			// Catmull-Rom through p1..p2, as takensSmooth documents.
			vertices[j+c] = 0.5 * (2*p1 + (-p0+p2)*f +
				(2*p0-5*p1+4*p2-p3)*f*f +
				(-p0+3*p1-3*p2+p3)*f*f*f) * g
		}
		vertices[j+3] = w
	}
	uploadVerticesOnly(vertices, attractorDrawMode, nv)

	corr, ok := stereoCorrelation(l, r)
	stereoNoteState(src.Channels() < 2, ok, corr)

	if !stereoFitted {
		// Fitted to the FIXED scale's worst case, not to this window — see the
		// same block in generateTakens for why fitting the instantaneous
		// figure is what put loud passages off the screen. Every coordinate
		// here is bounded by gain (samples are bounded to ±1; mid and side by
		// construction; the time ramp by its own mapping), so the Takens
		// mode's √3 cube-corner extent is the right bound unchanged.
		stereoFitted = true
		fitExtentOverride = takensFitExtent(stereoGain)
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

var (
	stereoReadEl   js.Value // the readout in the parameter grid
	stereoReadText string   // last text written to it (DOM write only on change)

	stereoMonoSrc bool    // the source itself has one channel
	stereoCorrOK  bool    // there is enough signal for a correlation to mean anything
	stereoCorr    float32 // Pearson r between the channels, −1..+1

	// stereoCollapsed counts CONSECUTIVE frames in which the two axes carry
	// the same signal. A count rather than a flag because music is mono for a
	// bar at a time all the time — a solo instrument panned center, a fade to
	// a single voice — and a notice that fired on that would be noise. A
	// second of it is a property of the source, not of the passage.
	stereoCollapsed int
	stereoNoticed   bool // the notice has been shown once since audio started
)

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
func stereoNoteState(monoSrc, ok bool, corr float32) {
	stereoMonoSrc, stereoCorrOK, stereoCorr = monoSrc, ok, corr
	showStereoReadout(stereoReadout(monoSrc, ok, corr))

	if !stereoIsCollapsed(monoSrc, ok, corr) {
		// Re-armed, so a source that is swapped for a mono one later in the
		// session is reported too. It cannot become chatter: re-arming costs a
		// non-collapsed frame and the notice costs a further second of
		// collapse, and showAudioStatus suppresses a message identical to the
		// one it last showed anyway.
		stereoCollapsed, stereoNoticed = 0, false
		return
	}
	if stereoCollapsed < stereoNoticeFrames {
		// Stops at the threshold rather than counting on: this runs every
		// frame for as long as the mode is up, and the number past the
		// threshold means nothing to anyone.
		stereoCollapsed++
	}
	if stereoNoticed || stereoCollapsed < stereoNoticeFrames {
		return
	}
	stereoNoticed = true
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
func showStereoReadout(s string) {
	if s == stereoReadText {
		return
	}
	stereoReadText = s
	if stereoReadEl.Truthy() {
		stereoReadEl.Set("textContent", s)
	}
}

// appendStereoReadout adds the correlation cell to the Stereo parameter grid.
// Into the grid, not #params, for the reason appendTakensEstimate is: #params
// stacks below the height-bounded grid and gets clipped.
func appendStereoReadout(grid js.Value) {
	card := doc.Call("createElement", "div")
	card.Set("className", "punit")

	lbl := doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", false))
	lbl.Set("textContent", "corr")
	card.Call("appendChild", lbl)

	stereoReadEl = doc.Call("createElement", "span")
	stereoReadEl.Set("className", "led counter-led")
	stereoReadEl.Set("title", "Correlation between the two channels over the display window, as a goniometer's "+
		"correlation meter reads it: +1.00 means the channels are identical and the figure is the diagonal "+
		"line, 0 means they are unrelated and the figure is a round cloud, −1.00 means one is the other's "+
		"polarity inverted (and the difference disappears if the mix is summed to mono). "+
		"\"mono\" means the source has only one channel, so there is no stereo relationship to draw. "+
		"\"r --\" means silence, or one dead channel: nothing to correlate.")
	// Seeded from the last measurement, not from a placeholder: the panel is
	// rebuilt on every mode change and every module toggle, and a cell that
	// came back reading "r --" over a live stereo source would be reporting
	// silence that is not there. stereoReadText is cleared so the next frame
	// writes into the NEW element rather than skipping it as unchanged.
	stereoReadText = ""
	stereoReadEl.Set("textContent", stereoReadout(stereoMonoSrc, stereoCorrOK, stereoCorr))
	card.Call("appendChild", stereoReadEl)

	grid.Call("appendChild", card)
}
