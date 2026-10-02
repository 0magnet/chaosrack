package attractor

import (
	"math"
	"strings"
)

// A bank position's knob turns without end (turns_js.go wires it).
//
// A knob with stops can only ever say its range once: a constant whose
// interesting values run past the top of its scale needed a wider scale for
// everyone, which made the values most people want a sliver of the sweep. An
// encoder has no stops, so it does not have to choose. One full turn is the
// parameter's range; turning on past the top goes round again, and the ring
// lights in the next LED color to say which lap it is on (paint).
//
// What a lap means is the parameter's:
//
//   - bounded: no laps — a count, a setting, anything with a hard edge
//   - linear: each lap is another range's worth, above or below (a system's
//     constants, which have no edge but the one the scale was drawn with)
//   - log: each lap is ten times the one before, up from the top of the range
//     (a step, a scale, a size: things that matter in proportion)
//   - wrap: the range is a circle, and turning past the end comes round to
//     the start (a hue, an angle, a phase)

type turnKind int

const (
	turnBounded turnKind = iota
	turnLinear
	turnLog
	turnWrap
)

// turnSpec is a knob's base range and what its laps are.
type turnSpec struct {
	kind   turnKind
	lo, hi float64
}

// ringLEDs is the full ring's LED count: one per detent of a turn
// (pkg/encoder), so each click of the knob moves the ring by one.
const ringLEDs = 24

// maxLaps is how far a knob goes past its range, either way: far enough to
// be a range of its own, short of a value a float32 cannot keep.
const maxLaps = 6

// maxLogLaps is the same for a knob whose laps are tenfold: three is a
// thousand times its range, which for a step is already past where a system
// can be integrated.
const maxLogLaps = 3

// paramTurns is the kind of knob a parameter gets, from what the parameter
// declares: a named setting or a count is bounded, a step or a scale is
// logarithmic, a circle wraps, and a constant is linear.
func paramTurns(id, label string, lo, hi, step float64, named bool) turnKind {
	l := strings.ToLower(label)
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(l, w) || strings.Contains(id, w) {
				return true
			}
		}
		return false
	}
	switch {
	case named || !(hi > lo):
		return turnBounded
	// Before the counts: a hue in whole degrees is still a circle.
	case has("hue", "angle", "phase", "rot") || l == "θ" || l == "φ" ||
		nearly(hi-lo, 360) || nearly(hi-lo, 2*math.Pi):
		return turnWrap
	case step >= 1:
		return turnBounded
	case strings.HasSuffix(id, "-dt") || l == "dt":
		return turnLog
	case lo >= 0 && has("scale", "size", "zoom", "gain", "rate", "speed", "freq", "extent", "radius"):
		return turnLog
	}
	return turnLinear
}

func nearly(a, b float64) bool { return math.Abs(a-b) < 1e-3*math.Max(1, math.Abs(b)) }

// clamp is v kept where this knob can go.
func (s turnSpec) clamp(v float64) float64 {
	w := s.hi - s.lo
	switch s.kind {
	case turnLinear:
		return math.Max(s.lo-maxLaps*w, math.Min(s.hi+maxLaps*w, v))
	case turnLog:
		top := s.hi
		if top <= 0 {
			top = w
		}
		return math.Max(s.lo, math.Min(top*math.Pow(10, maxLogLaps), v))
	case turnWrap:
		if !(w > 0) {
			return s.lo
		}
		return s.lo + math.Mod(math.Mod(v-s.lo, w)+w, w)
	}
	return math.Max(s.lo, math.Min(s.hi, v))
}

// domain is the widest a knob's value can be, for the hidden slider under
// it, which clamps to its own min and max.
func (s turnSpec) domain() (lo, hi float64) { //nolint:unused // called from turns_js.go, which the native lint pass cannot see
	w := s.hi - s.lo
	switch s.kind {
	case turnLinear:
		return s.lo - maxLaps*w, s.hi + maxLaps*w
	case turnLog:
		return s.lo, s.clamp(math.Inf(1))
	}
	return s.lo, s.hi
}

// lap is which turn v is on, and how far round it, 0 to 1. Lap 0 is the
// range itself; a value at the very top of a lap is the end of that lap, not
// the start of the next, so the range's own maximum reads as a full ring.
func (s turnSpec) lap(v float64) (lap int, frac float64) {
	w := s.hi - s.lo
	if !(w > 0) {
		return 0, 0
	}
	switch s.kind {
	case turnLinear:
		u := (v - s.lo) / w
		lap = int(math.Ceil(u)) - 1
		if u <= 0 {
			lap = int(math.Floor(u))
		}
		return lap, u - float64(lap)
	case turnLog:
		if v <= s.hi || s.hi <= 0 {
			return 0, math.Max(0, (v-s.lo)/w)
		}
		lap = int(math.Ceil(math.Log10(v/s.hi) - 1e-9))
		a := s.hi * math.Pow(10, float64(lap-1))
		return lap, (v - a) / (9 * a)
	case turnWrap:
		return 0, (s.clamp(v) - s.lo) / w
	}
	return 0, math.Max(0, math.Min(1, (v-s.lo)/w))
}

// ringPaint is what a full ring shows for a value: the LEDs lit in this
// lap's color, and the ones still lit in the color of the lap before. Empty
// when to < from.
type ringPaint struct {
	from, to, lap    int // lit in this lap's color
	uFrom, uTo, uLap int // lit in the color of the lap it came from
}

// paint is which LEDs of the full ring are lit for v, from the top
// clockwise, and in which lap's color.
//
// Every lap is its own color (lapColor), and a lap paints over the one
// before it rather than starting from a dark ring: going up, the new color
// sweeps clockwise from the top over a ring still lit in the last lap's;
// going down below the range, it sweeps back counter-clockwise from the top
// over the lap above. So nothing jumps at the top of a turn, and how many
// laps round the knob is shows as a color, all the way round.
//
// Lap zero, the knob's own range, lights over a dark ring. A wrapping knob
// lights one LED, where it points, since a circle has no zero to measure
// from and no laps.
func (s turnSpec) paint(v float64) ringPaint {
	lap, f := s.lap(v)
	i := int(math.Round(f * ringLEDs))
	if s.kind == turnWrap {
		i %= ringLEDs
		return ringPaint{from: i, to: i, uTo: -1}
	}
	i = min(max(i, 0), ringLEDs)
	p := ringPaint{uTo: -1}
	if lap >= 0 {
		p.from, p.to, p.lap = 0, i-1, lap
		if lap > 0 {
			p.uFrom, p.uTo, p.uLap = i, ringLEDs-1, lap-1
		}
		return p
	}
	p.from, p.to, p.lap = i, ringLEDs-1, lap
	if lap < -1 {
		p.uFrom, p.uTo, p.uLap = 0, i-1, lap+1
	}
	return p
}

// lapColor is the index of the color a lap lights in, in a list of n:
// lap zero is base, the rack's own LED color, and each lap on is the next
// one in the list, round and round — backward for the laps below the range.
func lapColor(base, lap, n int) int {
	if n < 1 {
		return 0
	}
	return ((base+lap)%n + n) % n
}

// angle is where the knob's pointer is for v, in degrees clockwise from the
// top: round the ring with the lap, rather than across a 270° sweep.
func (s turnSpec) angle(v float64) float64 { //nolint:unused // called from knobs_js.go, which the native lint pass cannot see
	_, f := s.lap(v)
	return 360 * f
}
