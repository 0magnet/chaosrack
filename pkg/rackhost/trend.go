package rackhost

import (
	"image"
	"image/color"

	"github.com/0magnet/chaosrack/pkg/recurrence"
)

// The RQA trend, natively: RR, DET and LAM over the last
// recurrence.RQASeriesSpanMs, on the model's monitor while recurrence runs
// with the head's TREND switch up — the page's chart (rqaseries_js.go, which
// says why it lives there), drawn from the same ring and scales.

// rqaTrend is the record and what it is a record of.
type rqaTrend struct {
	series recurrence.RQASeries
	seam   string
	begun  bool
	snap   []recurrence.RQASample
}

// sample records one reading at nowMs, of a plot whose settings seam says.
// A change of settings breaks the trace, but not the first: a chart that
// opens with a break in it reads as a fault.
func (t *rqaTrend) sample(nowMs float64, seam string, res recurrence.RQAResult) {
	if seam != t.seam {
		if t.begun {
			t.series.Break(nowMs)
		}
		t.seam, t.begun = seam, true
	}
	t.series.Push(nowMs, res)
}

// The chart's colors, the page's.
var (
	trendGround = color.RGBA{10, 13, 16, 255}
	trendRule   = color.RGBA{27, 34, 42, 255}
	trendEdge   = color.RGBA{42, 51, 60, 255}
	trendBand   = color.RGBA{255, 59, 48, 255} // at 0.13
	trendText   = color.RGBA{125, 148, 166, 255}
	trendTrace  = [recurrence.RQATraceCount]color.RGBA{
		recurrence.RQATraceRR:  {255, 59, 48, 255},
		recurrence.RQATraceDET: {90, 209, 255, 255},
		recurrence.RQATraceLAM: {255, 210, 74, 255},
	}
)

// trendTickMs is how often a time rule crosses the chart.
const trendTickMs = 10000

// draw paints the chart over all of m: three panes, a trace each, newest at
// the right edge.
func (t *rqaTrend) draw(m *image.RGBA) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	if len(t.snap) != recurrence.RQASeriesLen {
		t.snap = make([]recurrence.RQASample, recurrence.RQASeriesLen)
	}
	n := t.series.Snapshot(t.snap)
	fillRGBA(m, trendGround, 1)
	colX := func(c float64) float64 { return c * float64(w) / recurrence.RQASeriesLen }
	pane := float64(h) / float64(recurrence.RQATraceCount)
	paneY := func(top, f float64) float64 { return top + 1 + (pane-3)*(1-f) }
	hline := func(y float64, c color.RGBA, a float64) {
		strokePolyline(m, [][2]float64{{0, y}, {float64(w), y}}, 1, c, a)
	}
	for ms := trendTickMs; ms < recurrence.RQASeriesSpanMs; ms += trendTickMs {
		x := colX(recurrence.RQASeriesLen - float64(ms)/recurrence.RQASamplePeriodMs)
		strokePolyline(m, [][2]float64{{x, 0}, {x, float64(h)}}, 1, trendRule, 1)
	}
	for tr := range recurrence.RQATraceCount {
		top := float64(tr) * pane
		if tr == recurrence.RQATraceRR {
			// The band the plot is readable in, behind the trace.
			hi := paneY(top, recurrence.RQATraceY(tr, recurrence.RQAReadableHi))
			lo := paneY(top, recurrence.RQATraceY(tr, recurrence.RQAReadableLo))
			for y := hi; y <= lo; y++ {
				hline(y, trendBand, 0.13)
			}
		} else {
			hline(paneY(top, 0.5), trendRule, 1)
		}
		if tr+1 < recurrence.RQATraceCount {
			hline(top+pane-1, trendEdge, 1)
		}
		if n > 0 {
			// Broken across the slots with no reading: a hole in the record
			// is a hole in the line.
			var run [][2]float64
			flush := func() {
				if len(run) == 1 {
					run = append(run, run[0])
				}
				strokePolyline(m, run, 1, trendTrace[tr], 1)
				run = run[:0]
			}
			for i, s := range t.snap {
				if !s.OK {
					flush()
					continue
				}
				run = append(run, [2]float64{colX(float64(i) + 0.5), paneY(top, recurrence.RQATraceY(tr, s.Value(tr)))})
			}
			flush()
		}
		drawText(m, 4, int(top)+3, tr.String(), trendText, 1)
	}
}

// monitorTrend draws the RQA trend on monitor m, when it is what the monitor
// shows: recurrence running with the TREND switch up. Called with r.mu held.
func (r *Rack) monitorTrend(m *image.RGBA) (source string, ok bool) {
	if r.model() != "recurrence" || r.analyzers.trendOff {
		return "", false
	}
	r.analyzers.ro.trend.draw(m)
	if !r.analyzers.rec.Trajectory() {
		return "audio in", true
	}
	return r.modelName(r.analyzers.lastFlow), true
}
