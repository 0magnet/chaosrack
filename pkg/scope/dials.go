package scope

import (
	"math"
	"strconv"
)

// The rack-mount oscilloscope: the part of it that is arithmetic.
//
// This is a SECOND instrument, not a second view of the model. It has its own
// tube, its own timebase and its own trigger, it is fed from the live audio
// rather than from whatever the model knob is pointed at, and it keeps
// running when the model is a polyhedron. That independence is the whole
// point of putting a scope in a rack: you wire a signal to it and it tells
// you about the signal, whatever else the rack is doing.
//
// Kept out of the js file so it can be tested, because every number here is
// a claim about what the front panel MEANS. A TIME/DIV knob that does not
// actually put that many seconds in a division is a knob with a lie
// silkscreened next to it.

// steps125 is the sequence a real range switch steps through: 1, 2, 5,
// 10, 20, 50, and so on, in both directions from 1.
//
// Not a smooth knob. A scope's VOLTS/DIV and TIME/DIV are detented switches
// in a 1-2-5 sequence, and that is not an arbitrary styling choice — it is
// what makes a division worth a round number, so a reading is counted off
// the screen rather than computed. A continuous knob would look more capable
// and make the graticule useless.
//
// lo and hi bound the range inclusively; the result is ascending.
func steps125(lo, hi float64) []float64 {
	if !(lo > 0) || !(hi >= lo) {
		return nil
	}
	var out []float64
	// Start at the decade below lo so the first in-range step is not missed
	// when lo falls between two of them.
	dec := math.Pow(10, math.Floor(math.Log10(lo))-1)
	for dec <= hi*10 {
		for _, m := range []float64{1, 2, 5} {
			v := snap125(dec * m)
			if v >= lo*(1-1e-9) && v <= hi*(1+1e-9) {
				out = append(out, v)
			}
		}
		dec *= 10
	}
	return out
}

// snap125 rounds away the float error that repeated decade multiplication
// leaves, so a step reads as 0.05 and not 0.05000000000000001 — these
// numbers are printed on a front panel.
func snap125(v float64) float64 {
	e := math.Floor(math.Log10(v))
	p := math.Pow(10, e)
	return math.Round(v/p*1000) / 1000 * p
}

// The ranges the two knobs cover. Both are what a modest bench scope offers,
// and both are bounded by what the instrument can actually show: the fastest
// sweep is limited by the sample rate, the slowest by how long anyone will
// watch one screen fill.
const (
	secPerDivMin = 10e-6 // 10 µs/div
	secPerDivMax = 0.5   // 500 ms/div
	voltsDivMin  = 0.002 // full scale is ±1, so 2 mV/div is 500 screens
	voltsDivMax  = 1.0   // 1.0/div puts full scale in one division
)

// Timebases and VoltsDivs are the detents on the two range
// switches, built once from the sequence rather than typed out.
var (
	Timebases = steps125(secPerDivMin, secPerDivMax)
	VoltsDivs = steps125(voltsDivMin, voltsDivMax)
)

// SweepSamples is how many samples the beam crosses the whole screen in
// at this timebase: ten divisions of secPerDiv at sr samples a second.
//
// At least two, because a screen with one sample on it is a dot and the line
// drawing has nothing to join. That floor is reached by asking for a sweep
// faster than the converter can feed, which is a real thing to do — it is
// what the fastest detent means on a scope whose sample rate is low — and it
// should show a coarse trace rather than nothing.
func SweepSamples(secPerDiv float64, sr int) int {
	if sr <= 0 || !(secPerDiv > 0) {
		return 2
	}
	n := int(secPerDiv * float64(DivX) * float64(sr))
	if n < 2 {
		return 2
	}
	return n
}

// YDiv converts one sample to its height on the screen, in divisions
// above center, at this VOLTS/DIV and vertical POSITION.
//
// NOT clamped. A scope does not fold a signal back onto the screen when it
// overruns — the trace leaves the top and comes back, which is how you know
// you are overdriving the input and need a coarser range. Clipping here
// would hide exactly the fault the knob exists to find. The caller clips to
// the tube, which is what the glass does.
func YDiv(sample float32, voltsPerDiv, posDiv float64) float64 {
	if !(voltsPerDiv > 0) {
		return posDiv
	}
	return float64(sample)/voltsPerDiv + posDiv
}

// TriggerIndex finds the sample where the trace should start: the first
// crossing of level in the chosen direction, searching forward from the
// start of the buffer.
//
// Returns -1 for no crossing found, which the caller must treat as AUTO
// treats it — sweep anyway, from wherever, so a signal that never crosses
// (silence, or a level set off the signal) still shows a baseline rather
// than a blank tube. A scope with no free-running sweep looks broken every
// time the trigger is misadjusted, and that is the most common thing to have
// got wrong.
//
// The crossing is looked for BETWEEN samples, on the sign change, rather
// than on "is this sample past the level": a sample exactly at the level is
// not an edge, and a run of them is not a run of edges.
func TriggerIndex(s []float32, level float32, rising bool, limit int) int {
	if limit > len(s) {
		limit = len(s)
	}
	for i := 1; i < limit; i++ {
		a, b := s[i-1], s[i]
		if rising && a < level && b >= level {
			return i
		}
		if !rising && a > level && b <= level {
			return i
		}
	}
	return -1
}

// NearestStep returns the index of the detent closest to v, for
// restoring a knob from a saved value onto a switch that has moved.
func NearestStep(steps []float64, v float64) int {
	if len(steps) == 0 {
		return 0
	}
	best, bestD := 0, math.Inf(1)
	for i, s := range steps {
		// Compared in the log domain, because these are a ratio scale: 2 is
		// as far from 1 as 5 is from 10, and a linear distance would make
		// every knob position near the top of the range look adjacent.
		d := math.Abs(math.Log(s) - math.Log(v))
		if d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// FormatTime renders a timebase the way it is silkscreened: a whole
// number and a unit, never an exponent.
func FormatTime(sec float64) string {
	switch {
	case sec >= 1:
		return trimNum(sec) + " s"
	case sec >= 1e-3:
		return trimNum(sec*1e3) + " ms"
	default:
		return trimNum(sec*1e6) + " µs"
	}
}

// FormatVolts renders a VOLTS/DIV the same way. The signal is a
// normalized sample and not a voltage, so the unit is full scale — "FS" —
// which is the honest label: this is a level, and calling it volts when
// nothing was calibrated against a volt would be the lie the comment at the
// top of this file is about.
func FormatVolts(v float64) string {
	if v >= 1 {
		return trimNum(v) + " FS"
	}
	return trimNum(v*1e3) + " mFS"
}

// trimNum prints a number with no trailing zeros and no exponent — a
// front panel says 20 ms, not 2.0000e-02.
func trimNum(v float64) string {
	return strconv.FormatFloat(math.Round(v*1000)/1000, 'f', -1, 64)
}
