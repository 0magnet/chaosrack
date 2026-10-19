package attractor

import "testing"

// Every position OUT sets reads back as itself, and any other pinning is MIX.
func TestGenOutReadsWhatItSets(t *testing.T) {
	for pos := range GenOutCount {
		l, r, ok := GenOutPins(pos)
		if pos == GenOutMix {
			if ok {
				t.Error("MIX sets pins")
			}
			continue
		}
		if got := GenOutOf(l, r, l, r); got != pos {
			t.Errorf("%s reads back as %s", GenOutNames[pos], GenOutNames[got])
		}
	}
	for _, p := range [][4]float32{
		{1, 0, 0, 0},     // the speakers only
		{0, 0, 1, 1},     // the rack only
		{0.5, 0, 0.5, 0}, // a gain
		{-1, -1, -1, -1}, // inverted
	} {
		if got := GenOutOf(p[0], p[1], p[2], p[3]); got != GenOutMix {
			t.Errorf("%v reads %s, want MIX", p, GenOutNames[got])
		}
	}
}
