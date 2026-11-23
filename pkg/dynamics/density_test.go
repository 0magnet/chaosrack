package dynamics

import "testing"

func TestDensityCountsAndScales(t *testing.T) {
	var d Density
	d.Reset([2]float64{0, 0}, [2]float64{2, 1})
	if d.W != DensityMaxSide || d.H != DensityMaxSide/2 {
		t.Fatalf("grid %dx%d for a 2:1 box", d.W, d.H)
	}
	for range 9 {
		d.Add(0.5, 0.5)
	}
	d.Add(1.5, 0.25)
	d.Add(5, 5) // outside: counted as a hit, not in a cell
	if d.Max != 9 || d.Hits != 11 {
		t.Fatalf("max %d hits %d", d.Max, d.Hits)
	}
	busiest := int(0.5*float64(d.H))*d.W + int(0.25*float64(d.W))
	if d.Level(busiest) != 1 {
		t.Errorf("the busiest cell reads %v, want 1", d.Level(busiest))
	}
	if l := d.Level(0); l != 0 {
		t.Errorf("an empty cell reads %v", l)
	}
	once := int(0.25*float64(d.H))*d.W + int(0.75*float64(d.W))
	if l := d.Level(once); !(l > 0 && l < 1) {
		t.Errorf("a cell hit once reads %v, want between 0 and 1", l)
	}
	lv := make([]byte, d.W*d.H)
	d.LevelBytes(lv)
	if lv[busiest] != 255 || lv[0] != 0 || lv[once] == 0 || lv[once] == 255 {
		t.Errorf("level bytes: busiest %d, empty %d, once %d", lv[busiest], lv[0], lv[once])
	}
	d.Reset([2]float64{0, 0}, [2]float64{1, 1})
	if d.Max != 0 || d.Hits != 0 || d.Count[busiest%len(d.Count)] != 0 {
		t.Error("Reset kept counts")
	}
}

// The box holds the figure: every iterate after the transient lands in it.
func TestDensityBoxHoldsTheMap(t *testing.T) {
	for _, k := range []string{"clifford", "dejong", "hopalong", "henon", "standardmap"} {
		m := mustMap(t, k)
		lo, hi := DensityBox(m, 50000)
		var d Density
		d.Reset(lo, hi)
		p := m.Start(0, m.Count())
		for i := range 20000 {
			p[0], p[1], p[2] = m.Step(p[0], p[1], p[2])
			if i >= 200 {
				d.Add(p[0], p[1])
			}
		}
		var in uint64
		for _, c := range d.Count {
			in += uint64(c)
		}
		if share := float64(in) / float64(d.Hits); share < 0.999 {
			t.Errorf("%s: only %.4f of the iterates land in the box", k, share)
		}
	}
}
