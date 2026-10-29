package attractor

import "math"

// featSegs is how many segments a Features ladder has. The stylesheet's
// segment pitch (.lad-bar::before) is its share of the ladder, 5%, and has to
// agree, and so does the canvas over it that draws the lit ones (featladder_js.go).
const featSegs = 20

// featLit is how many segments a reading of v (0..1) lights: the nearest,
// none for silence and every one at full scale.
func featLit(v float32) int {
	if !(v > 0) {
		return 0
	}
	return min(featSegs, int(math.Round(float64(v)*featSegs)))
}

// ladderTint is which color segment k (from the bottom) is, as the
// stylesheet's gradient has them: green to 70%, amber to 90%, red above.
func ladderTint(k int) int {
	switch {
	case k < featSegs*70/100:
		return 0
	case k < featSegs*90/100:
		return 1
	}
	return 2
}
