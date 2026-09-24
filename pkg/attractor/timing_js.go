//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/led"
	"syscall/js"
	"time"
)

// The Timing module's panel. See timing.go for what it measures and why
// those numbers and not others.

// timingPeriodMs is how often the readouts latch.
//
// Not sixty times a second, which is how often the numbers change: a frame
// meter that updates every frame is unreadable, and it would also be the one
// module whose readout cost showed up in its own measurement. Half a second
// is a window long enough to hold a stutter that happens twice a second —
// which is the rate the analyzers latch at, and therefore the rate the thing
// this exists to find happens at.
const timingPeriodMs = 500

// timingPanel is the Timing module's panel: its LEDs and its refresh clock.
type timingPanel struct {
	stats                        frameStats
	budget                       sectionBudget
	nextMs                       float64
	lastMs                       float64 // rAF timestamp of the previous frame
	fpsEl, frameEl, minEl, maxEl js.Value
	lateEl, modelEl, metersEl    js.Value
	scopeEl, restEl              js.Value
}

var tpanel timingPanel

// timingMark is the start of a span. Separate from the accumulators so the
// three spans can nest inside one frame without a stack.
type timingMark time.Time

func timingStart() timingMark { return timingMark(time.Now()) }

// ms is how long the span took, in milliseconds.
func (m timingMark) ms() float32 {
	return float32(time.Since(time.Time(m)).Microseconds()) / 1000
}

// frame records one frame. Called from renderLoop with the rAF
// timestamp, before anything else uses it.
//
// Accumulated whether or not the panel is on screen, unlike every other meter
// here. Three clock reads and some adds are far below the cost of the DOM
// check that would decide to skip them, and a frame meter that only starts
// measuring once you look at it cannot answer "what was it doing before I
// scrolled here", which is the question.
func (t *timingPanel) frame(nowMs float64) {
	if t.lastMs > 0 {
		t.stats.add(float32(nowMs - t.lastMs))
		t.budget.Frames++
	}
	t.lastMs = nowMs
}

// tick latches the readouts on their own clock.
//
// ALONE AMONG THE METERS, THIS ONE DOES NOT STOP WHEN IT IS OFF SCREEN.
// Every other module here checks moduleOnScreen first, and should: their work
// is filtering and FFTs, and doing it for a panel nobody can see is the whole
// point of the check. This module's work is nine guarded LED writes twice a
// second — the readouts skip a write that would not change anything anyway —
// and against that the check would cost more than it saves.
//
// The reason is not only cost. A frame meter that freezes while you look away
// and shows the last numbers it had is worse than one that is simply right:
// scroll back and the panel reads like a live instrument while displaying
// whatever the rack was doing before you left. Blanking instead would be
// honest but useless, since the question is always "what is it doing NOW".
// So it keeps measuring and keeps latching, and is correct the instant it
// comes into view.
func (t *timingPanel) tick(nowMs float64) {
	if nowMs < t.nextMs {
		return
	}
	t.nextMs = nowMs + timingPeriodMs
	t.showTiming()
	t.stats.reset()
	t.budget.reset()
}

// showTiming writes the readouts.
func (t *timingPanel) showTiming() {
	if t.stats.n == 0 {
		for _, p := range []struct {
			k string
			e js.Value
		}{
			{"tm-fps", t.fpsEl}, {"tm-frame", t.frameEl},
			{"tm-min", t.minEl}, {"tm-max", t.maxEl},
			{"tm-late", t.lateEl}, {"tm-model", t.modelEl},
			{"tm-meters", t.metersEl}, {"tm-scope", t.scopeEl},
			{"tm-rest", t.restEl},
		} {
			owed.readouts.Set(p.k, p.e, "  --.-")
		}
		return
	}
	owed.readouts.Set("tm-fps", t.fpsEl, led.Format(float64(t.stats.fps()), 3, 1, false))
	owed.readouts.Set("tm-frame", t.frameEl, led.Format(float64(t.stats.avg()), 3, 1, false))
	owed.readouts.Set("tm-min", t.minEl, led.Format(float64(t.stats.min), 3, 1, false))
	owed.readouts.Set("tm-max", t.maxEl, led.Format(float64(t.stats.max), 3, 1, false))
	owed.readouts.Set("tm-late", t.lateEl, led.Format(float64(t.stats.latePct()), 3, 1, false))

	model, meters, scope := t.budget.perFrame()
	owed.readouts.Set("tm-model", t.modelEl, led.Format(float64(model), 2, 2, false))
	owed.readouts.Set("tm-meters", t.metersEl, led.Format(float64(meters), 2, 2, false))
	owed.readouts.Set("tm-scope", t.scopeEl, led.Format(float64(scope), 2, 2, false))
	// What is left is the browser's: style, layout, raster, compositing, and
	// the wasm boundary. Shown because it is usually the largest share and
	// there is no honesty in three numbers that quietly do not add up to the
	// frame they came out of.
	rest := t.stats.avg() - model - meters - scope
	if rest < 0 {
		rest = 0
	}
	owed.readouts.Set("tm-rest", t.restEl, led.Format(float64(rest), 2, 2, false))
}

// wireTimingModule finds the readouts. Called once from Run.
func (t *timingPanel) wireTimingModule() {
	t.fpsEl = dom.Doc.Call("getElementById", "tm-fps-led")
	t.frameEl = dom.Doc.Call("getElementById", "tm-frame-led")
	t.minEl = dom.Doc.Call("getElementById", "tm-min-led")
	t.maxEl = dom.Doc.Call("getElementById", "tm-max-led")
	t.lateEl = dom.Doc.Call("getElementById", "tm-late-led")
	t.modelEl = dom.Doc.Call("getElementById", "tm-model-led")
	t.metersEl = dom.Doc.Call("getElementById", "tm-meters-led")
	t.scopeEl = dom.Doc.Call("getElementById", "tm-scope-led")
	t.restEl = dom.Doc.Call("getElementById", "tm-rest-led")
}
