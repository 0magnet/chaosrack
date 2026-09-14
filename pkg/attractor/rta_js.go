//go:build js && wasm

package attractor

import (
	"syscall/js"
)

// The RTA mode — fractional-octave bands drawn as a bar display.
//
// The band arithmetic is in rta.go, untagged and checked against the Test
// module's own pink noise. This is the picture: how the bars are drawn, how
// often the analysis runs, and the knobs.
//
// It draws with the xy scope's program — a pass-through vec2 vertex shader and
// a solid-colour fragment — because that is exactly what a bar display needs
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
	rtaCursor  = tapUnjoined
	rtaBuf     []float32
	rtaFill    int
	rtaNextMs  float64
	rtaBands   []RTABand
	rtaLevels  []float64 // this analysis
	rtaHeld    []float64 // after the meter ballistics
	rtaPeaks   []float64
	rtaLastB   int
	rtaLine    []float32
	rtaJsUint8 js.Value
	rtaJsFloat js.Value

	// The knobs.
	rtaFracF  float32 = 1  // index into rtaFractions; 1 is third-octave
	rtaChanF  float32      // which signal
	rtaRangeF float32 = 70 // dB shown from the top of the scale down
	rtaTopF   float32      // dBFS at the top of the display
	rtaAvgF   float32 = 3  // averaging, 0 = none
	rtaHoldF  float32 = 1  // peak hold: 0 off, else decay in dB/s
)

func init() {
	registerGenerate("rta", generateRTA)
	attractorParams["rta"] = []paramDef{
		{"rta-frac", "band", &rtaFracF, 1, 0, float32(len(rtaFractions) - 1), 1},
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
		return rtaFractions[0]
	}
	last := len(rtaFractions) - 1
	if v > float32(last) {
		return rtaFractions[last]
	}
	return rtaFractions[int(v+0.5)]
}

// rtaAnalyze drains the tap and runs the FFT when its period is up.
func rtaAnalyze(nowMs float64) {
	b := rtaFraction()
	if b != rtaLastB || rtaBands == nil {
		rtaLastB = b
		rtaBands = RTABands(b)
		rtaLevels = make([]float64, len(rtaBands))
		rtaHeld = make([]float64, len(rtaBands))
		rtaPeaks = make([]float64, len(rtaBands))
		for i := range rtaHeld {
			rtaHeld[i] = rtaFloorDB
			rtaPeaks[i] = rtaFloorDB
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
	RTALevels(computeFFTMagsKind(rtaBuf, rtaWindowKind), rtaFFT, takensSourceRate(),
		rtaBands, rtaWindowKind, rtaLevels)
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
	RTASmooth(rtaHeld, rtaLevels, rise, fall)
	if rtaHoldF > 0 {
		// The knob is dB per second; the decay is per frame.
		RTAPeakHold(rtaPeaks, rtaHeld, float64(rtaHoldF)/60)
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
func drawRTA() {
	if !xyReady {
		initXY()
	}
	n := len(rtaBands)
	if n == 0 {
		return
	}
	// Two vertices per bar, plus two per peak mark.
	need := n * 8
	if len(rtaLine) < need {
		rtaLine = make([]float32, need+need/2)
		rtaJsUint8 = js.Global().Get("Uint8Array").New(len(rtaLine) * 4)
		rtaJsFloat = js.Global().Get("Float32Array").New(rtaJsUint8.Get("buffer"), 0, len(rtaLine))
	}
	// Bars are spaced evenly across the screen rather than by frequency: the
	// bands are already equal RATIOS, so equal widths is what puts a logarithmic
	// frequency axis on the display. That is the whole visual point of a
	// fractional-octave analyzer over a spectrogram's linear bins.
	bottom := rtaY(rtaFloorDB)
	o := 0
	for i := range rtaBands {
		x := float32(-0.9 + 1.8*(float64(i)+0.5)/float64(n))
		rtaLine[o], rtaLine[o+1] = x, bottom
		rtaLine[o+2], rtaLine[o+3] = x, rtaY(rtaHeld[i])
		o += 4
	}
	barVerts := o / 2
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
		rtaLine[o], rtaLine[o+1] = x-half, y
		rtaLine[o+2], rtaLine[o+3] = x+half, y
		o += 4
	}
	peakVerts := o/2 - barVerts

	gl.Call("disable", glTypes.DepthTest)
	gl.Call("clearColor", 0, 0, 0, 0)
	gl.Call("clear", glTypes.ColorBufferBit)

	gl.Call("useProgram", xyProgram)
	gl.Call("bindBuffer", glTypes.ArrayBuffer, xyBuf)
	js.CopyBytesToJS(rtaJsUint8, sliceToByteSlice(rtaLine))
	gl.Call("bufferData", glTypes.ArrayBuffer, rtaJsFloat, glTypes.DynamicDraw)
	gl.Call("enableVertexAttribArray", xyAPos)
	gl.Call("vertexAttribPointer", xyAPos, 2, glTypes.Float, false, 0, 0)

	col := [3]float32{0.4, 1.0, 0.45}
	if phosphorActive() {
		p := phosphors[phosphorIdx]
		col = [3]float32{float32(p.tr), float32(p.tg), float32(p.tb)}
	}
	gl.Call("enable", gl.Get("BLEND"))
	gl.Call("blendFunc", gl.Get("SRC_ALPHA"), gl.Get("ONE"))
	gl.Call("uniform2f", xyUOffset, 0, 0)

	// The bars, widened the way the scope's trace is: WebGL cannot be relied on
	// for lineWidth, so a bar is drawn several times at sub-pixel offsets.
	gl.Call("uniform3f", xyUColor, col[0], col[1], col[2])
	dx := float32(1.0) / float32(width)
	for k := -2; k <= 2; k++ {
		gl.Call("uniform2f", xyUOffset, float32(k)*dx, 0)
		gl.Call("uniform1f", xyUAlpha, 0.5)
		gl.Call("drawArrays", glTypes.Lines, 0, barVerts)
	}
	// The peak marks, dimmer and in the same colour, so they read as a held
	// maximum rather than as a second measurement.
	gl.Call("uniform2f", xyUOffset, 0, 0)
	gl.Call("uniform1f", xyUAlpha, 0.9)
	gl.Call("drawArrays", glTypes.Lines, barVerts, peakVerts)
	gl.Call("disable", gl.Get("BLEND"))
}
