package attractor

import "testing"

// A point figure is drawn whole however many points it has. Its indices were
// uint16, so it stopped at 65536, and a density view has more cells than
// that: the rest of the figure was simply not drawn.
func TestPointFiguresDrawPast65536(t *testing.T) {
	f := Figure{Kind: FigurePoints}
	// Two clusters: the first 70000 points on the left, the last on the right.
	for i := range 70001 {
		x := -1.0
		if i == 70000 {
			x = 1
		}
		f.Points = append(f.Points, [3]float64{x, 0, 0})
	}
	img := DrawFigure(f, 0, len(f.Points), DrawOptions{Width: 100, Height: 100})
	bg := img.RGBAAt(50, 99)
	right := false
	for x := 50; x < 100 && !right; x++ {
		for y := range 100 {
			if img.RGBAAt(x, y) != bg {
				right = true
				break
			}
		}
	}
	if !right {
		t.Error("point 70000, on the right, was not drawn")
	}
}
