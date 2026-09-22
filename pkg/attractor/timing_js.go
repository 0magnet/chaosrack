//go:build js && wasm

package attractor

import (
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

var (
	timingStats  frameStats
	timingBudget sectionBudget
	timingNextMs float64
	timingLastMs float64 // rAF timestamp of the previous frame

	timingFpsEl, timingFrameEl, timingMinEl, timingMaxEl js.Value
	timingLateEl, timingModelEl, timingMetersEl          js.Value
	timingScopeEl, timingRestEl                          js.Value
)

// timingMark is the start of a span. Separate from the accumulators so the
// three spans can nest inside one frame without a stack.
type timingMark time.Time

func timingStart() timingMark { return timingMark(time.Now()) }

// ms is how long the span took, in milliseconds.
func (m timingMark) ms() float32 {
	return float32(time.Since(time.Time(m)).Microseconds()) / 1000
}

// timingFrame records one frame. Called from renderLoop with the rAF
// timestamp, before anything else uses it.
//
// Accumulated whether or not the panel is on screen, unlike every other meter
// here. Three clock reads and some adds are far below the cost of the DOM
// check that would decide to skip them, and a frame meter that only starts
// measuring once you look at it cannot answer "what was it doing before I
// scrolled here", which is the question.
func timingFrame(nowMs float64) {
	if timingLastMs > 0 {
		timingStats.add(float32(nowMs - timingLastMs))
		timingBudget.Frames++
	}
	timingLastMs = nowMs
}

// timingTick latches the readouts on their own clock.
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
func timingTick(nowMs float64) {
	if nowMs < timingNextMs {
		return
	}
	timingNextMs = nowMs + timingPeriodMs
	showTiming()
	timingStats.reset()
	timingBudget.reset()
}

// showTiming writes the readouts.
func showTiming() {
	if timingStats.n == 0 {
		for _, p := range []struct {
			k string
			e js.Value
		}{
			{"tm-fps", timingFpsEl}, {"tm-frame", timingFrameEl},
			{"tm-min", timingMinEl}, {"tm-max", timingMaxEl},
			{"tm-late", timingLateEl}, {"tm-model", timingModelEl},
			{"tm-meters", timingMetersEl}, {"tm-scope", timingScopeEl},
			{"tm-rest", timingRestEl},
		} {
			setLEDText(p.k, p.e, "  --.-")
		}
		return
	}
	setLEDText("tm-fps", timingFpsEl, formatLED(float64(timingStats.fps()), 3, 1, false))
	setLEDText("tm-frame", timingFrameEl, formatLED(float64(timingStats.avg()), 3, 1, false))
	setLEDText("tm-min", timingMinEl, formatLED(float64(timingStats.min), 3, 1, false))
	setLEDText("tm-max", timingMaxEl, formatLED(float64(timingStats.max), 3, 1, false))
	setLEDText("tm-late", timingLateEl, formatLED(float64(timingStats.latePct()), 3, 1, false))

	model, meters, scope := timingBudget.perFrame()
	setLEDText("tm-model", timingModelEl, formatLED(float64(model), 2, 2, false))
	setLEDText("tm-meters", timingMetersEl, formatLED(float64(meters), 2, 2, false))
	setLEDText("tm-scope", timingScopeEl, formatLED(float64(scope), 2, 2, false))
	// What is left is the browser's: style, layout, raster, compositing, and
	// the wasm boundary. Shown because it is usually the largest share and
	// there is no honesty in three numbers that quietly do not add up to the
	// frame they came out of.
	rest := timingStats.avg() - model - meters - scope
	if rest < 0 {
		rest = 0
	}
	setLEDText("tm-rest", timingRestEl, formatLED(float64(rest), 2, 2, false))
}

// wireTimingModule finds the readouts. Called once from Run.
func wireTimingModule() {
	timingFpsEl = doc.Call("getElementById", "tm-fps-led")
	timingFrameEl = doc.Call("getElementById", "tm-frame-led")
	timingMinEl = doc.Call("getElementById", "tm-min-led")
	timingMaxEl = doc.Call("getElementById", "tm-max-led")
	timingLateEl = doc.Call("getElementById", "tm-late-led")
	timingModelEl = doc.Call("getElementById", "tm-model-led")
	timingMetersEl = doc.Call("getElementById", "tm-meters-led")
	timingScopeEl = doc.Call("getElementById", "tm-scope-led")
	timingRestEl = doc.Call("getElementById", "tm-rest-led")
}
