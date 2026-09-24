//go:build js && wasm

package attractor

import "github.com/0magnet/chaosrack/pkg/audiosrc"

// audioTap is the audio fan-out: the ring every consumer reads from and the
// source that fills it.
type audioTap struct {
	// Audio fan-out. Source.Drain hands each sample to its caller EXACTLY ONCE —
	// that is the contract the overlapping STFT needs, and it is stated that way in
	// the interface. It also means two callers on one source do not each see the
	// stream: they split it.
	//
	// That is a real defect, not a theoretical one. The spectrogram backdrop and
	// the Takens embedding both used to call Drain, and the backdrop is painted
	// first (renderBackgroundVisual runs before the model is generated), so the
	// spectrogram — which drains until its accumulator is full, i.e. everything
	// available — took every sample and Takens got nothing. Its ring stopped
	// advancing, the window it draws stopped moving, and the attractor sat frozen
	// on screen while the backdrop scrolled happily behind it. Turning the backdrop
	// off unfroze it. The same collision exists for every other pair among the
	// frame-loop consumers (recurrence and the frequency counter drain too); the
	// spectrogram/Takens pair was merely the one with an obvious symptom.
	//
	// The fix is to drain ONCE per frame, here, and give every consumer its own
	// read cursor into what was drained. Each consumer then sees the whole stream,
	// which is what each of them was written to assume.
	//
	// This is the same shape as the existing fvf.vis ring, which was added for the
	// same reason: when FVF is on it drains the source in the audio callback, so
	// the spectrogram reads fvf.vis rather than draining a second time. That case
	// stays as it is — it is a different clock, not a frame-loop consumer.
	//
	// THE TAP CARRIES BOTH CHANNELS. It used to carry one — Source.Drain's, which
	// is the primary channel folded — and every consumer got the same mono stream
	// whether or not that was the signal it wanted. A consumer that wants the left
	// channel, or mid, or side cannot get there from a stream already folded, so
	// "which channel" was a question only the two modes that snapshot could answer
	// and the two that accumulate could not.
	//
	// So the tap drains stereo and keeps a ring per channel, and the fold is a
	// CONSUMER'S choice made on read (tapChan) rather than the source's made on
	// write. A mono source writes the same samples into both rings, which is what
	// Source.DrainStereo already promises, so nothing downstream has to special-case
	// it — asking for "side" of a mono source correctly gives silence.
	ringL    []float32
	ringR    []float32
	w        int // monotonic count of samples ever written into the rings
	scratch  []float32
	scratchR []float32
	src      audiosrc.Source // source the cursors below are relative to
	upstream tapUpstreamKind
}

var tap audioTap

// tapChan names the signal a consumer reads out of the tap. The first three
// are a fold of one (L, R) pair; they are the Stereo Embedding's basis choices
// under the same names, because they are the same four signals.
type tapChan uint8

const (
	tapMix  tapChan = iota // (L+R)/2 — what is playing, and what Drain used to give
	tapLeft                // one channel as it is
	tapRight
	tapMid  // (L+R)/2 by construction; the same as the mix, and named for the pair
	tapSide // (L−R)/2 — what is NOT common to the two, and silence on a mono source
)

// tapChanNames are the positions of any knob that selects one, and tapChanRing
// what fits around such a dial.
var (
	tapChanNames = []string{"mix", "left", "right", "mid", "side"}
	tapChanRing  = []string{"mix", "L", "R", "M", "S"}
)

// tapChanDescs say what each channel IS, one per position, and are what a
// channel knob's dial label carries as its tooltip. Three parallel slices, so a
// channel cannot acquire a detent without a sentence explaining it.
var tapChanDescs = []string{
	"mix — the two channels summed: what a mono meter would read",
	"left — the left channel alone",
	"right — the right channel alone",
	"mid — the sum, halved: what both channels agree on, and what a mono listener hears",
	"side — the difference, halved: what the two channels disagree about, which is the stereo width itself",
}

// tapFold reduces one (L, R) pair to the signal a channel names. mid carries
// the ½ and side carries it too, so that switching between them does not resize
// the figure and a full-scale input stays inside ±1 — stereoChanValue's
// argument, and the bound every fixed-scale camera fit in this package relies
// on.
func tapFold(c tapChan, l, r float32) float32 {
	switch c {
	case tapLeft:
		return l
	case tapRight:
		return r
	case tapSide:
		return (l - r) * 0.5
	default: // tapMix, tapMid
		return (l + r) * 0.5
	}
}

// tapChanSel clamps a knob value to a channel. Audio modulation can drive any
// registered parameter, so the value arriving is not necessarily a detent, and
// the range is checked BEFORE the conversion because a float-to-int conversion
// whose value does not fit is implementation-defined in Go (stereoAxisSel's
// argument, and its trap).
func tapChanSel(v float32) tapChan {
	if !(v > 0) { // false for NaN too
		return tapMix
	}
	if last := float32(len(tapChanNames) - 1); v > last {
		v = last
	}
	// Clamped to the name table just above, so the value is 0..4 — gosec sees
	// only int → uint8 and cannot see the clamp.
	return tapChan(int(v + 0.5)) //nolint:gosec // clamped to len(tapChanNames)-1 above
}

// tapDrainCap bounds one frame's pull. It is not optional: FuncGen synthesizes
// on demand and always fills the buffer it is handed, so "drain until a partial
// fill" never terminates against it — the loop that did allocated until the
// wasm heap died.
const tapDrainCap = 16384

// tapRingSize holds well over a frame's worth at any sample rate, so a consumer
// that runs once per frame never misses samples. One that stops reading (its
// model is not selected) falls behind and is fast-forwarded by tapRead, which
// is what should happen — a mode that was off has no claim on old audio.
const tapRingSize = 1 << 15

// tapUpstreamKind names where the tap is pulling from. A change of upstream
// invalidates the cursors exactly the way a change of source does.
type tapUpstreamKind int

const (
	tapFromSource tapUpstreamKind = iota // Source.Drain
	tapFromFVF                           // fvf.vis, the FVF engine's processed output
)

// tapPumpUpstream reports where this frame's audio should come from.
//
// When the FVF audio engine is running it OWNS the source: it drains it in the
// audio callback and publishes the processed result to fvf.vis. Draining the
// source here too would take samples out from under it, and in that state the
// processed stream is what every display should show anyway — it is the sound
// actually coming out. So the tap switches upstream rather than competing.
//
// The spectrogram used to reach into fvf.vis itself, which is precisely why it
// was the ONLY display that worked while FVF was listening. Routing fvf.vis
// through the tap gives every consumer the same stream.
func tapPumpUpstream() tapUpstreamKind {
	if run.selectedMode == "fvf" && fvf.audioActive {
		return tapFromFVF
	}
	return tapFromSource
}

// pump fills the tap once per frame. Call it before anything that reads
// audio — the backdrop, the model, the counter — and exactly once.
func (a *audioTap) pump() {
	src := aud.ensureAudioSource()
	up := tapPumpUpstream()
	if src != a.src || up != a.upstream {
		// Switching source (mic -> generator, say) or upstream (source -> FVF)
		// invalidates every cursor, because they index a stream that no longer
		// exists. Start clean and let tapRead fast-forward each consumer on its
		// next call.
		a.src, a.upstream = src, up
		a.w = 0
		// τ is a property of WHAT IS PLAYING, so a new source needs a new
		// measurement: the one taken from the old stream describes a signal
		// that is no longer there. This is the re-arm that matters, and it is
		// here rather than beside the source switch itself because every way of
		// changing the source — the backend selector, the generator switch, FVF
		// taking the stream over — arrives at this comparison.
		emb.armAutoMeasure()
	}
	if src == nil || !src.Ready() {
		return
	}
	if a.ringL == nil {
		a.ringL = make([]float32, tapRingSize)
		a.ringR = make([]float32, tapRingSize)
		a.scratch = make([]float32, 4096)
		a.scratchR = make([]float32, 4096)
	}
	for drained := 0; drained < tapDrainCap; {
		var n int
		if up == tapFromFVF {
			// The FVF engine's output is one processed signal — it is what is
			// coming out of the speakers, and there is no second channel of it
			// — so it goes into both rings. A consumer asking for "side" of it
			// then gets silence, which is the true answer.
			n = fvf.vis.drain(a.scratch)
			copy(a.scratchR[:n], a.scratch[:n])
		} else {
			n = src.DrainStereo(a.scratch, a.scratchR)
		}
		if n <= 0 {
			break
		}
		for i := 0; i < n; i++ {
			a.ringL[a.w%len(a.ringL)] = a.scratch[i]
			a.ringR[a.w%len(a.ringR)] = a.scratchR[i]
			a.w++
		}
		drained += n
		if n < len(a.scratch) {
			break
		}
	}
}

// tapRead copies the samples written since *cursor into dst (oldest first),
// advances *cursor by the number copied, and returns that count. A cursor that
// has fallen further behind than the ring holds is fast-forwarded to the oldest
// sample still present, matching what ring.drain does with a stale reader.
//
// A zero cursor on a running tap means "new consumer": it starts at the current
// write position rather than replaying the whole ring, so switching a model on
// does not hand it a backlog it would have to discard anyway.
func tapRead(cursor *int, dst []float32) int { return tap.readChan(cursor, dst, tapMix) }

// readChan is tapRead with the fold chosen by the CONSUMER rather than by
// the source. tapRead is the mix, which is what every reader got when the tap
// was mono and what most of them still want.
func (a *audioTap) readChan(cursor *int, dst []float32, c tapChan) int {
	if a.ringL == nil || len(dst) == 0 {
		return 0
	}
	size := len(a.ringL)
	if *cursor < 0 || *cursor > a.w {
		// Not yet joined (the -1 sentinel), or pointing past the write head
		// because a source switch reset it. Either way, start here.
		//
		// The sentinel has to be negative rather than 0: a consumer that joins
		// while the tap is still empty legitimately holds cursor 0, and testing
		// for <= 0 made that indistinguishable from "new", so it re-joined on
		// every call and never read a sample.
		*cursor = a.w
		return 0
	}
	if a.w-*cursor > size {
		*cursor = a.w - size
	}
	n := a.w - *cursor
	if n > len(dst) {
		n = len(dst)
	}
	for i := 0; i < n; i++ {
		j := (*cursor + i) % size
		dst[i] = tapFold(c, a.ringL[j], a.ringR[j])
	}
	*cursor += n
	return n
}

// readStereo is tapRead delivering BOTH channels, for a consumer that needs
// the pair rather than a fold of it. One cursor still, so the two come back
// sample-aligned — which is the whole point for anything measuring a
// relationship between them.
func (a *audioTap) readStereo(cursor *int, l, r []float32) int {
	if a.ringL == nil || len(l) == 0 || len(l) != len(r) {
		return 0
	}
	size := len(a.ringL)
	if *cursor < 0 || *cursor > a.w {
		*cursor = a.w
		return 0
	}
	if a.w-*cursor > size {
		*cursor = a.w - size
	}
	n := a.w - *cursor
	if n > len(l) {
		n = len(l)
	}
	for i := 0; i < n; i++ {
		j := (*cursor + i) % size
		l[i] = a.ringL[j]
		r[i] = a.ringR[j]
	}
	*cursor += n
	return n
}

// ready reports whether the tap has a live source behind it, so callers can
// keep the "no audio yet" branches they had around Drain.
func (a *audioTap) ready() bool { return a.src != nil && a.src.Ready() }

// Read cursors, one per frame-loop consumer. They live here rather than beside
// each consumer so it is visible at a glance that the tap has exactly these
// readers, and that adding another means adding a cursor rather than a Drain.
// tapUnjoined is the cursor value meaning "this consumer has not read yet".
const tapUnjoined = -1

var (
	spectCursor   = tapUnjoined
	counterCursor = tapUnjoined
	rpCursor      = tapUnjoined
)
