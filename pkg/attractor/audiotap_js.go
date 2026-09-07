//go:build js && wasm

package attractor

import "github.com/0magnet/chaosrack/pkg/audiosrc"

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
// This is the same shape as the existing fvfVis ring, which was added for the
// same reason: when FVF is on it drains the source in the audio callback, so
// the spectrogram reads fvfVis rather than draining a second time. That case
// stays as it is — it is a different clock, not a frame-loop consumer.
var (
	tapRing    []float32
	tapW       int // monotonic count of samples ever written into tapRing
	tapScratch []float32
	tapSrc     audiosrc.Source // source the cursors below are relative to
)

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
	tapFromFVF                           // fvfVis, the FVF engine's processed output
)

var tapUpstream tapUpstreamKind

// tapPumpUpstream reports where this frame's audio should come from.
//
// When the FVF audio engine is running it OWNS the source: it drains it in the
// audio callback and publishes the processed result to fvfVis. Draining the
// source here too would take samples out from under it, and in that state the
// processed stream is what every display should show anyway — it is the sound
// actually coming out. So the tap switches upstream rather than competing.
//
// The spectrogram used to reach into fvfVis itself, which is precisely why it
// was the ONLY display that worked while FVF was listening. Routing fvfVis
// through the tap gives every consumer the same stream.
func tapPumpUpstream() tapUpstreamKind {
	if selectedMode == "fvf" && fvfAudioActive {
		return tapFromFVF
	}
	return tapFromSource
}

// tapPump fills the tap once per frame. Call it before anything that reads
// audio — the backdrop, the model, the counter — and exactly once.
func tapPump() {
	src := ensureAudioSource()
	up := tapPumpUpstream()
	if src != tapSrc || up != tapUpstream {
		// Switching source (mic -> generator, say) or upstream (source -> FVF)
		// invalidates every cursor, because they index a stream that no longer
		// exists. Start clean and let tapRead fast-forward each consumer on its
		// next call.
		tapSrc, tapUpstream = src, up
		tapW = 0
	}
	if src == nil || !src.Ready() {
		return
	}
	if tapRing == nil {
		tapRing = make([]float32, tapRingSize)
		tapScratch = make([]float32, 4096)
	}
	for drained := 0; drained < tapDrainCap; {
		var n int
		if up == tapFromFVF {
			n = fvfVis.drain(tapScratch)
		} else {
			n = src.Drain(tapScratch)
		}
		if n <= 0 {
			break
		}
		for i := 0; i < n; i++ {
			tapRing[tapW%len(tapRing)] = tapScratch[i]
			tapW++
		}
		drained += n
		if n < len(tapScratch) {
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
func tapRead(cursor *int, dst []float32) int {
	if tapRing == nil || len(dst) == 0 {
		return 0
	}
	size := len(tapRing)
	if *cursor < 0 || *cursor > tapW {
		// Not yet joined (the -1 sentinel), or pointing past the write head
		// because a source switch reset it. Either way, start here.
		//
		// The sentinel has to be negative rather than 0: a consumer that joins
		// while the tap is still empty legitimately holds cursor 0, and testing
		// for <= 0 made that indistinguishable from "new", so it re-joined on
		// every call and never read a sample.
		*cursor = tapW
		return 0
	}
	if tapW-*cursor > size {
		*cursor = tapW - size
	}
	n := tapW - *cursor
	if n > len(dst) {
		n = len(dst)
	}
	for i := 0; i < n; i++ {
		dst[i] = tapRing[(*cursor+i)%size]
	}
	*cursor += n
	return n
}

// tapReady reports whether the tap has a live source behind it, so callers can
// keep the "no audio yet" branches they had around Drain.
func tapReady() bool { return tapSrc != nil && tapSrc.Ready() }

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
