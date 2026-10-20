package attractor

import (
	"strconv"
	"strings"
)

// An LED ring as a front end without the page lights it. The page marks each
// LED ring's dial with how it lights (data-ring, RingSpec.String), the
// catalog keeps the mark on every LED of it, and a native rack lights the
// LEDs from a control's value with what the page lights them with: ringLit,
// ringDot and turnSpec.paint, in lapColor's colors.

// RingSpec is how a dial's LED ring lights: one LED at a selector's setting,
// or a run from the start of a range or from zero, or an endless knob's laps.
type RingSpec struct {
	sel  bool
	turn turnSpec // turnBounded for a run within the range
}

// String is the spec as the page marks it on the dial: "sel", or the turn's
// kind and range — "b 0 1", "l -2 2", "g 0.0001 0.02", "w 0 360" — and a
// bounded linear knob's ends after its range.
func (r RingSpec) String() string {
	if r.sel {
		return "sel"
	}
	f := func(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
	s := ringKinds[r.turn.kind] + " " + f(r.turn.lo) + " " + f(r.turn.hi)
	if r.turn.bounded {
		s += " " + f(r.turn.floor) + " " + f(r.turn.ceil)
	}
	return s
}

// ringKinds are the turn kinds as a mark spells them.
var ringKinds = map[turnKind]string{turnBounded: "b", turnLinear: "l", turnLog: "g", turnWrap: "w"}

// ParseRing reads a mark String wrote.
func ParseRing(s string) (RingSpec, bool) {
	f := strings.Fields(s)
	if len(f) == 1 && f[0] == "sel" {
		return RingSpec{sel: true}, true
	}
	if len(f) != 3 && len(f) != 5 {
		return RingSpec{}, false
	}
	var t turnSpec
	found := false
	for k, name := range ringKinds {
		if name == f[0] {
			t.kind, found = k, true
		}
	}
	nums := make([]float64, len(f)-1)
	for i := range nums {
		v, err := strconv.ParseFloat(f[i+1], 64)
		if err != nil {
			return RingSpec{}, false
		}
		nums[i] = v
	}
	if !found {
		return RingSpec{}, false
	}
	t.lo, t.hi = nums[0], nums[1]
	if len(nums) == 4 {
		t.bounded, t.floor, t.ceil = true, nums[2], nums[3]
	}
	return RingSpec{turn: t}, true
}

// Lights is each of a ring's n LEDs for value v: -1 dark, or the index in
// LEDColors of the color it lights in, base being the one the Style knob
// chose. For a selector v is the setting's index among count options.
func (r RingSpec) Lights(v float64, count, n, base int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = -1
	}
	light := func(from, to, color int) {
		for i := max(from, 0); i <= to && i < n; i++ {
			out[i] = color
		}
	}
	switch {
	case r.sel:
		d := ringDot(int(v), count, n)
		light(d, d, base)
	case r.turn.kind == turnBounded:
		from, to := ringLit(v, r.turn.lo, r.turn.hi, n)
		light(from, to, base)
	default:
		p := r.turn.paint(v)
		light(p.uFrom, p.uTo, lapColor(base, p.uLap, len(LEDColors)))
		light(p.from, p.to, lapColor(base, p.lap, len(LEDColors)))
	}
	return out
}
