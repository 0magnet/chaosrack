// Package encoder is a rotary encoder: rotation in, detents out.
//
// A potentiometer's position IS its value, so a knob that stands for a
// different parameter every time the MODEL knob turns cannot be one — the
// value it would read is the last model's. An incremental encoder has no
// position, only motion: a click of the detent is one step, clockwise or
// back, and what the step means is up to what it is connected to. That is
// the part a programmable bank is built from (a Bourns PEC11R-4215F-S0024:
// 24 detents a turn, a push switch), and this is its behavior, apart from
// any screen.
//
// One thing a bare encoder does badly is cover a wide range: ninety steps is
// nearly four turns. Every instrument built on them accelerates, turning a
// quick spin into bigger steps, and so does this.
package encoder

import "math"

// Encoder accumulates rotation and emits detents. The zero value is a
// 24-detent encoder at rest.
type Encoder struct {
	PerRev int // detents a full turn; 0 means 24

	pos    float64   // position, in detents from where the grab began at rest
	recent []float64 // times of the last detents, ms, for the rate
}

// accelWindowMs is how far back the turning rate is measured over.
const accelWindowMs = 150.0

// Turn feeds rad of rotation (clockwise positive) at time nowMs, and returns
// the steps it makes: whole detents, multiplied by the acceleration of how
// fast they are coming. Negative is counterclockwise.
func (e *Encoder) Turn(rad, nowMs float64) int {
	per := e.PerRev
	if per <= 0 {
		per = 24
	}
	// A click is a crossing of the halfway point between two rest positions,
	// whichever way it is crossed, so turning back over one clicks back.
	prev := e.pos
	e.pos += rad / (2 * math.Pi) * float64(per)
	det := int(math.Floor(e.pos+0.5) - math.Floor(prev+0.5))
	if det == 0 {
		return 0
	}
	n := det
	if n < 0 {
		n = -n
	}
	for range n {
		e.recent = append(e.recent, nowMs)
	}
	// Only the window's detents count toward the rate.
	cut := 0
	for cut < len(e.recent) && e.recent[cut] < nowMs-accelWindowMs {
		cut++
	}
	e.recent = e.recent[cut:]
	rate := float64(len(e.recent)) / (accelWindowMs / 1000) // detents a second
	return det * Accel(rate)
}

// Reset returns to a rest position and forgets the rate: a new grab starts
// still.
func (e *Encoder) Reset() {
	e.pos = 0
	e.recent = e.recent[:0]
}

// Accel is the step multiplier at a turning rate in detents a second. Slow
// turning is one step a detent, so a value can be set exactly; past about a
// third of a turn a second it grows, to ten at the speed of a flick.
func Accel(rate float64) int {
	const slow, span, top = 8.0, 4.0, 10
	if rate <= slow {
		return 1
	}
	return min(top, 1+int((rate-slow)/span))
}
