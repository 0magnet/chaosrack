package attractor

import (
	"math"
	"sort"
)

// Loudness to ITU-R BS.1770 and EBU R 128 — LUFS, loudness range and true peak.
//
// The measurement everything is delivered against: broadcast, streaming and
// mastering all specify a loudness target in LUFS, and a peak meter cannot
// answer the question because loudness is not peak — a compressed mix and a
// dynamic one can share a peak and be ten decibels apart to listen to.
//
// Four numbers, and they are four because they answer four questions:
//
//	MOMENTARY    over 400 ms, ungated. What it sounds like right now.
//	SHORT-TERM   over 3 s, ungated. What the last few seconds sounded like,
//	             which is the window a person actually judges level over.
//	INTEGRATED   over everything, GATED. The number a delivery spec means, and
//	             the gating is what makes it mean anything: silence between
//	             songs and a quiet intro must not drag the programme's loudness
//	             down, so blocks below a threshold are thrown away before the
//	             average is taken.
//	LRA          the loudness RANGE: how far the loud parts are above the quiet
//	             ones. A single number for "how dynamic is this", and the thing
//	             a loudness target alone says nothing about.
//
// True peak is beside them because a loudness target always comes with a peak
// ceiling, and the peak that matters is the one BETWEEN samples — see
// TruePeak, where a signal whose samples never exceed 0.71 is shown reaching
// 1.0 on reconstruction.
//
// Untagged, so the native test can check the filter against the published
// coefficients and the integrator against arithmetic.

// ── K-weighting ──────────────────────────────────────────────────────────
//
// Two biquads: a high shelf that lifts everything above about 2 kHz by 4 dB,
// standing in for the head's own acoustic response, and a high-pass at about
// 38 Hz that removes the rumble the ear does not weigh. Together they are the
// "K" in LUFS.
//
// The coefficients are DERIVED FROM THE ANALOG PROTOTYPE at the rate in use,
// rather than being the 48 kHz numbers the standard prints. The printed numbers
// are only correct at 48 kHz, and a meter that used them at 44.1 would be
// weighting by a filter whose corners had moved — quietly, and in a way no
// readout would show. The test checks that this derivation reproduces the
// published numbers exactly at 48 kHz, which is what makes it the same filter
// rather than a similar one.

// biquad is one direct-form-I section, with the a₀ normalization already done.
type biquad struct{ b0, b1, b2, a1, a2 float64 }

// biquadState is one section's memory, per channel.
type biquadState struct{ x1, x2, y1, y2 float64 }

func (s *biquadState) step(c biquad, x float64) float64 {
	y := c.b0*x + c.b1*s.x1 + c.b2*s.x2 - c.a1*s.y1 - c.a2*s.y2
	s.x2, s.x1 = s.x1, x
	s.y2, s.y1 = s.y1, y
	return y
}

func (s *biquadState) reset() { *s = biquadState{} }

// The prototype's design constants, as BS.1770-4 gives them.
const (
	kShelfF0 = 1681.974450955533
	kShelfG  = 3.999843853973347
	kShelfQ  = 0.7071752369554196
	kHPF0    = 38.13547087602444
	kHPQ     = 0.5003270373238773
)

// kWeightingShelf returns the high-shelf section at a sample rate.
func kWeightingShelf(sampleRate float64) biquad {
	k := math.Tan(math.Pi * kShelfF0 / sampleRate)
	vh := math.Pow(10, kShelfG/20)
	vb := math.Pow(vh, 0.4996667741545416)
	a0 := 1 + k/kShelfQ + k*k
	return biquad{
		b0: (vh + vb*k/kShelfQ + k*k) / a0,
		b1: 2 * (k*k - vh) / a0,
		b2: (vh - vb*k/kShelfQ + k*k) / a0,
		a1: 2 * (k*k - 1) / a0,
		a2: (1 - k/kShelfQ + k*k) / a0,
	}
}

// kWeightingHighpass returns the RLB high-pass section at a sample rate.
func kWeightingHighpass(sampleRate float64) biquad {
	k := math.Tan(math.Pi * kHPF0 / sampleRate)
	den := 1 + k/kHPQ + k*k
	return biquad{
		b0: 1, b1: -2, b2: 1,
		a1: 2 * (k*k - 1) / den,
		a2: (1 - k/kHPQ + k*k) / den,
	}
}

// biquadGainDB is a section's magnitude response at a frequency, in dB —
// evaluated from the coefficients rather than measured, so a test can predict
// what a tone should read before the meter is asked.
func biquadGainDB(c biquad, f, sampleRate float64) float64 {
	w := 2 * math.Pi * f / sampleRate
	cw, sw := math.Cos(w), math.Sin(w)
	c2w, s2w := math.Cos(2*w), math.Sin(2*w)
	nr := c.b0 + c.b1*cw + c.b2*c2w
	ni := -(c.b1*sw + c.b2*s2w)
	dr := 1 + c.a1*cw + c.a2*c2w
	di := -(c.a1*sw + c.a2*s2w)
	return 20 * math.Log10(math.Hypot(nr, ni)/math.Hypot(dr, di))
}

// KWeightingGainDB is the whole weighting's response at a frequency.
func KWeightingGainDB(f, sampleRate float64) float64 {
	return biquadGainDB(kWeightingShelf(sampleRate), f, sampleRate) +
		biquadGainDB(kWeightingHighpass(sampleRate), f, sampleRate)
}

// ── The meter ────────────────────────────────────────────────────────────

const (
	// loudnessOffset is the −0.691 dB in BS.1770's loudness equation, which
	// aligns the scale so that LUFS and dBFS agree for a reference signal.
	loudnessOffset = -0.691

	// blockMS is the momentary window, and the block the integrated
	// measurement gates over. stepMS is the 75% overlap the standard asks for.
	blockMS = 400
	stepMS  = 100

	// shortMS is the short-term window.
	shortMS = 3000

	// absGateLUFS is the absolute gate: anything quieter than this is silence
	// as far as a programme's loudness is concerned.
	absGateLUFS = -70.0

	// relGateLU is the relative gate, below the ungated mean of what survived
	// the absolute one. Together they are what stops a quiet intro and the gaps
	// between tracks from dragging a programme's number down.
	relGateLU = -10.0

	// lraRelGateLU is the loudness range's own relative gate, which is looser
	// than the integrated measurement's because LRA is ABOUT the quiet parts
	// and a −10 LU gate would throw away the very thing being measured.
	lraRelGateLU = -20.0

	// LoudnessFloor is what a silent or too-short measurement reports. −∞ is
	// the true answer and not a number anybody can put on a meter.
	LoudnessFloor = -120.0
)

// LoudnessResult is one reading.
type LoudnessResult struct {
	Momentary  float64 // LUFS over 400 ms
	ShortTerm  float64 // LUFS over 3 s
	Integrated float64 // LUFS, gated, over everything measured
	LRA        float64 // LU
	TruePeak   float64 // dBTP
	OK         bool    // enough audio for the integrated number to mean anything
}

// LoudnessMeter accumulates the blocks the four numbers are made of.
//
// It keeps the mean square of each 100 ms block rather than the samples: the
// integrated measurement needs every block of a whole program and the short
// ones are windows over the same blocks, so one list of block powers serves all
// four and the memory is a few hundred bytes a minute rather than the audio.
type LoudnessMeter struct {
	sampleRate int
	shelf, hp  biquad
	stL, stHP  [2]biquadState // per channel, per section

	acc      [2]float64 // running sum of squares in the block being filled
	accN     int
	blockLen int

	blocks   []float64 // mean square per 100 ms block, summed over channels
	truePeak float64
}

// NewLoudnessMeter returns a meter for a sample rate.
func NewLoudnessMeter(sampleRate int) *LoudnessMeter {
	m := &LoudnessMeter{}
	m.Reset(sampleRate)
	return m
}

// Reset clears the measurement and retunes the filter for a sample rate.
func (m *LoudnessMeter) Reset(sampleRate int) {
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	m.sampleRate = sampleRate
	m.shelf = kWeightingShelf(float64(sampleRate))
	m.hp = kWeightingHighpass(float64(sampleRate))
	for i := range m.stL {
		m.stL[i].reset()
		m.stHP[i].reset()
	}
	m.acc = [2]float64{}
	m.accN = 0
	m.blockLen = sampleRate * stepMS / 1000
	if m.blockLen < 1 {
		m.blockLen = 1
	}
	m.blocks = m.blocks[:0]
	m.truePeak = 0
}

// SampleRate is the rate the meter is currently tuned for.
func (m *LoudnessMeter) SampleRate() int { return m.sampleRate }

// Add feeds one buffer of both channels through the weighting and into the
// block accumulator.
func (m *LoudnessMeter) Add(l, r []float32) {
	if len(l) != len(r) {
		return
	}
	for i := range l {
		lv, rv := float64(l[i]), float64(r[i])
		// Peak BEFORE the weighting: a peak ceiling is about what the converter
		// has to reproduce, not about what the ear weighs.
		if a := math.Abs(lv); a > m.truePeak {
			m.truePeak = a
		}
		if a := math.Abs(rv); a > m.truePeak {
			m.truePeak = a
		}
		lw := m.stHP[0].step(m.hp, m.stL[0].step(m.shelf, lv))
		rw := m.stHP[1].step(m.hp, m.stL[1].step(m.shelf, rv))
		m.acc[0] += lw * lw
		m.acc[1] += rw * rw
		m.accN++
		if m.accN >= m.blockLen {
			// The channel weights are 1.0 for left and right; the standard's
			// 1.41 is for the surround channels, which this app does not have.
			ms := (m.acc[0] + m.acc[1]) / float64(m.accN)
			m.blocks = append(m.blocks, ms)
			m.acc = [2]float64{}
			m.accN = 0
		}
	}
}

// SetTruePeak records an externally measured true peak — the oversampled one,
// which Add cannot compute because it sees the samples and not what lies
// between them.
func (m *LoudnessMeter) SetTruePeak(v float64) {
	if v > m.truePeak {
		m.truePeak = v
	}
}

// loudnessOf turns a mean square into LUFS.
func loudnessOf(ms float64) float64 {
	if ms <= 0 {
		return LoudnessFloor
	}
	v := loudnessOffset + 10*math.Log10(ms)
	if v < LoudnessFloor {
		return LoudnessFloor
	}
	return v
}

// windowLoudness is the loudness over the last n blocks.
func (m *LoudnessMeter) windowLoudness(ms int) float64 {
	n := ms / stepMS
	if n < 1 || len(m.blocks) < n {
		return LoudnessFloor
	}
	var s float64
	for _, b := range m.blocks[len(m.blocks)-n:] {
		s += b
	}
	return loudnessOf(s / float64(n))
}

// overlappedBlocks builds the 400 ms gating blocks from the 100 ms ones — the
// 75% overlap BS.1770 asks for, which is what stops a loud moment falling
// across a block boundary and being averaged away.
func (m *LoudnessMeter) overlappedBlocks() []float64 {
	per := blockMS / stepMS
	if len(m.blocks) < per {
		return nil
	}
	out := make([]float64, 0, len(m.blocks)-per+1)
	for i := 0; i+per <= len(m.blocks); i++ {
		var s float64
		for _, b := range m.blocks[i : i+per] {
			s += b
		}
		out = append(out, s/float64(per))
	}
	return out
}

// gatedMean applies the two-stage gate and returns the mean square that
// survives it.
//
// The absolute gate first, then a relative one computed from what the absolute
// gate left. Two stages because one cannot do it: an absolute threshold alone
// cannot know what "quiet for this program" means, and a relative one alone
// would be dragged down by the silence it is supposed to ignore.
func gatedMean(blocks []float64) (float64, bool) {
	var sum float64
	var n int
	for _, b := range blocks {
		if loudnessOf(b) > absGateLUFS {
			sum += b
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	threshold := loudnessOf(sum/float64(n)) + relGateLU
	sum, n = 0, 0
	for _, b := range blocks {
		if l := loudnessOf(b); l > absGateLUFS && l > threshold {
			sum += b
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n), true
}

// loudnessRange is EBU Tech 3342: the spread between the 10th and 95th
// percentiles of the gated short-term distribution.
//
// Percentiles rather than the extremes, and that is the whole design: a single
// cymbal or one moment of silence would otherwise set the range of a whole
// program. The gate is looser than the integrated measurement's because LRA
// is ABOUT the quiet parts, and a −10 LU gate would discard what is being
// measured.
func loudnessRange(blocks []float64) float64 {
	per := shortMS / stepMS
	if len(blocks) < per {
		return 0
	}
	var st []float64
	for i := 0; i+per <= len(blocks); i++ {
		var s float64
		for _, b := range blocks[i : i+per] {
			s += b
		}
		if l := loudnessOf(s / float64(per)); l > absGateLUFS {
			st = append(st, l)
		}
	}
	if len(st) < 2 {
		return 0
	}
	var sum float64
	for _, l := range st {
		sum += math.Pow(10, (l-loudnessOffset)/10)
	}
	threshold := loudnessOf(sum/float64(len(st))) + lraRelGateLU
	var kept []float64
	for _, l := range st {
		if l > threshold {
			kept = append(kept, l)
		}
	}
	if len(kept) < 2 {
		return 0
	}
	sort.Float64s(kept)
	return percentileOf(kept, 0.95) - percentileOf(kept, 0.10)
}

// percentileOf reads a percentile from a sorted slice, interpolating between
// the two neighbors rather than snapping to one.
func percentileOf(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	x := p * float64(len(sorted)-1)
	i := int(x)
	if i >= len(sorted)-1 {
		return sorted[len(sorted)-1]
	}
	f := x - float64(i)
	return sorted[i]*(1-f) + sorted[i+1]*f
}

// Result reads the four numbers.
func (m *LoudnessMeter) Result() LoudnessResult {
	var r LoudnessResult
	r.Momentary = m.windowLoudness(blockMS)
	r.ShortTerm = m.windowLoudness(shortMS)
	r.Integrated = LoudnessFloor
	r.TruePeak = LoudnessFloor
	if m.truePeak > 0 {
		r.TruePeak = 20 * math.Log10(m.truePeak)
	}
	if ms, ok := gatedMean(m.overlappedBlocks()); ok {
		r.Integrated = loudnessOf(ms)
		r.OK = true
	}
	r.LRA = loudnessRange(m.blocks)
	return r
}

// ── True peak ────────────────────────────────────────────────────────────

// truePeakOversample is how many times the signal is upsampled before its peak
// is read. BS.1770 asks for at least four, which holds the error under about
// 0.1 dB.
const truePeakOversample = 4

// TruePeak is the peak of the RECONSTRUCTED signal, which is not the peak of
// the samples and can be a great deal higher.
//
// A converter does not output the samples, it outputs the bandlimited curve
// through them, and that curve overshoots. The clearest case is a full-scale
// sine at a quarter of the sample rate landing halfway between samples: every
// sample reads 0.707 and the signal it represents reaches 1.0 — so a sample
// peak meter says −3 dBFS while the converter is clipping. A delivery spec's
// peak ceiling is a TRUE peak ceiling for exactly this reason.
//
// Upsampled with a windowed-sinc rather than by interpolation: linear
// interpolation between samples finds the chord, and the overshoot is precisely
// what is not on the chord.
//
// THE KERNELS ARE NORMALIZED, one per phase. A windowed sinc does not sum to
// exactly one at an arbitrary fractional offset — the window truncates a tail
// that is not symmetric about that offset — so the interpolator has a small
// gain of its own that varies with phase. Unnormalized, a plain 0.5-amplitude
// tone measured a true peak of 0.534: a 0.6 dB overshoot invented by the
// meter, on a signal with no intersample peak at all. A peak meter that reads
// high on everything fails every delivery spec it is consulted about.
func TruePeak(x []float32) float64 {
	if len(x) == 0 {
		return 0
	}
	peak := 0.0
	for _, v := range x {
		if a := math.Abs(float64(v)); a > peak {
			peak = a
		}
	}
	// ONLY WHERE THE WHOLE KERNEL FITS. At the ends of a buffer the taps run
	// off the edge, and skipping the missing ones is the same as treating the
	// audio either side as silence — a step discontinuity, which a bandlimited
	// interpolator answers with exactly the overshoot this function exists to
	// detect. Measured that way, a plain 0.5-amplitude tone with no intersample
	// peak at all read 0.534: the meter was reporting the edge of its own
	// buffer. The interior is the part that can be reconstructed, so it is the
	// part that is measured; a caller wanting the peak of a continuous stream
	// hands over overlapping buffers, as the meter does.
	if len(x) <= 2*truePeakTaps {
		return peak // too short to reconstruct anything: the sample peak is the answer
	}
	// WHERE AN OVERSHOOT COULD NOT POSSIBLY BE, DO NOT LOOK.
	//
	// This is the loudness meter's expensive half and it runs on every sample
	// of both channels on every frame — the meter's integration cannot skip a
	// block, so unlike the distortion and wow-and-flutter analyzers it cannot
	// be put on a timer. It was 6.5% of the whole page's main thread, the
	// largest single leaf in the profile: 49 taps by three phases is 147
	// multiply-adds a sample, ~14 million a second at 48 kHz stereo, to feed a
	// readout that repaints five times a second.
	//
	// But an interpolated point is a weighted sum of the samples around it, so
	// it cannot exceed the largest of them by more than the kernel's L1 norm:
	//
	//	|s| = |Σ x[i+t]·k[t]| ≤ max|x[i+t]| · Σ|k[t]|
	//
	// Take that bound over a block of positions at once — every sample any of
	// them can reach — and where it does not beat the peak already found,
	// none of those positions can, and the whole block's filtering is skipped.
	// Scanning the block for its largest sample costs about two operations per
	// position against the 147 it saves.
	//
	// The result is EXACTLY the one the full pass gives: the bound is a real
	// bound, not an approximation, and peak only grows, so a block ruled out
	// stays ruled out. What changes is that quiet audio — which is most audio,
	// most of the time, a peak being by definition the rare part — stops being
	// filtered at all. Silence now costs one scan instead of 147 multiplies a
	// sample.
	gain := truePeakMaxGain()
	n := len(x)
	for b0 := truePeakTaps; b0 < n-truePeakTaps; b0 += truePeakBlock {
		b1 := b0 + truePeakBlock
		if b1 > n-truePeakTaps {
			b1 = n - truePeakTaps
		}
		reach := 0.0
		for _, v := range x[b0-truePeakTaps : b1+truePeakTaps] {
			if a := math.Abs(float64(v)); a > reach {
				reach = a
			}
		}
		if reach*gain <= peak {
			continue
		}
		for i := b0; i < b1; i++ {
			// Phases innermost so the block's samples are read once for all
			// three rather than three times over.
			for p := 1; p < truePeakOversample; p++ {
				k := truePeakKernel(p)
				var s float64
				for t := -truePeakTaps; t <= truePeakTaps; t++ {
					s += float64(x[i+t]) * k[t+truePeakTaps]
				}
				if a := math.Abs(s); a > peak {
					peak = a
				}
			}
		}
	}
	return peak
}

// truePeakBlock is how many positions share one bound check. Big enough that
// the scan is cheap against what it skips, small enough that one loud sample
// does not drag a long run of quiet ones through the filter with it.
const truePeakBlock = 64

// truePeakGain is the kernel's L1 norm — the most an interpolated point can
// exceed the samples it is built from. Slightly above 1: the kernels sum to
// 1 by construction, and the negative lobes of a windowed sinc are what puts
// the absolute sum over.
var truePeakGain float64

func truePeakMaxGain() float64 {
	if truePeakGain == 0 {
		for p := 1; p < truePeakOversample; p++ {
			s := 0.0
			for _, v := range truePeakKernel(p) {
				s += math.Abs(v)
			}
			if s > truePeakGain {
				truePeakGain = s
			}
		}
	}
	return truePeakGain
}

// truePeakTaps is the kernel's half-width.
const truePeakTaps = 24

// truePeakKernels holds one normalized kernel per fractional phase, built once.
var truePeakKernels [truePeakOversample][]float64

// truePeakKernel returns the interpolation kernel for phase p/oversample.
func truePeakKernel(p int) []float64 {
	if truePeakKernels[p] != nil {
		return truePeakKernels[p]
	}
	frac := float64(p) / truePeakOversample
	k := make([]float64, 2*truePeakTaps+1)
	var sum float64
	for t := -truePeakTaps; t <= truePeakTaps; t++ {
		v := sincWindowed(frac-float64(t), truePeakTaps)
		k[t+truePeakTaps] = v
		sum += v
	}
	if sum != 0 {
		for i := range k {
			k[i] /= sum
		}
	}
	truePeakKernels[p] = k
	return k
}

// sincWindowed is a Blackman-windowed sinc, the interpolation kernel.
func sincWindowed(t float64, taps int) float64 {
	if t == 0 {
		return 1
	}
	if math.Abs(t) > float64(taps) {
		return 0
	}
	x := math.Pi * t
	sinc := math.Sin(x) / x
	// Blackman, over the kernel's own width.
	w := 0.42 + 0.5*math.Cos(math.Pi*t/float64(taps)) + 0.08*math.Cos(2*math.Pi*t/float64(taps))
	return sinc * w
}
