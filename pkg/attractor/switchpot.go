package attractor

// A potentiometer with a switch on its shaft: past the bottom of its travel
// is a detent, OFF, apart from the range. Turning down from the bottom of the
// range it snaps into OFF, and turning up out of OFF it snaps to the bottom
// of the range; everywhere else it turns smoothly. The scope's SCALE ILLUM is
// one, and its OFF is the scope's power.
//
// The knob's slider carries the gap as a stretch of values below zero, OFF at
// the far end of it, so the pointer's sweep has room for the detent and a
// drag has to cross half of it to snap either way. The range proper is zero
// up.

// switchPot is one such knob, by where its OFF is on the slider (below zero).
type switchPot struct{ off float64 }

// isOff says whether v is in the detent.
func (s switchPot) isOff(v float64) bool { return v <= s.off/2 }

// snap is where a raw slider value comes to rest: the detent, or the range.
func (s switchPot) snap(v float64) float64 {
	switch {
	case s.isOff(v):
		return s.off
	case v < 0:
		return 0
	}
	return v
}

// step is one detent of the wheel or an arrow key, from old to v: stepping
// down out of the range goes to OFF, and up out of OFF to the bottom of the
// range, so a click is never lost in the gap between them.
func (s switchPot) step(old, v float64) float64 {
	switch {
	case v < old && old >= 0 && v < 0:
		return s.off
	case v > old && old < 0:
		return 0
	}
	return s.snap(v)
}
