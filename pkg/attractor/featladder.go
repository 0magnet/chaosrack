package attractor

import "math"

// featSegs is how many segments a Features ladder has. The stylesheet's
// segment pitch (.lad-bar) is its share of the ladder, 5%, and has to agree.
const featSegs = 20

// featLit is how many segments a reading of v (0..1) lights: the nearest,
// none for silence and every one at full scale.
func featLit(v float32) int {
	if !(v > 0) {
		return 0
	}
	return min(featSegs, int(math.Round(float64(v)*featSegs)))
}
