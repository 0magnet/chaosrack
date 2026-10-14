package attractor

import (
	"image/color"
	"math"

	"github.com/0magnet/chaosrack/pkg/colormap"
	"github.com/0magnet/chaosrack/pkg/spectcol"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
)

// The spectrogram's knobs and its column pipeline, shared by the page
// (spectrogram_js.go, spectparams_js.go) and a native rack: what the knobs
// tell audioprism-go, how a stream of samples becomes colored columns, and
// the pace the columns are drawn at. Where the samples come from and where
// the columns are written are each side's own.

// SpectroControls are the spectrogram's knobs (spectparams_js.go says what
// each is and why it is there).
type SpectroControls struct {
	// Floats because every control in this panel is a float behind a hidden
	// slider, which is what makes reset, permalinks and audio modulation work
	// the same way for all of them.
	dftf   float32 // index into spectDFTSizes, not the size itself
	ovlF   float32
	winF   float32
	scaleF float32
	minF   float32
	maxF   float32

	// chanF is the knob, an index into spectChanNames.
	chanF float32
}

// NewSpectroControls are the spectrogram's knobs at the page's defaults.
func NewSpectroControls() *SpectroControls {
	return &SpectroControls{dftf: 4, ovlF: 50, maxF: 45}
}

// SpectroKnobs are the spectrogram's knobs, by the page's ids.
var SpectroKnobs = []string{"spect-dft", "spect-ovl", "spect-win", "spect-chan", "spect-scale", "spect-min", "spect-max"}

func (s *SpectroControls) field(id string) *float32 {
	switch id {
	case "spect-dft":
		return &s.dftf
	case "spect-ovl":
		return &s.ovlF
	case "spect-win":
		return &s.winF
	case "spect-chan":
		return &s.chanF
	case "spect-scale":
		return &s.scaleF
	case "spect-min":
		return &s.minF
	case "spect-max":
		return &s.maxF
	}
	return nil
}

// Set turns knob id to v, reporting whether the spectrogram has it.
func (s *SpectroControls) Set(id string, v float32) bool {
	p := s.field(id)
	if p == nil {
		return false
	}
	*p = v
	return true
}

// spectDFTSizes is the original's range, 64 to 8192, every power of two. The
// knob carries the index rather than the size so that its positions are evenly
// spaced — a knob running 64 to 8192 linearly would spend three quarters of its
// travel between 2048 and 8192 and be unable to stop on 128 at all.
var spectDFTSizes = []int{64, 128, 256, 512, 1024, 2048, 4096, 8192}

var (
	spectWinNames   = []string{"hann", "hamming", "bartlett", "rectangular"}
	spectScaleNames = []string{"logarithmic", "linear"}
)

// spectChanNames are the folds of a stereo source the spectrogram can show.
//
// The mix is first because it is the honest answer to "what is playing", and a
// spectrogram of one channel of a stereo mix is a spectrogram of half of it.
// The single channels are worth having because the sum hides things: an
// instrument panned hard, a dead side of an interface, a channel out of
// polarity with the other that cancels in the sum and looks like silence.
//
// A mono source ignores this: both channels are the same signal, so every
// position shows the same picture.
var spectChanNames = []string{"mix", "left", "right"}

// pick reads a knob as an index into a list, since a knob can be dragged past
// either end of one by audio modulation or by a permalink written by hand.
func pick(v float32, n int) int {
	i := max(int(v+0.5), 0)
	if i >= n {
		i = n - 1
	}
	return i
}

// Apply copies the knobs into the shared audioprism-go settings, which is
// where every renderer reads them from, and reports whether the transform
// size changed: everything sized by it has to be built again.
//
// Called every frame rather than on change. The knobs can move from the panel,
// from a MIDI controller, from an audio modulator or from a permalink being
// applied, and there is no single place all of those pass through; reading them
// once a frame is a handful of comparisons and cannot get out of step.
func (s *SpectroControls) Apply() (resized bool) {
	sg.S.SetWindowByName(spectWinNames[pick(s.winF, len(spectWinNames))])
	sg.S.SetScaleByName(spectScaleNames[pick(s.scaleF, len(spectScaleNames))])

	lo, hi := float64(s.minF), float64(s.maxF)
	if sg.S.MagScale() == sg.ScaleLinear && lo < 0 {
		lo = 0
	}
	// The window must not invert: Normalize would divide by a negative span and
	// paint the picture backwards rather than fail, so the ceiling gives way to
	// the floor rather than the other way around.
	if hi <= lo {
		hi = lo + 1
	}
	// Ceiling first — setting a floor above the old ceiling and then raising it
	// would pass through an inverted state.
	sg.S.SetMagMax(hi)
	sg.S.SetMagMin(lo)

	sg.S.SetOverlap(float64(s.ovlF) / 100.0)

	if size := spectDFTSizes[pick(s.dftf, len(spectDFTSizes))]; size != sg.S.GetDFTSize() {
		sg.S.SetDFTSize(size)
		return true
	}
	return false
}

// Fold is l and r reduced to the channel the CHAN knob picks, as the page's
// source folds them as they arrive.
func (s *SpectroControls) Fold(l, r []float32) []float32 {
	c := [...]tapChan{tapMix, tapLeft, tapRight}[pick(s.chanF, len(spectChanNames))]
	out := make([]float32, min(len(l), len(r)))
	for i := range out {
		out[i] = tapFold(c, l[i], r[i])
	}
	return out
}

// The magnitude half of the spectrogram's coloring, split out so palette_js.go
// can normalize a magnitude without reaching into the library's settings lock
// at three separate call sites.
//
// spectMagnitude applies the scale knob: logarithmic is the decibel reading the
// MIN and MAX knobs are calibrated in, linear is the raw magnitude.
func spectMagnitude(v float64) float64 {
	if sg.S.MagScale() == sg.ScaleLog {
		return 20 * math.Log10(v+1e-10)
	}
	return v
}

// spectMagMin and spectMagMax are the window the normalization runs over — the
// MIN and MAX knobs, read back through the library so there is one copy of
// them rather than two that can drift.
func spectMagMin() float64 { lo, _ := sg.S.MagWindow(); return lo }
func spectMagMax() float64 { _, hi := sg.S.MagWindow(); return hi }

// SpectroPixel is the color a raw magnitude paints on the spectrogram through
// map m: normalized by the library's scale and MIN/MAX window, then colored
// (palette_js.go's spectrogramPixel says why the two are split there).
func SpectroPixel(m colormap.Map) func(mag float64) color.Color {
	return func(mag float64) color.Color {
		return m.At(sg.Normalize(spectMagnitude(mag), spectMagMin(), spectMagMax()))
	}
}

// SpectroRows is how many rows a column has at the current transform size:
// one per bin.
func SpectroRows() int { return spectcol.Rows(sg.S.GetDFTSize()) }

const (
	spectMaxQueue     = 120 // fast-forward the scroll if we fall this far behind
	spectQueueCatchup = 60

	// spectQueueTarget is the depth the queue is drained toward. Not zero: the
	// producer and the flush are on different clocks, so a column or two in
	// hand is what keeps the scroll smooth rather than stuttering whenever one
	// arrives a moment late.
	spectQueueTarget = 2

	// SpectroMaxAccum is the most audio one update may take in. A source backed
	// by a generator rather than a buffer never reports "nothing left", so the
	// drain needs a ceiling of its own or it runs until the heap does. 32768
	// samples is about 0.7 s at 44.1 kHz — far more than a frame can turn into
	// visible columns, and small enough that the worst case is a fraction of a
	// second of audio held, not gigabytes.
	SpectroMaxAccum = 1 << 15
)

// SpectroFeed turns a stream of samples into spectrogram columns: an
// overlapping STFT whose columns are produced as the samples arrive (bursty)
// and handed on at a steady wall-clock rate, so the scroll never stutters.
type SpectroFeed struct {
	// accum buffers samples until a full StepSize hop is available; overlap
	// is the sliding window.
	overlap []float32
	accum   []float32
	queue   [][]byte
	lastMs  float64
	colFrac float64
}

// Reset empties the feed for a transform of size samples.
func (f *SpectroFeed) Reset(size int) {
	f.overlap = make([]float32, size)
	f.accum = f.accum[:0]
	f.queue = f.queue[:0]
	f.lastMs = 0
	f.colFrac = 0
}

// Room is how many more samples the feed takes before Step.
func (f *SpectroFeed) Room() int { return SpectroMaxAccum - len(f.accum) }

// Add takes samples in.
func (f *SpectroFeed) Add(x []float32) { f.accum = append(f.accum, x...) }

// Step turns every full hop taken in into a column rows high, colored by
// pixel, and queues it.
func (f *SpectroFeed) Step(rows int, pixel func(float64) color.Color) {
	size, step := len(f.overlap), sg.S.StepSize()
	if step < 1 {
		step = 1
	}
	if step > size {
		step = size
	}
	consumed := 0
	for len(f.accum)-consumed >= step {
		// Slide the window by one hop: keep the size-step samples the next
		// window shares with this one, append the step that follows them.
		// Those two counts are equal only at 50% overlap, which is why they
		// are written out separately rather than both called "step".
		copy(f.overlap, f.overlap[step:])
		copy(f.overlap[size-step:], f.accum[consumed:consumed+step])
		consumed += step
		if col := spectcol.ColumnWith(spectcol.Mags(f.overlap), rows, pixel); col != nil {
			f.queue = append(f.queue, col)
		}
	}
	f.accum = append(f.accum[:0], f.accum[consumed:]...)
}

// Flush hands put the queued columns due by nowMs at the audio column rate
// (sr/StepSize per second), paced by wall-clock time rather than frame or
// burst timing. A backlog beyond spectMaxQueue is fast-forwarded so the
// display never falls permanently behind.
func (f *SpectroFeed) Flush(nowMs float64, sr int, put func(col []byte)) {
	if f.lastMs == 0 {
		f.lastMs = nowMs
	}
	elapsed := max(nowMs-f.lastMs, 0)
	f.lastMs = nowMs

	step := max(sg.S.StepSize(), 1)
	colsPerMs := float64(sr) / float64(step) / 1000.0

	f.colFrac += elapsed * colsPerMs
	toFlush := int(f.colFrac)
	f.colFrac -= float64(toFlush)

	for i := 0; i < toFlush && len(f.queue) > 0; i++ {
		put(f.queue[0])
		f.queue = f.queue[1:]
	}
	// Work off a standing backlog. The pacing above flushes at exactly the rate
	// columns are produced, so a queue — however it formed — is never worked
	// off: it just becomes permanent delay between what is heard and what is
	// drawn. Behind the Takens embedding this parked at seventeen columns, a
	// third of a second, and stayed there.
	//
	// One extra column per call clears that in about as long as it represents,
	// and one column is a single texel of scroll, so the correction is not
	// visible as a jump. The fast-forward below still handles the large
	// backlogs this is too gentle for.
	if len(f.queue) > spectQueueTarget {
		put(f.queue[0])
		f.queue = f.queue[1:]
	}
	if len(f.queue) > spectMaxQueue {
		drop := len(f.queue) - spectQueueCatchup
		for i := range drop {
			put(f.queue[i])
		}
		f.queue = f.queue[drop:]
	}
	if len(f.queue) == 0 {
		f.queue = f.queue[:0]
	}
}

// SpectroSize is the transform size the knobs have set.
func SpectroSize() int { return sg.S.GetDFTSize() }
