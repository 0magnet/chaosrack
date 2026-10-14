package attractor

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
	"github.com/0magnet/chaosrack/pkg/takens"
)

// The Stereo Embedding's frame, apart from drawing it: its knobs, its axes,
// its window and its trigger. The page's stereoInst draws a StereoScope into
// the GPU's vertex buffer; the native rack draws one of its own.

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

// StereoScope is one Stereo Embedding's knobs and the trigger's memory
// between frames.
type StereoScope struct {
	axesF float32 // axes knob: an index into stereoPlans
	tau   float32 // delay τ, reference samples (delay positions only)
	win   float32 // display window, milliseconds
	gain  float32 // world units a full-scale (±1) sample maps to

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

	// start is where the last drawn window began past the snapshot's base:
	// the search margin when the trigger is off, and where it moved the
	// window when it is on.
	start int
}

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
// where you are in the window, not of the sample there, so stereoInst.generate
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
func (s *StereoScope) axisSel() int {
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
func stereoWindow(winMS float32, sampleRate, budget, tau, align, smooth int) (n, stride int) {
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
	return takens.Window(winMS, sr, budget, smooth)
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
func (s *StereoScope) trigMode() int {
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
func (s *StereoScope) trigMargin(span int) int {
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

const (
	trigCplDC = iota // everything reaches the trigger
	trigCplLFRej
	trigCplHFRej
)

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

func (s *StereoScope) trigSrc() int { return clampSel(s.tsrc, trigSrcR) }
func (s *StereoScope) trigCpl() int { return clampSel(s.tcpl, trigCplHFRej) }

// buildTrigSignal fills the trigger path for this frame: the chosen source
// over the search margin, oldest sample first, coupled.
//
// Its own buffer and its own filter run, rather than reading the drawn axes:
// the trigger is allowed to look at a different signal from the one on
// screen, and on a scope that is the whole point of having a trigger source
// and a coupling at all.
func (s *StereoScope) buildTrigSignal(l, r []float32, baseL, baseR, tau, n, sr int) []float32 {
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
	for off := range n {
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

// trigRun reads the run-mode dial.
//
// AUTO, NORMAL and SINGLE are the three a bench scope has, and they differ
// only in what happens when there is NO trigger — which is the whole
// question on program material, where a lock comes and goes with the
// music. Auto keeps drawing so the display is never blank; normal keeps
// the last locked frame, so a figure that did hold stays up to be read
// instead of dissolving the moment the signal changes; single catches one
// and stops, which is the only way to look at a transient properly.
func (s *StereoScope) trigRun() int { return clampSel(s.trun, trigRunSingle) }

// trigHoldSamples is the holdoff knob in samples at the live rate. The knob
// is in milliseconds, for takens.TauSamples' reason: a sample is not a duration,
// and a holdoff that meant different things on the microphone and the feed
// would lock to different things on each.
func (s *StereoScope) trigHoldSamples(sr int) int {
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
func (s *StereoScope) trigPos() float32 {
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
func (s *StereoScope) rearmIfModeMoved() {
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
		for i := range n {
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
func (s *StereoScope) sampleLockWave(trig []float32, off, decim int) {
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
	d := max(span/lockPoints, 1)
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

func (s *StereoScope) gratMode() int { return clampSel(s.grat, gratFull) }

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
func (s *StereoScope) field(id string) *float32 {
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
	case "stereo-tsrc":
		return &s.tsrc
	case "stereo-tcpl":
		return &s.tcpl
	case "stereo-trun":
		return &s.trun
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

// NewStereoScope returns a scope at the defaults the knobs reset to.
func NewStereoScope() *StereoScope {
	return &StereoScope{
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

// stereoFrame is one frame's arithmetic at the scope's knobs: the delay, the
// offset between the channels, the window, and how much snapshot they and
// the trigger need.
type stereoFrame struct {
	tau, align, n, stride, span int
	// baseL and baseR are where each channel's run starts in the snapshot.
	// The two channels are read from indices `align` apart, so the snapshot
	// has to cover both runs: |align| more samples, with the earlier channel
	// starting at 0 and the later one offset into them. Choosing them as
	// max(±align, 0) keeps both inside [0, |align|] whichever way round the
	// offset goes.
	baseL, baseR int
	margin       int // trigger search behind the window
	snap         int // samples of each channel the frame reads
	nv           int // vertices it draws
}

func (s *StereoScope) frame(sr, budget, smooth int) stereoFrame {
	f := stereoFrame{
		tau:   takens.TauSamples(s.tau, sr),
		align: stereoAlignSamples(s.align, sr),
	}
	f.n, f.stride = stereoWindow(s.win, sr, budget, f.tau, f.align, smooth)
	f.span = (f.n-1)*f.stride + f.tau
	absA := f.align
	if absA < 0 {
		absA = -absA
	}
	if f.align > 0 {
		f.baseL = f.align // right is read EARLIER, i.e. delayed into line with left
	} else {
		f.baseR = -f.align
	}
	// The trigger searches BEHIND the window, so the snapshot carries a margin
	// of older audio for it to look in. Zero when the trigger is off, which
	// leaves the snapshot exactly the size it always was.
	f.margin = s.trigMargin(f.span)
	// The waveform lock compares a whole window at every candidate offset,
	// so the snapshot has to reach further back than the search margin
	// alone would need.
	lockPad := 0
	if s.trigMode() == stereoTrigLock {
		lockPad = lockPoints*lockDecim(f.span) + 1
	}
	f.snap = f.span + absA + 1 + f.margin + lockPad
	f.nv = takens.Verts(f.n, smooth)
	return f
}

// Window is the frame the scope draws at sample rate sr, a point budget and
// smooth spline steps a sample: how many samples of each channel it reads,
// and how many vertices it draws.
func (s *StereoScope) Window(sr, budget, smooth int) (snap, verts int) {
	f := s.frame(sr, budget, smooth)
	return f.snap, f.nv
}

// Frame draws the newest window of l and r, the snapshot Window asked for
// (oldest first), into vertices: four floats a vertex, x, y and z and the
// trail position 0..1. It reports false, leaving vertices as they were,
// when the trigger holds the last frame instead: SINGLE has caught one, or
// NORMAL found nothing.
func (s *StereoScope) Frame(l, r []float32, sr, budget, smooth int, vertices []float32) bool {
	f := s.frame(sr, budget, smooth)
	n, stride, tau, span, margin := f.n, f.stride, f.tau, f.span, f.margin

	// Where in the margin this frame starts. margin means "the newest
	// possible window", which is what an untriggered draw has always used.
	s.rearmIfModeMoved()
	toff := margin
	if m := s.trigMode(); m != stereoTrigOff {
		if s.frozen {
			// SINGLE has its frame. The caller redraws it rather than
			// recomputing: the point of a single shot is that what is on
			// screen stops changing, including in the ways a redraw would
			// change it.
			return false
		}
		// LOCK compares a whole window, so it needs the window behind every
		// candidate offset, not just the offsets themselves.
		decim := lockDecim(span)
		need := margin + 1
		if m == stereoTrigLock {
			need = margin + lockPoints*decim + 1
		}
		trig := s.buildTrigSignal(l, r, f.baseL, f.baseR, tau, need, sr)
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
			return false
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
	s.start = toff
	baseL, baseR := f.baseL+toff, f.baseR+toff

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
	nv := f.nv
	invN := float32(1) / float32(nv-1)
	for m := range nv {
		i := m / smooth
		t := float32(m%smooth) / float32(smooth)
		j := m * 4
		w := float32(m) * invN
		for c := range 3 {
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
			vertices[j+c] = 0.5 * (2*p1 + (-p0+p2)*t +
				(2*p0-5*p1+4*p2-p3)*t*t +
				(-p0+3*p1-3*p2+p3)*t*t*t) * g * vg
		}
		vertices[j+3] = w
	}
	return true
}

// Set turns knob id, one of the stereo-* parameters, to v, and reports
// whether the scope has it.
func (s *StereoScope) Set(id string, v float32) bool {
	f := s.field(id)
	if f != nil {
		*f = v
	}
	return f != nil
}

// gratLines draws the graticule a line at a time, at the extent the signal
// axes are scaled to: gain times the vertical gain, so the lines stay with
// the trace when either is turned. Nothing when it is off.
func (s *StereoScope) gratLines(line func(x0, y0, x1, y1 float32)) {
	m, e := s.gratMode(), s.gain*s.vgain
	if m == gratOff || !(e > 0) {
		return
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
}

// GraticuleColor is the graticule's flat gray, dim beside any trace.
var GraticuleColor = [3]float32{0.22, 0.26, 0.32}

// Graticule is the lines the figure is read against, as a figure of lines:
// empty when the GRID knob is off.
func (s *StereoScope) Graticule() Figure {
	f := Figure{Kind: FigureLines}
	var k uint16
	s.gratLines(func(x0, y0, x1, y1 float32) {
		f.Points = append(f.Points, [3]float64{float64(x0), float64(y0), 0}, [3]float64{float64(x1), float64(y1), 0})
		f.Edges = append(f.Edges, k, k+1)
		k += 2
	})
	return f
}

// StereoKnobs are the Stereo Embedding's own knobs, each one Set takes. Its
// smoothing is takens-smooth, which it shares with the Takens Embedding.
var StereoKnobs = []string{
	"stereo-axes", "stereo-tau", "stereo-win", "stereo-gain", "stereo-align",
	"stereo-width", "stereo-vg", "stereo-span", "stereo-trig", "stereo-lvl",
	"stereo-tsrc", "stereo-tcpl", "stereo-hyst", "stereo-trun", "stereo-hold",
	"stereo-tpos", "stereo-grat",
}
