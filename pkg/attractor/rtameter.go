package attractor

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/acoustics"
	"github.com/0magnet/chaosrack/pkg/meters"
)

// The RTA's measurement and its picture, shared by the page (rta_js.go) and a
// native rack: the bands, when the FFT runs, the meter ballistics, and the
// bars and peak marks they make. Where the samples come from and how the lines
// reach the screen are each side's own.

const (
	// RTAFFT is the analysis length. 16384 at 48 kHz is a 2.9 Hz bin, which is
	// what the bottom bands need: a 1/3-octave band at 25 Hz is 5.8 Hz wide and
	// would otherwise hold no bins at all.
	RTAFFT = 16384

	// rtaPeriodMs is how often the FFT runs. Six times a second is faster than
	// anyone reads a bar chart and slow enough to leave the frame budget alone.
	rtaPeriodMs = 160
)

// RTA is a fractional-octave analyzer: its band levels and peaks and its
// knobs.
type RTA struct {
	nextMs float64
	bands  []acoustics.RTABand
	levels []float64 // this analysis
	held   []float64 // after the meter ballistics
	peaks  []float64
	lastB  int
	bars   []PlotVertex
	marks  []PlotVertex

	// The knobs.
	fracF  float32 // index into acoustics.RTAFractions; 1 is third-octave
	chanF  float32 // which signal
	rangeF float32 // dB shown from the top of the scale down
	topF   float32 // dBFS at the top of the display
	avgF   float32 // averaging, 0 = none
	holdF  float32 // peak hold: 0 off, else decay in dB/s
}

// NewRTA is an RTA at the page's defaults.
func NewRTA() *RTA {
	return &RTA{fracF: 1, rangeF: 70, avgF: 3, holdF: 1}
}

// RTAKnobs are the RTA's knobs, by the page's ids.
var RTAKnobs = []string{"rta-frac", "rta-chan", "rta-top", "rta-range", "rta-avg", "rta-hold"}

func (r *RTA) field(id string) *float32 {
	switch id {
	case "rta-frac":
		return &r.fracF
	case "rta-chan":
		return &r.chanF
	case "rta-top":
		return &r.topF
	case "rta-range":
		return &r.rangeF
	case "rta-avg":
		return &r.avgF
	case "rta-hold":
		return &r.holdF
	}
	return nil
}

// Set turns knob id to v, reporting whether the RTA has it.
func (r *RTA) Set(id string, v float32) bool {
	p := r.field(id)
	if p == nil {
		return false
	}
	*p = v
	return true
}

// fraction is the band-width knob as a 1/b, clamped. Audio modulation can
// drive any registered parameter, so the value arriving is not necessarily a
// detent, and the range is checked before the conversion (stereoAxisSel's trap).
func (r *RTA) fraction() int {
	v := r.fracF
	if !(v > 0) { // false for NaN
		return acoustics.RTAFractions[0]
	}
	last := len(acoustics.RTAFractions) - 1
	if v > float32(last) {
		return acoustics.RTAFractions[last]
	}
	return acoustics.RTAFractions[int(v+0.5)]
}

// useBands lays the bands out again when the band-width knob has moved.
func (r *RTA) useBands() {
	b := r.fraction()
	if b == r.lastB && r.bands != nil {
		return
	}
	r.lastB = b
	r.bands = acoustics.RTABands(b)
	r.levels = make([]float64, len(r.bands))
	r.held = make([]float64, len(r.bands))
	r.peaks = make([]float64, len(r.bands))
	for i := range r.held {
		r.held[i] = acoustics.RTAFloorDB
		r.peaks[i] = acoustics.RTAFloorDB
	}
}

// Fold is l and rt reduced to the signal the SRC knob picks.
func (r *RTA) Fold(l, rt []float32) []float32 { return FoldChan(r.chanF, l, rt) }

// Due reports whether an analysis is due at nowMs, and if so starts the next
// period.
func (r *RTA) Due(nowMs float64) bool {
	if nowMs < r.nextMs {
		return false
	}
	r.nextMs = nowMs + rtaPeriodMs
	return true
}

// Analyze measures the bands of x, the newest RTAFFT samples at rate sr.
func (r *RTA) Analyze(x []float32, sr int) {
	r.useBands()
	acoustics.RTALevels(meters.ComputeFFTMagsKind(x, acoustics.RTAWindowKind), RTAFFT, sr,
		r.bands, acoustics.RTAWindowKind, r.levels)
}

// Advance applies the meter ballistics over frames of the page's 60 a second:
// 1 is the page's step, once a frame.
//
// Per frame rather than per analysis, so the bars move smoothly between
// measurements — which is what makes a 6 Hz analysis look like a 60 Hz display.
func (r *RTA) Advance(frames float64) {
	if r.held == nil {
		return
	}
	// AVG 0 is no smoothing at all: the bars show each analysis as it lands,
	// which is the setting for watching a transient rather than reading a room.
	rise, fall := 1.0, 1.0
	if r.avgF > 0 {
		// The knob is "how much", so it has to become a coefficient that gets
		// SMALLER as the knob goes up. Rise stays quicker than fall, which is
		// every level meter ever built: a peak that is there and gone inside one
		// window still has to move the display, and a display that fell as fast
		// would flicker at the frame rate.
		a := float64(r.avgF)
		rise = 1 / (1 + a*0.5)
		fall = 1 / (1 + a*3)
	}
	if frames != 1 {
		// k frames of a one-pole step are one step of 1-(1-c)^k.
		rise = 1 - math.Pow(1-rise, frames)
		fall = 1 - math.Pow(1-fall, frames)
	}
	acoustics.RTASmooth(r.held, r.levels, rise, fall)
	if r.holdF > 0 {
		// The knob is dB per second; the decay is per frame.
		acoustics.RTAPeakHold(r.peaks, r.held, float64(r.holdF)/60*frames)
	} else {
		copy(r.peaks, r.held)
	}
}

// y maps a level in dB to a clip-space y, with the top of the scale at the
// top of the screen and the bottom of the range at the bottom.
func (r *RTA) y(db float64) float32 {
	top := float64(r.topF)
	rng := float64(r.rangeF)
	if rng < 1 {
		rng = 1
	}
	v := (db - (top - rng)) / rng // 0 at the bottom of the range, 1 at the top
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	return float32(v*1.8 - 0.9) // the scope's own 0.9 of the screen
}

// barColor is the color of a bar at a level, through the Colors module's
// palette.
//
// The value handed to the colormap is the bar's own HEIGHT on the displayed
// scale — the same 0..1 the bar is drawn at — so the color and the height say
// the same thing twice, in two ways the eye reads differently. That is the
// point rather than a redundancy: a row of bars is scanned for its SHAPE and a
// color ramp is scanned for its outliers, and one loud band among thirty is
// far more obvious as a color than as a height.
func (r *RTA) barColor(col AnalyzerColors, db float64) [3]float32 {
	top := float64(r.topF)
	rng := float64(r.rangeF)
	if rng < 1 {
		rng = 1
	}
	return col.At((db - (top - rng)) / rng)
}

// Lines are the bars, a line from the floor to each band's level, and the
// peak marks, a short dash at each band's held maximum, colored by col. They
// are the RTA's own until the next call.
func (r *RTA) Lines(col AnalyzerColors) (bars, marks []PlotVertex) {
	r.useBands()
	n := len(r.bands)
	r.bars, r.marks = r.bars[:0], r.marks[:0]
	// Bars are spaced evenly across the screen rather than by frequency: the
	// bands are already equal RATIOS, so equal widths is what puts a logarithmic
	// frequency axis on the display. That is the whole visual point of a
	// fractional-octave analyzer over a spectrogram's linear bins.
	bottom := r.y(acoustics.RTAFloorDB)
	// The FOOT of a colored bar is drawn at the floor's color rather than the
	// level's, so it is a gradient up its own height instead of a flat stripe.
	// On a colormap that runs dark-to-bright that reads as a bar lit from its
	// top, which is what the level is.
	foot := r.barColor(col, acoustics.RTAFloorDB)
	// The marks reach just over a third of the band spacing either side, so
	// consecutive marks have a visible gap between them. At a half — the full
	// spacing — they touch, and thirty-one of them become one continuous line
	// across the display that reads as a curve rather than as a per-band maximum.
	half := float32(0.63 / float64(n))
	for i := range r.bands {
		x := float32(-0.9 + 1.8*(float64(i)+0.5)/float64(n))
		r.bars = append(r.bars,
			PlotVertex{x, bottom, foot},
			PlotVertex{x, r.y(r.held[i]), r.barColor(col, r.held[i])})
		y, c := r.y(r.peaks[i]), r.barColor(col, r.peaks[i])
		r.marks = append(r.marks, PlotVertex{x - half, y, c}, PlotVertex{x + half, y, c})
	}
	return r.bars, r.marks
}
