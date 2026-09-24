//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/acoustics"
	"github.com/0magnet/chaosrack/pkg/meters"

	"github.com/0magnet/chaosrack/pkg/glctx"
)

// The RTA mode — fractional-octave bands drawn as a bar display.
//
// The band arithmetic is in pkg/acoustics and checked against the Test
// module's own pink noise. This is the picture: how the bars are drawn, how
// often the analysis runs, and the knobs.
//
// It draws with the xy scope's program — a pass-through vec2 vertex shader and
// a solid-color fragment — because that is exactly what a bar display needs
// and a second shader for the same job would be a second thing to keep working.
// Bars are vertical LINES rather than quads: at any usable band count a bar is
// a few pixels wide, the additive halo the scope already uses makes a line read
// as a lit bar, and lines cost two vertices where a quad costs six.
//
// ── IT DOES NOT ANALYZE EVERY FRAME ──────────────────────────────────────
//
// The FFT runs on its own clock and the DISPLAY runs every frame, which are
// different things: between analyses the bars go on moving because the
// smoothing and the peak-hold decay are per-frame. That is what a real
// analyzer's ballistics are, and it is why the picture is smooth at 60 Hz while
// the measurement underneath it is not.

const (
	// rtaFFT is the analysis length. 16384 at 48 kHz is a 2.9 Hz bin, which is
	// what the bottom bands need: a 1/3-octave band at 25 Hz is 5.8 Hz wide and
	// would otherwise hold no bins at all.
	rtaFFT = 16384

	// rtaPeriodMs is how often the FFT runs. Six times a second is faster than
	// anyone reads a bar chart and slow enough to leave the frame budget alone.
	rtaPeriodMs = 160
)

var (
	rtaCursor = tapUnjoined
	rtaBuf    []float32
	rtaFill   int
	rtaNextMs float64
	rtaBands  []acoustics.RTABand
	rtaLevels []float64 // this analysis
	rtaHeld   []float64 // after the meter ballistics
	rtaPeaks  []float64
	rtaLastB  int

	// The knobs.
	rtaFracF  float32 = 1  // index into acoustics.RTAFractions; 1 is third-octave
	rtaChanF  float32      // which signal
	rtaRangeF float32 = 70 // dB shown from the top of the scale down
	rtaTopF   float32      // dBFS at the top of the display
	rtaAvgF   float32 = 3  // averaging, 0 = none
	rtaHoldF  float32 = 1  // peak hold: 0 off, else decay in dB/s
)

func init() {
	registerGenerate("rta", generateRTA)
	attractorParams["rta"] = []paramDef{
		{"rta-frac", "band", &rtaFracF, 1, 0, float32(len(acoustics.RTAFractions) - 1), 1},
		{"rta-chan", "src", &rtaChanF, 0, 0, float32(len(tapChanNames) - 1), 1},
		{"rta-top", "top", &rtaTopF, 0, -60, 20, 1},
		{"rta-range", "rnge", &rtaRangeF, 70, 20, 120, 5},
		{"rta-avg", "avg", &rtaAvgF, 3, 0, 10, 1},
		{"rta-hold", "hold", &rtaHoldF, 1, 0, 40, 1},
	}
}

// rtaFraction is the band-width knob as a 1/b, clamped. Audio modulation can
// drive any registered parameter, so the value arriving is not necessarily a
// detent, and the range is checked before the conversion (stereoAxisSel's trap).
func rtaFraction() int {
	v := rtaFracF
	if !(v > 0) { // false for NaN
		return acoustics.RTAFractions[0]
	}
	last := len(acoustics.RTAFractions) - 1
	if v > float32(last) {
		return acoustics.RTAFractions[last]
	}
	return acoustics.RTAFractions[int(v+0.5)]
}

// rtaAnalyze drains the tap and runs the FFT when its period is up.
func rtaAnalyze(nowMs float64) {
	b := rtaFraction()
	if b != rtaLastB || rtaBands == nil {
		rtaLastB = b
		rtaBands = acoustics.RTABands(b)
		rtaLevels = make([]float64, len(rtaBands))
		rtaHeld = make([]float64, len(rtaBands))
		rtaPeaks = make([]float64, len(rtaBands))
		for i := range rtaHeld {
			rtaHeld[i] = acoustics.RTAFloorDB
			rtaPeaks[i] = acoustics.RTAFloorDB
		}
	}
	if rtaBuf == nil {
		rtaBuf = make([]float32, rtaFFT)
	}
	var scratch [4096]float32
	ch := tapChanSel(rtaChanF)
	for {
		n := tapReadChan(&rtaCursor, scratch[:], ch)
		if n <= 0 {
			break
		}
		if n >= rtaFFT {
			copy(rtaBuf, scratch[n-rtaFFT:n])
			rtaFill = rtaFFT
		} else {
			copy(rtaBuf, rtaBuf[n:])
			copy(rtaBuf[rtaFFT-n:], scratch[:n])
			if rtaFill += n; rtaFill > rtaFFT {
				rtaFill = rtaFFT
			}
		}
		if n < len(scratch) {
			break
		}
	}
	if rtaFill < rtaFFT || nowMs < rtaNextMs {
		return
	}
	rtaNextMs = nowMs + rtaPeriodMs
	acoustics.RTALevels(meters.ComputeFFTMagsKind(rtaBuf, acoustics.RTAWindowKind), rtaFFT, takensSourceRate(),
		rtaBands, acoustics.RTAWindowKind, rtaLevels)
}

// rtaAdvance applies the meter ballistics, once a frame.
//
// Per frame rather than per analysis, so the bars move smoothly between
// measurements — which is what makes a 6 Hz analysis look like a 60 Hz display.
func rtaAdvance() {
	if rtaHeld == nil {
		return
	}
	// AVG 0 is no smoothing at all: the bars show each analysis as it lands,
	// which is the setting for watching a transient rather than reading a room.
	rise, fall := 1.0, 1.0
	if rtaAvgF > 0 {
		// The knob is "how much", so it has to become a coefficient that gets
		// SMALLER as the knob goes up. Rise stays quicker than fall, which is
		// every level meter ever built: a peak that is there and gone inside one
		// window still has to move the display, and a display that fell as fast
		// would flicker at the frame rate.
		a := float64(rtaAvgF)
		rise = 1 / (1 + a*0.5)
		fall = 1 / (1 + a*3)
	}
	acoustics.RTASmooth(rtaHeld, rtaLevels, rise, fall)
	if rtaHoldF > 0 {
		// The knob is dB per second; the decay is per frame.
		acoustics.RTAPeakHold(rtaPeaks, rtaHeld, float64(rtaHoldF)/60)
	} else {
		copy(rtaPeaks, rtaHeld)
	}
}

// generateRTA is the mode's frame: analyze, advance, draw.
func generateRTA() {
	rtaAnalyze(frameNowMs)
	rtaAdvance()
	drawRTA()
}

// rtaY maps a level in dB to a clip-space y, with the top of the scale at the
// top of the screen and the bottom of the range at the bottom.
func rtaY(db float64) float32 {
	top := float64(rtaTopF)
	rng := float64(rtaRangeF)
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

// drawRTA draws the bars and the peak-hold marks.
// rtaBarColor is the color of a bar at a level, through the Colors module's
// palette.
//
// The value handed to the colormap is the bar's own HEIGHT on the displayed
// scale — the same 0..1 the bar is drawn at — so the color and the height say
// the same thing twice, in two ways the eye reads differently. That is the
// point rather than a redundancy: a row of bars is scanned for its SHAPE and a
// color ramp is scanned for its outliers, and one loud band among thirty is
// far more obvious as a color than as a height.
func rtaBarColor(idx int, db float64) [3]float32 {
	top := float64(rtaTopF)
	rng := float64(rtaRangeF)
	if rng < 1 {
		rng = 1
	}
	return analyzerColorAt(idx, (db-(top-rng))/rng)
}

func drawRTA() {
	initVColor()
	n := len(rtaBands)
	if n == 0 {
		return
	}
	pal, colored := analyzerPalette()
	flat := analyzerTraceColor()
	// Two vertices per bar, plus two per peak mark.
	vcFit(n * 4)
	// Bars are spaced evenly across the screen rather than by frequency: the
	// bands are already equal RATIOS, so equal widths is what puts a logarithmic
	// frequency axis on the display. That is the whole visual point of a
	// fractional-octave analyzer over a spectrogram's linear bins.
	bottom := rtaY(acoustics.RTAFloorDB)
	v := 0
	for i := range rtaBands {
		x := float32(-0.9 + 1.8*(float64(i)+0.5)/float64(n))
		c := flat
		if colored {
			c = rtaBarColor(pal, rtaHeld[i])
		}
		// The FOOT of the bar is drawn at the floor's color rather than the
		// level's, so a colored bar is a gradient up its own height instead of
		// a flat stripe. On a colormap that runs dark-to-bright that reads as a
		// bar lit from its top, which is what the level is.
		foot := c
		if colored {
			foot = rtaBarColor(pal, acoustics.RTAFloorDB)
		}
		vcPut(v, x, bottom, foot)
		vcPut(v+1, x, rtaY(rtaHeld[i]), c)
		v += 2
	}
	barVerts := v
	// The peak marks: a short horizontal dash at each band's held maximum.
	//
	// Just over a third of the band spacing either side, so consecutive marks
	// have a visible gap between them. At a half — the full spacing — they touch,
	// and thirty-one of them become one continuous line across the display that
	// reads as a curve rather than as a per-band maximum.
	half := float32(0.63 / float64(n))
	for i := range rtaBands {
		y := rtaY(rtaPeaks[i])
		x := float32(-0.9 + 1.8*(float64(i)+0.5)/float64(n))
		c := flat
		if colored {
			c = rtaBarColor(pal, rtaPeaks[i])
		}
		vcPut(v, x-half, y, c)
		vcPut(v+1, x+half, y, c)
		v += 2
	}
	peakVerts := v - barVerts

	glctx.GL.Call("disable", glctx.Types.DepthTest)
	glctx.GL.Call("clearColor", 0, 0, 0, 0)
	glctx.GL.Call("clear", glctx.Types.ColorBufferBit)
	glctx.GL.Call("enable", glctx.GL.Get("BLEND"))
	glctx.GL.Call("blendFunc", glctx.GL.Get("SRC_ALPHA"), glctx.GL.Get("ONE"))

	// The bars, widened the way the scope's trace is: WebGL cannot be relied on
	// for lineWidth, so each is drawn several times at sub-pixel offsets.
	vcUpload(v)
	dx := float32(1.0) / float32(width)
	for k := -2; k <= 2; k++ {
		vcSpan(glctx.Types.Lines, 0, barVerts, 0.5, float32(k)*dx, 0)
	}
	// The peak marks once and brighter: they are a held maximum rather than a
	// level, and widening them would make them read as bars of their own.
	vcSpan(glctx.Types.Lines, barVerts, peakVerts, 0.9, 0, 0)
	vcDone()
	glctx.GL.Call("disable", glctx.GL.Get("BLEND"))
}
