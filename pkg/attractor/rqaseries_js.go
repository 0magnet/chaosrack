//go:build js && wasm

package attractor

import (
	"strconv"
	"strings"
	"syscall/js"
)

// The RQA strip chart: RR, DET and LAM over the last RQASeriesSpanMs, scrolling
// beside the recurrence plot. The ring and the scales are in rqaseries.go,
// untagged; this file is the cell it lives in and the pixels.
//
// ── WHERE IT LIVES, AND THE THREE PLACES IT DOES NOT ─────────────────────
//
// In the recurrence mode's own parameter grid, next to the RQA readout, added
// by the same panel build and for the same reason that readout gives: these
// numbers describe THIS PLOT AT THIS ε rather than the system, so they belong
// under the knob that moves them. The chart is the readout's history and the
// readout is the chart's present value; separating them would put a number and
// its own past in two different modules.
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
// So: a panel cell, built the way appendTakensEstimate and appendRecurrenceRQA
// build theirs, holding a 2-D canvas drawn at the sampling tick — the desk
// monitor's and the record preview's arrangement, down to the offsetParent
// guard that keeps it from painting into a module nobody has open.
//
// ── WHAT IT COSTS ────────────────────────────────────────────────────────
//
// Nothing is measured here. The chart is fed from rpMaybeMeasure, which has
// rate-limited the RQA scan to RQASamplePeriodMs since the readout existed; all
// this does is keep the answers. A knob drag therefore cannot provoke a storm
// of recomputation through this path, because this path recomputes nothing —
// the settle delay on the trajectory source and the ε cache in recurrence_js.go
// are still the only things deciding how often the expensive work happens, and
// the chart is downstream of both.
//
// The drawing is a full redraw of RQASeriesLen columns × 3 panes, at 6.25 Hz.
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
	// RQASeriesLen because the ring is sized for the chart rather than the
	// other way round — history that is never drawn is not history.
	rqaChartCols = RQASeriesLen

	// rqaPaneH is one trace's pane, in backing-store pixels. --krow is 38 mm =
	// 152 px at interface scale 1, so at that scale the backing store and the
	// CSS box are 1:1 down the whole chart and nothing is resampled. Other
	// interface sizes scale the box and let the browser resample, which is what
	// the desk monitor does with its own fixed backing store: a line chart
	// upscaled twofold is a two-pixel line, which is not a defect.
	rqaPaneH = 152

	// rqaChartH is the whole canvas: one pane per trace, stacked, so the cell
	// is exactly the three grid rows tall that it spans.
	rqaChartH = rqaPaneH * int(RQATraceCount)

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
var rqaTraceColor = [RQATraceCount]string{
	RQATraceRR:  "#ff3b30",
	RQATraceDET: "#5ad1ff",
	RQATraceLAM: "#ffd24a",
}

var (
	rqaSeries RQASeries
	rqaSnap   = make([]RQASample, rqaChartCols) // reused; Snapshot fills it whole

	rqaChartEl  js.Value
	rqaChartCtx js.Value
)

// rqaConfig is everything that decides WHAT is being measured, as one
// comparable value. When it changes, the readings before and after are answers
// to different questions and the series takes a seam — see the gap note in
// rqaseries.go.
//
// Compared by VALUE rather than hooked off the knobs, for the reason
// rpTrajChanged gives about the trajectory cache: an edit that reaches a
// parameter by any route at all — knob, permalink, preset, Reset All, MIDI,
// audio modulation — moves a float that this then sees, and nothing added later
// can forget to announce itself.
type rqaConfig struct {
	src, win, eps float32
	dim, tau      float32
	traj          int // bumped whenever the trajectory source re-integrates
}

var (
	rqaCfg     rqaConfig
	rqaCfgHave bool
)

// rqaConfigNow reads the current settings.
//
// m and τ are only included for the embed source, because they only mean
// anything there: the raw-audio path is m = 1 whatever the knob says, and
// turning τ while watching raw audio must not put a seam through a trace that
// did not change.
func rqaConfigNow() rqaConfig {
	c := rqaConfig{src: rpSrc, win: rpWin, eps: rpEps, traj: rpTrajGen}
	if int(rpSrc) == rpSrcEmbed {
		c.dim, c.tau = float32(rpEmbedDim()), takensTau
	}
	return c
}

// rqaSample records one measurement and repaints. Called from rpMaybeMeasure,
// on its tick, with whatever the readout is showing — including a result with
// nothing lit, which Push stores as a gap rather than as three zeros.
func rqaSample(nowMs float64, r RQAResult) {
	if cfg := rqaConfigNow(); cfg != rqaCfg {
		// Not on the first sample: there is no history for the seam to
		// separate, and a chart that opens with a break in it reads as a fault.
		if rqaCfgHave {
			rqaSeries.Break(nowMs)
		}
		rqaCfg, rqaCfgHave = cfg, true
	}
	rqaSeries.Push(nowMs, r)
	rqaPaint()
}

// rqaPaneY maps a 0..1 height within a pane to a canvas y.
//
// Inset a pixel top and bottom so a reading pinned at either end — DET at 1.00
// on a clean periodic orbit is the common one — draws as a line inside the pane
// rather than as half a line on its border.
func rqaPaneY(top int, f float64) float64 {
	return float64(top) + 1 + float64(rqaPaneH-3)*(1-f)
}

// rqaPaint redraws the whole chart from the ring.
//
// Skipped when nothing can see it: offsetParent is null while the Parameters
// module is hidden, and the rack re-measures every module on each pointer move
// during a panel resize, which is how a drag comes to cost the model a frame.
// The desk monitor is guarded the same way for the same two reasons. Sampling
// is NOT skipped with it — the ring keeps filling while the module is shut, so
// opening it shows the history that was there rather than a hole the size of
// however long it was closed.
func rqaPaint() {
	if !rqaChartCtx.Truthy() || !rqaChartEl.Get("offsetParent").Truthy() || resizing {
		return
	}
	ctx := rqaChartCtx
	n := rqaSeries.Snapshot(rqaSnap)

	ctx.Set("fillStyle", rqaColBg)
	ctx.Call("fillRect", 0, 0, rqaChartCols, rqaChartH)

	// The time rules run the full height, under everything, so a step in one
	// pane can be read against a step in another.
	ctx.Set("fillStyle", rqaColRule)
	for ms := rqaTimeTickMs; ms < RQASeriesSpanMs; ms += rqaTimeTickMs {
		x := float64(rqaChartCols) - float64(ms)/RQASamplePeriodMs
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

	for tr := RQATrace(0); tr < RQATraceCount; tr++ {
		top := int(tr) * rqaPaneH
		if tr == RQATraceRR {
			// The band the plot is readable in, shaded behind the trace: below
			// it there is nothing but the diagonal, above it the square
			// saturates. It turns "turn ε until RR is a few percent" from a
			// sentence in a tooltip into somewhere to aim.
			hi := rqaPaneY(top, RQATraceY(tr, RQAReadableHi))
			lo := rqaPaneY(top, RQATraceY(tr, RQAReadableLo))
			ctx.Set("fillStyle", rqaColBand)
			ctx.Call("fillRect", 0, hi, rqaChartCols, lo-hi)
		} else {
			// Half scale. DET and LAM are drawn as themselves, so this line is
			// the only reference either pane needs.
			ctx.Set("fillStyle", rqaColRule)
			ctx.Call("fillRect", 0, int(rqaPaneY(top, 0.5)), rqaChartCols, 1)
		}
		if tr+1 < RQATraceCount {
			ctx.Set("fillStyle", rqaColEdge)
			ctx.Call("fillRect", 0, top+rqaPaneH-1, rqaChartCols, 1)
		}
		if n > 0 {
			if d := rqaTracePath(tr, top); d != "" {
				ctx.Set("strokeStyle", rqaTraceColor[tr])
				ctx.Call("stroke", js.Global().Get("Path2D").New(d))
			}
		}
		ctx.Set("fillStyle", rqaColText)
		ctx.Call("fillText", tr.String(), 4, top+3)
	}
}

// rqaTracePath builds one trace as an SVG path, broken across the slots with no
// measurement in them. Empty when the whole window is a gap.
//
// A string handed to Path2D rather than a run of moveTo/lineTo calls: see the
// note on the boundary at the top of this file. Coordinates get one decimal,
// which is exact for the x (column + a half) and a tenth of a pixel on the y,
// well under the resampling the CSS box does at any interface size but 1.
func rqaTracePath(tr RQATrace, top int) string {
	var b strings.Builder
	pen := false
	for i, s := range rqaSnap {
		if !s.OK {
			pen = false // a hole in the record is a hole in the line
			continue
		}
		// Column i is the i-th oldest slot, so time runs left to right and the
		// newest reading is against the right edge.
		x := strconv.FormatFloat(float64(i)+0.5, 'f', 1, 64)
		y := strconv.FormatFloat(rqaPaneY(top, RQATraceY(tr, s.Value(tr))), 'f', 1, 64)
		if !pen {
			// Opened as a degenerate segment so a lone reading is a round dot
			// rather than nothing; see the lineCap in rqaPaint.
			b.WriteString("M" + x + " " + y + "L")
			pen = true
		} else {
			b.WriteString("L")
		}
		b.WriteString(x + " " + y)
	}
	return b.String()
}

// appendRecurrenceSeries adds the strip chart to the recurrence parameter grid,
// into the GRID rather than below it for the reason appendRecurrenceRQA gives:
// the grid is the height-bounded column-wrap container and anything appended
// after it is clipped.
//
// It spans a whole column group — all three rows, two columns wide — rather
// than sitting in one cell like the knobs. A strip chart's information is in
// its width: one 116 px cell would be 18 seconds of history, against 41 in two,
// and 41 is the span a transition and enough context to read it against fit
// inside. Taking all three rows is also what makes the placement safe, because
// a full-height item cannot be interleaved with the knob cells by the grid's
// column auto-flow — it takes its own column group and the knobs keep theirs.
// The module widens to hold it, which is what the Parameters module is built to
// do (its content column-wraps within a fixed height; more content is more
// columns, not a taller module).
func appendRecurrenceSeries(grid js.Value) {
	card := doc.Call("createElement", "div")
	card.Set("className", "punit")
	// Inline, because #params .punit pins every cell to one --kcol by --krow.
	// justify-content is reset to the top so the label sits above the chart and
	// the chart takes the rest, rather than the pair being centered with slack
	// above and below.
	card.Get("style").Set("cssText",
		"grid-row:1/-1;grid-column:span 2;width:100%;height:100%;justify-content:flex-start;gap:2px;")

	lbl := doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", false))
	lbl.Set("textContent", "trend")
	card.Call("appendChild", lbl)

	rqaChartEl = doc.Call("createElement", "canvas")
	rqaChartEl.Set("width", rqaChartCols)
	rqaChartEl.Set("height", rqaChartH)
	rqaChartEl.Set("title", "Recurrence quantification over time — the last "+
		strconv.Itoa(RQASeriesSpanMs/1000)+" seconds, newest at the right, one column per measurement "+
		"(about six a second). Vertical rules every 10 s. "+
		"THE THREE PANES DO NOT SHARE A SCALE and their heights are not comparable; what they share is "+
		"the time axis, which is the only one they have in common. "+
		"RR is the density, drawn as its square root because RR is a lit fraction of a SQUARE and the "+
		"root of an area is the fraction of the side — the shaded band is the 1–5% the plot is readable "+
		"in, so turn ε until the trace sits in it. "+
		"DET and LAM are drawn as themselves, 0 at the bottom and 1 at the top: DET climbing is structure "+
		"appearing, DET falling off is noise or a change of regime. "+
		"A break in a trace is never a value — it is a stretch with no measurement behind it, either "+
		"because the tab was not being drawn or because a knob changed what was being measured.")
	// Fixed backing store, CSS box sized by the grid: see rqaPaneH. min-height
	// is zeroed because a flex item's default min-height:auto would refuse to
	// shrink below the canvas's intrinsic height and overflow the module.
	rqaChartEl.Get("style").Set("cssText",
		"display:block;width:100%;flex:1 1 auto;min-height:0;box-sizing:border-box;"+
			"background:"+rqaColBg+";border:1px solid "+rqaColEdge+";border-radius:2px;")
	card.Call("appendChild", rqaChartEl)
	rqaChartCtx = rqaChartEl.Call("getContext", "2d")

	grid.Call("appendChild", card)
	// Seeded from the ring rather than left blank, for the reason the stereo
	// readout is: the panel is rebuilt on every mode change and every module
	// toggle, and a chart that came back empty over a series that has forty
	// seconds in it would be reporting a stall that never happened.
	rqaPaint()
}
