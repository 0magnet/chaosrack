package attractor

import "math"

// An encoder's LED ring (ledring_js.go lights it): the dial's ticks are the
// LEDs, since a knob that is reprogrammed cannot say its value with a
// pointer — the pointer is wherever the last model left it. The ring says it
// instead, the way every instrument built on encoders does.

// ringLit is which of n LEDs are lit for v in [lo, hi], first to last,
// inclusive. A range that runs from zero lights from the first LED to the
// value's; a signed one, from the LED at zero out to the value's, either way,
// so a twist of -2 and one of +2 read as the same size.
func ringLit(v, lo, hi float64, n int) (from, to int) {
	if n <= 0 || !(hi > lo) {
		return 0, -1
	}
	at := func(x float64) int {
		t := (math.Max(lo, math.Min(hi, x)) - lo) / (hi - lo)
		return int(math.Round(t * float64(n-1)))
	}
	i := at(v)
	if lo < 0 && hi > 0 {
		z := at(0)
		return min(z, i), max(z, i)
	}
	return 0, i
}

// ringDot is the one LED lit for position i of count on a ring of n: a
// selector's detents spread over the ring's travel, the way its labels would
// be.
func ringDot(i, count, n int) int { //nolint:unparam // the js build passes each selector's option count; the host sees only the test's
	if count <= 1 || n <= 0 {
		return 0
	}
	i = max(0, min(count-1, i))
	return int(math.Round(float64(i) / float64(count-1) * float64(n-1)))
}
