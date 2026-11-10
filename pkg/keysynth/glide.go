package keysynth

import "math"

// GlideSpan clamps the time between two pointer events, in seconds, to what
// a run of passing notes is spread across: long enough to hear as a run,
// short enough that a pointer which paused does not play one in slow motion.
func GlideSpan(dt float64) float64 { return min(max(dt, 0.02), 0.15) }

// GlidePath is the keys a pointer crossed going in a straight line from
// (x0, y0) to (x, y), in the order it crossed them, the one it lands on
// last. at is the key under a point, and held the key already sounding,
// which is left out, as is a key met twice in a row.
//
// The line is walked three pixels at a time because a pointer event is not
// sent per key: a fast drag crosses several keys between two events, and a
// run that skipped them would not be the run the hand played. The page and
// the native rack each had this loop; they find the key under a point
// differently, and that is all at is.
func GlidePath(x0, y0, x, y float64, held int, at func(x, y float64) (int, bool)) []int {
	var path []int
	steps := int(math.Hypot(x-x0, y-y0)/3) + 1
	for i := 1; i <= steps; i++ {
		f := float64(i) / float64(steps)
		m, ok := at(x0+(x-x0)*f, y0+(y-y0)*f)
		if !ok || m == held || (len(path) > 0 && path[len(path)-1] == m) {
			continue
		}
		path = append(path, m)
	}
	return path
}
