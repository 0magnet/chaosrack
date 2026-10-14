//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/acoustics"

	"github.com/0magnet/chaosrack/pkg/glctx"
)

// The RTA mode — fractional-octave bands drawn as a bar display.
//
// The band arithmetic is in pkg/acoustics and checked against the Test
// module's own pink noise, and the meter and its bars are RTA (rtameter.go),
// which a native rack draws too. This is the page's side: reading the tap,
// and drawing the lines.
//
// It draws with the per-vertex-color program (vcolor_js.go). Bars are
// vertical LINES rather than quads: at any usable band count a bar is a few
// pixels wide, the additive halo the scope already uses makes a line read as
// a lit bar, and lines cost two vertices where a quad costs six.
//
// ── IT DOES NOT ANALYZE EVERY FRAME ──────────────────────────────────────
//
// The FFT runs on its own clock and the DISPLAY runs every frame, which are
// different things: between analyses the bars go on moving because the
// smoothing and the peak-hold decay are per-frame. That is what a real
// analyzer's ballistics are, and it is why the picture is smooth at 60 Hz while
// the measurement underneath it is not.

// rtaMode is the RTA mode: the meter, and the window of the tap it reads.
type rtaMode struct {
	RTA
	cursor int
	buf    []float32
	fill   int
}

var rta = rtaMode{cursor: tapUnjoined, RTA: *NewRTA()}

func init() {
	registerGenerate("rta", generateRTA)
	attractorParams["rta"] = []paramDef{
		{"rta-frac", "band", &rta.fracF, 1, 0, float32(len(acoustics.RTAFractions) - 1), 1},
		{"rta-chan", "src", &rta.chanF, 0, 0, float32(len(tapChanNames) - 1), 1},
		{"rta-top", "top", &rta.topF, 0, -60, 20, 1},
		{"rta-range", "rnge", &rta.rangeF, 70, 20, 120, 5},
		{"rta-avg", "avg", &rta.avgF, 3, 0, 10, 1},
		{"rta-hold", "hold", &rta.holdF, 1, 0, 40, 1},
	}
}

// analyze drains the tap and runs the FFT when its period is up.
func (r *rtaMode) analyze(nowMs float64) {
	if r.buf == nil {
		r.buf = make([]float32, RTAFFT)
	}
	var scratch [4096]float32
	ch := tapChanSel(r.chanF)
	for {
		n := tap.readChan(&r.cursor, scratch[:], ch)
		if n <= 0 {
			break
		}
		if n >= RTAFFT {
			copy(r.buf, scratch[n-RTAFFT:n])
			r.fill = RTAFFT
		} else {
			copy(r.buf, r.buf[n:])
			copy(r.buf[RTAFFT-n:], scratch[:n])
			if r.fill += n; r.fill > RTAFFT {
				r.fill = RTAFFT
			}
		}
		if n < len(scratch) {
			break
		}
	}
	if r.fill < RTAFFT || !r.Due(nowMs) {
		return
	}
	r.Analyze(r.buf, takensSourceRate())
}

// generateRTA is the mode's frame: analyze, advance, draw.
func generateRTA() {
	rta.analyze(frameNowMs)
	rta.Advance(1)
	rta.drawRTA()
}

// drawRTA draws the bars and the peak-hold marks.
func (r *rtaMode) drawRTA() {
	vc.initVColor()
	bars, marks := r.Lines(analyzerColors())
	if len(bars) == 0 {
		return
	}
	vc.fit(len(bars) + len(marks))
	v := 0
	for _, l := range [][]PlotVertex{bars, marks} {
		for _, p := range l {
			vc.put(v, p.X, p.Y, p.C)
			v++
		}
	}

	glctx.GL.Call("disable", glctx.Types.DepthTest)
	glctx.GL.Call("clearColor", 0, 0, 0, 0)
	glctx.GL.Call("clear", glctx.Types.ColorBufferBit)
	glctx.GL.Call("enable", glctx.GL.Get("BLEND"))
	glctx.GL.Call("blendFunc", glctx.GL.Get("SRC_ALPHA"), glctx.GL.Get("ONE"))

	// The bars, widened the way the scope's trace is: WebGL cannot be relied on
	// for lineWidth, so each is drawn several times at sub-pixel offsets.
	vc.upload(v)
	dx := float32(1.0) / float32(gpu.width)
	for k := -2; k <= 2; k++ {
		vc.span(glctx.Types.Lines, 0, len(bars), 0.5, float32(k)*dx, 0)
	}
	// The peak marks once and brighter: they are a held maximum rather than a
	// level, and widening them would make them read as bars of their own.
	vc.span(glctx.Types.Lines, len(bars), len(marks), 0.9, 0, 0)
	vc.done()
	glctx.GL.Call("disable", glctx.GL.Get("BLEND"))
}
