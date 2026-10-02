package scope

import "math"

// FocusBest is where on the FOCUS knob's travel (0 to 1) the spot is
// sharpest. A tube's focus is the voltage on an electrostatic lens, and the
// beam converges at one setting of it: turned past that point the crossover
// moves in front of the screen, short of it behind, and the spot spreads
// either way. So the best focus is part-way round the knob, not at an end.
const FocusBest = 0.5

// Defocus is how far out of focus the beam is at knob position focus: 0 at
// FocusBest, rising to 1 at whichever end of the travel is further from it,
// the same on either side.
func Defocus(focus float64) float64 {
	span := math.Max(FocusBest, 1-FocusBest)
	return math.Min(1, math.Abs(focus-FocusBest)/span)
}
