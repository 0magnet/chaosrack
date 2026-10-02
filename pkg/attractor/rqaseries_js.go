//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/recurrence"
	"strconv"
	"strings"
	"syscall/js"
)

// The RQA strip chart: RR, DET and LAM over the last recurrence.RQASeriesSpanMs,
// scrolling beside the recurrence plot. The ring and the scales are in
// pkg/recurrence, untagged; this file is the pixels.
//
// ── WHERE IT LIVES, AND THE THREE PLACES IT DOES NOT ─────────────────────
//
// On the Visual head's screen, while recurrence is running and the head's
// TREND switch is on (rqaTrendOn). That screen is the model's monitor, and for
// recurrence the model is already on the main canvas, so the glass beside the
// RR, DET and LAM readouts is free to carry their history: a number and its
// own past, side by side. It was a cell three rows tall in a Parameters
// module that no other model had, which is what put an extra bay on the rack.
//
// The alternatives, and why each is wrong rather than merely not chosen:
//
//   - A BACKDROP LAYER. The backdrop (backdrop_js.go) is a full-screen visual
//     drawn behind ANY ordinary model — that generality is the whole idea of
//     it, and it is exactly what this cannot have. RR, DET and LAM only exist
//     while the recurrence matrix is being built, which happens in one mode; a
//     backdrop that is blank in forty modes and lit in one is not a layer, it
//     is a mode's display filed in the wrong place. bgVisualActive() also turns
//     the backdrop off for the audio modes outright, so in the one mode where
//     there would be data to draw it would not run at all.
//
//   - ITS OWN MODEL. A model is what is on the canvas INSTEAD of the recurrence
//     plot, so an RQA-history model would be plotting the history of a
//     measurement it had just stopped taking. Making it work means running the
//     whole matrix pipeline — audio ring, decimation, embedding, 65536
//     comparisons a frame — for a mode that never shows the matrix, which is
//     the expensive half of the feature paid for twice.
//
//   - THE ANALYSIS MODULE. Already argued and already settled next door: the
//     Analysis module measures the model on screen and switches on and off for
//     the whole rack, while these three move when ε moves. Filed there they
//     would read as facts about the attractor.
//
// So: a 2-D canvas off the page, drawn at the sampling tick while the screen
// is showing it, and copied onto the screen by the monitor's own paint
// (drawRowMonitor), the way the monitor copies the model.
//
// ── WHAT IT COSTS ────────────────────────────────────────────────────────
//
// Nothing is measured here. The chart is fed from rp.maybeMeasure, which has
// rate-limited the RQA scan to recurrence.RQASamplePeriodMs since the readout existed; all
// this does is keep the answers. A knob drag therefore cannot provoke a storm
// of recomputation through this path, because this path recomputes nothing —
// the settle delay on the trajectory source and the ε cache in recurrence_js.go
// are still the only things deciding how often the expensive work happens, and
// the chart is downstream of both.
//
// The drawing is a full redraw of recurrence.RQASeriesLen columns × 3 panes, at 6.25 Hz.
// The spectrogram scrolls its texture in place — one column uploaded per step,
// the read offset advanced — because a full re-upload there is 2048×512 RGBA,
// four megabytes a frame. Here a full redraw is a 256×456 canvas six times a
// second, and doing it the spectrogram's way (blit the canvas onto itself
// shifted by one pixel, draw the new column into the gap) would buy nothing and
// cost the property that makes this simple: what is drawn is always exactly
// what the ring holds, so a chart that disagrees with the data is impossible
// rather than merely unlikely.
//
// The one thing that IS optimized is the boundary. A polyline of 256 points
// drawn with moveTo/lineTo is 256 calls out of wasm into JS, three times over,
// six times a second — about five thousand crossings a second to draw three
// lines. The path is built as a string in Go instead and handed over as one
// Path2D, which makes it three crossings per redraw. Path2D has been in every
// browser that can run this since long before WebGL 1 was universal, and there
// is no fallback for the same reason there is none for WebGL.

const (
	// rqaChartCols is the chart's width in backing-store pixels, and so how
	// many samples it can show: one per column, the spectrogram's rule. It is
	// recurrence.RQASeriesLen because the ring is sized for the chart rather than the
	// other way round — history that is never drawn is not history.
	rqaChartCols = recurrence.RQASeriesLen

	// rqaPaneH is one trace's pane, in backing-store pixels: a third of the
	// head screen's height, so the chart's backing store is the screen's own
	// size down the whole chart and the monitor copies it without resampling
	// the traces (across, it is one column a sample, a little wider than the
	// screen, and squeezed).
	rqaPaneH = catMonitorHigh / int(recurrence.RQATraceCount)

	// rqaChartH is the whole canvas: one pane per trace, stacked.
	rqaChartH = rqaPaneH * int(recurrence.RQATraceCount)

	// rqaTimeTickMs is the spacing of the faint vertical rules. Ten seconds is
	// 62.5 columns here — close enough to a rule every inch of chart to read a
	// transition's duration off without a legend, and few enough rules (four
	// across the span) that they stay behind the traces.
	rqaTimeTickMs = 10000
)

// The chart's colors. Dark ground and a rule set that sits under the traces,
// because the traces are the only thing here anyone is reading.
const (
	rqaColBg   = "#0a0d10" // pane ground
	rqaColRule = "#1b222a" // the 0.5 gridline and the time rules
	rqaColEdge = "#2a333c" // pane separators, the panel's own border color
	rqaColBand = "rgba(255,59,48,0.13)"
	rqaColText = "#7d94a6"
)

// rqaTraceColor is one color per trace. RR takes the LED red the readout beside
// it is drawn in — it is the same number, and it is the first field of that
// readout — and the other two are picked to stay apart from it and from each
// other on a dark ground rather than to mean anything.
var rqaTraceColor = [recurrence.RQATraceCount]string{
	recurrence.RQATraceRR:  "#ff3b30",
	recurrence.RQATraceDET: "#5ad1ff",
	recurrence.RQATraceLAM: "#ffd24a",
}

// rqaChart is the RQA strip chart.
type rqaChart struct {
	series   recurrence.RQASeries
	snap     []recurrence.RQASample // reused; Snapshot fills it whole
	chartEl  js.Value
	chartCtx js.Value
	cfg      rqaConfig
	cfgHave  bool
}

var rqa = rqaChart{
	snap: make([]recurrence.RQASample, rqaChartCols),
}

// rqaConfig is everything that decides WHAT is being measured, as one
// comparable value. When it changes, the readings before and after are answers
// to different questions and the series takes a seam — see the gap note in
// pkg/recurrence.
//
// Compared by VALUE rather than hooked off the knobs, for the reason
// rp.trajChanged gives about the trajectory cache: an edit that reaches a
// parameter by any route at all — knob, permalink, preset, Reset All, MIDI,
// audio modulation — moves a float that this then sees, and nothing added later
// can forget to announce itself.
type rqaConfig struct {
	src, win, eps float32
	dim, tau      float32
	traj          int // bumped whenever the trajectory source re-integrates
}

// rqaConfigNow reads the current settings.
//
// m and τ are only included for the embed source, because they only mean
// anything there: the raw-audio path is m = 1 whatever the knob says, and
// turning τ while watching raw audio must not put a seam through a trace that
// did not change.
func rqaConfigNow() rqaConfig {
	c := rqaConfig{src: rp.src, win: rp.win, eps: rp.eps, traj: rp.trajGen}
	if int(rp.src) == rpSrcEmbed {
		c.dim, c.tau = float32(rp.embedDim()), emb.tau
	}
	return c
}

// sample records one measurement and repaints. Called from rp.maybeMeasure,
// on its tick, with whatever the readout is showing — including a result with
// nothing lit, which Push stores as a gap rather than as three zeros.
func (rq *rqaChart) sample(nowMs float64, r recurrence.RQAResult) {
	if cfg := rqaConfigNow(); cfg != rq.cfg {
		// Not on the first sample: there is no history for the seam to
		// separate, and a chart that opens with a break in it reads as a fault.
		if rq.cfgHave {
			rq.series.Break(nowMs)
		}
		rq.cfg, rq.cfgHave = cfg, true
	}
	rq.series.Push(nowMs, r)
	rq.paint()
}

// rqaPaneY maps a 0..1 height within a pane to a canvas y.
//
// Inset a pixel top and bottom so a reading pinned at either end — DET at 1.00
// on a clean periodic orbit is the common one — draws as a line inside the pane
// rather than as half a line on its border.
func rqaPaneY(top int, f float64) float64 {
	return float64(top) + 1 + float64(rqaPaneH-3)*(1-f)
}

// paint redraws the whole chart from the ring.
//
// Skipped when nothing can see it: when the head screen is not showing it
// (rqaTrendShown), and during a panel resize, when the rack re-measures every
// module on each pointer move and a drag would come to cost the model a
// frame. Sampling is NOT skipped with it: the ring keeps filling while the
// chart is off the screen, so turning it back on shows the history that was
// there rather than a hole the size of however long it was away.
func (rq *rqaChart) paint() {
	if !rqaTrendShown() || layout.resizing {
		return
	}
	rq.ensureCanvas()
	ctx := rq.chartCtx
	n := rq.series.Snapshot(rq.snap)

	ctx.Set("fillStyle", rqaColBg)
	ctx.Call("fillRect", 0, 0, rqaChartCols, rqaChartH)

	// The time rules run the full height, under everything, so a step in one
	// pane can be read against a step in another.
	ctx.Set("fillStyle", rqaColRule)
	for ms := rqaTimeTickMs; ms < recurrence.RQASeriesSpanMs; ms += rqaTimeTickMs {
		x := float64(rqaChartCols) - float64(ms)/recurrence.RQASamplePeriodMs
		if x < 0 {
			break
		}
		ctx.Call("fillRect", int(x), 0, 1, rqaChartH)
	}

	ctx.Set("lineWidth", 1)
	// Round caps and joins so an isolated reading between two gaps draws as a
	// dot: a single measurement either side of a stall is real data, and a
	// butt-capped zero-length segment renders nothing at all.
	ctx.Set("lineCap", "round")
	ctx.Set("lineJoin", "round")
	ctx.Set("font", "9px 'Chakra Petch',sans-serif")
	ctx.Set("textBaseline", "top")

	for tr := range recurrence.RQATraceCount {
		top := int(tr) * rqaPaneH
		if tr == recurrence.RQATraceRR {
			// The band the plot is readable in, shaded behind the trace: below
			// it there is nothing but the diagonal, above it the square
			// saturates. It turns "turn ε until RR is a few percent" from a
			// sentence in a tooltip into somewhere to aim.
			hi := rqaPaneY(top, recurrence.RQATraceY(tr, recurrence.RQAReadableHi))
			lo := rqaPaneY(top, recurrence.RQATraceY(tr, recurrence.RQAReadableLo))
			ctx.Set("fillStyle", rqaColBand)
			ctx.Call("fillRect", 0, hi, rqaChartCols, lo-hi)
		} else {
			// Half scale. DET and LAM are drawn as themselves, so this line is
			// the only reference either pane needs.
			ctx.Set("fillStyle", rqaColRule)
			ctx.Call("fillRect", 0, int(rqaPaneY(top, 0.5)), rqaChartCols, 1)
		}
		if tr+1 < recurrence.RQATraceCount {
			ctx.Set("fillStyle", rqaColEdge)
			ctx.Call("fillRect", 0, top+rqaPaneH-1, rqaChartCols, 1)
		}
		if n > 0 {
			if d := rq.tracePath(tr, top); d != "" {
				ctx.Set("strokeStyle", rqaTraceColor[tr])
				ctx.Call("stroke", js.Global().Get("Path2D").New(d))
			}
		}
		ctx.Set("fillStyle", rqaColText)
		ctx.Call("fillText", tr.String(), 4, top+3)
	}
}

// tracePath builds one trace as an SVG path, broken across the slots with no
// measurement in them. Empty when the whole window is a gap.
//
// A string handed to Path2D rather than a run of moveTo/lineTo calls: see the
// note on the boundary at the top of this file. Coordinates get one decimal,
// which is exact for the x (column + a half) and a tenth of a pixel on the y,
// well under the resampling the CSS box does at any interface size but 1.
func (rq *rqaChart) tracePath(tr recurrence.RQATrace, top int) string {
	var b strings.Builder
	pen := false
	for i, s := range rq.snap {
		if !s.OK {
			pen = false // a hole in the record is a hole in the line
			continue
		}
		// Column i is the i-th oldest slot, so time runs left to right and the
		// newest reading is against the right edge.
		x := strconv.FormatFloat(float64(i)+0.5, 'f', 1, 64)
		y := strconv.FormatFloat(rqaPaneY(top, recurrence.RQATraceY(tr, s.Value(tr))), 'f', 1, 64)
		if !pen {
			// Opened as a degenerate segment so a lone reading is a round dot
			// rather than nothing; see the lineCap in rqa.paint.
			b.WriteString("M" + x + " " + y + "L")
			pen = true
		} else {
			b.WriteString("L")
		}
		b.WriteString(x + " " + y)
	}
	return b.String()
}

// rqaTrendOn is the head's TREND switch for recurrence: the screen shows the
// chart, or the model as it does for every other one. On from the start,
// because the chart is what RQA is read by.
var rqaTrendOn = true

// rqaTrendShown is whether the head screen is showing the chart now.
func rqaTrendShown() bool { return rqaTrendOn && run.selectedMode == "recurrence" }

// ensureCanvas makes the chart's canvas the first time it is drawn. It is
// never on the page: the monitor copies it onto the screen.
func (rq *rqaChart) ensureCanvas() {
	if rq.chartCtx.Truthy() {
		return
	}
	rq.chartEl = dom.Doc.Call("createElement", "canvas")
	rq.chartEl.Set("width", rqaChartCols)
	rq.chartEl.Set("height", rqaChartH)
	rq.chartCtx = rq.chartEl.Call("getContext", "2d")
}

// drawOnMonitor puts the chart on a head screen of pw by ph, filling it: a
// strip chart has no shape to keep, unlike a model, and its information is
// in its width.
func (rq *rqaChart) drawOnMonitor(ctx js.Value, pw, ph float64) {
	rq.ensureCanvas()
	ctx.Call("drawImage", rq.chartEl, 0, 0, rqaChartCols, rqaChartH, 0, 0, pw, ph)
}

// rqaSource is what the plot is of, for the screen's caption: a plot of an
// unnamed system is not a measurement of anything. The trajectory source
// plots whichever flow was on screen last, the bifurcation explorer's rule.
func rqaSource() string {
	if int(rp.src) == rpSrcTraj {
		return modeInfo[bif.lastFlowMode].Label
	}
	return "audio in"
}

// rqaTrendTip is the TREND switch's tooltip, which is also the chart's key.
var rqaTrendTip = docf("sw.recurrence.trnd", "seconds", strconv.Itoa(recurrence.RQASeriesSpanMs/1000))
