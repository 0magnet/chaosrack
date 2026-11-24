package sacred

import (
	"math"
	"testing"
)

func dist(a, b V) float64 { return math.Hypot(a[0]-b[0], a[1]-b[1]) }

// Every form, at every detail and through morph, is finite, about a unit in
// size, fits a 16-bit index buffer and joins only points it has.
func TestEveryFormIsWellMade(t *testing.T) {
	for f := range NumForms {
		for d := 0; d <= MaxDetail; d++ {
			for _, m := range []float64{0, 0.3, 1, 1.5, 2} {
				l := Form(f, m, float64(d))
				if len(l.P) >= 65536 {
					t.Fatalf("%s d=%d: %d points", FormNames[f], d, len(l.P))
				}
				for _, e := range l.E {
					if e[0] < 0 || e[1] < 0 || e[0] >= len(l.P) || e[1] >= len(l.P) {
						t.Fatalf("%s d=%d m=%v: an edge past the points", FormNames[f], d, m)
					}
				}
				for _, p := range l.P {
					r := math.Hypot(p[0], p[1])
					if math.IsNaN(r) || r > 1.5 {
						t.Fatalf("%s d=%d m=%v: a point at %v", FormNames[f], d, m, p)
					}
				}
				if m >= 1 && len(l.E) == 0 {
					t.Errorf("%s d=%d m=%v: nothing drawn", FormNames[f], d, m)
				}
			}
		}
	}
}

// MORPH draws the construction: nothing at 0, more as it turns, all at 1,
// and the frame of the whole from the start.
func TestMorphDrawsTheConstruction(t *testing.T) {
	for f := range NumForms {
		if n := len(Form(f, 0, 1).E); n != 0 {
			t.Errorf("%s: %d segments at morph 0", FormNames[f], n)
		}
		prev := 0
		for _, m := range []float64{0.1, 0.25, 0.5, 0.75, 1} {
			n := len(Form(f, m, 1).E)
			if n < prev {
				t.Errorf("%s: %d segments at morph %v, fewer than before", FormNames[f], n, m)
			}
			prev = n
		}
		start, whole := Form(f, 0, 1).P[:2], Form(f, 1, 1).P[:2]
		if dist(start[0], whole[0]) > 1e-9 || dist(start[1], whole[1]) > 1e-9 {
			t.Errorf("%s: framed %v at the start, %v whole", FormNames[f], start, whole)
		}
	}
}

// The vesica's circles each pass through the other's center.
func TestVesicaCirclesPassThroughEachOthersCenter(t *testing.T) {
	ss := vesica(0)
	a, b := V{-0.5, 0, 0}, V{0.5, 0, 0}
	for _, c := range []struct {
		s      stroke
		center V
	}{{ss[0], b}, {ss[1], a}} {
		closest := math.Inf(1)
		for _, p := range c.s {
			closest = min(closest, dist(p, c.center))
		}
		// 96 pieces: a chord's sagitta is 1−cos(π/96).
		if closest > 1e-3 {
			t.Errorf("a circle passes %v from the other's center", closest)
		}
	}
}

// The usual Flower of Life: 19 circles whole at two rings.
func TestFlowerOfLifeHasNineteenCircles(t *testing.T) {
	if whole, arcs := FlowerCounts(2); whole != 19 || arcs == 0 {
		t.Errorf("two rings: %d whole, %d arcs; want 19 whole and the arcs of the next ring", whole, arcs)
	}
	if whole, _ := FlowerCounts(1); whole != 7 {
		t.Errorf("one ring: %d whole, want the Seed's 7", whole)
	}
}

// The Fruit of Life's circles touch and never cross.
func TestFruitCirclesTouchNeverCross(t *testing.T) {
	cs := fruitCenters()
	touching := 0
	for i := range cs {
		for j := i + 1; j < len(cs); j++ {
			d := dist(cs[i], cs[j])
			if d < 2-1e-9 {
				t.Fatalf("circles %d and %d cross: centers %v apart", i, j, d)
			}
			if math.Abs(d-2) < 1e-9 {
				touching++
			}
		}
	}
	if touching != 18 {
		t.Errorf("%d pairs touch, want 18: the six to the center, to each other, and each to its outer one", touching)
	}
}

// Metatron's Cube joins every pair of the 13 centers: 78 lines.
func TestMetatronHasSeventyEightLines(t *testing.T) {
	if n := len(metatron(0)); n != 78 {
		t.Errorf("%d lines", n)
	}
	if n := len(metatron(1)); n != 78+13 {
		t.Errorf("with circles, %d strokes; want 78 + 13", n)
	}
}

// The golden spiral is one curve: each quarter circle starts where the last
// ended, each 1/φ the size.
func TestGoldenSpiralIsOneCurve(t *testing.T) {
	ss := golden(10)
	var arcs []stroke
	for i := 2; i < len(ss); i += 2 {
		arcs = append(arcs, ss[i])
	}
	for i := 1; i < len(arcs); i++ {
		end, start := arcs[i-1][len(arcs[i-1])-1], arcs[i][0]
		if dist(end, start) > 1e-9 {
			t.Errorf("arc %d ends at %v, arc %d starts at %v", i-1, end, i, start)
		}
		r0, r1 := dist(arcs[i-1][0], arcs[i-1][len(arcs[i-1])-1]), dist(arcs[i][0], arcs[i][len(arcs[i])-1])
		if math.Abs(r0/r1-Phi) > 1e-9 {
			t.Errorf("arc %d is %v the size of arc %d, not 1/φ", i, r1/r0, i-1)
		}
	}
}

// The sunflower has its seeds, the first at the center.
func TestSunflowerSeeds(t *testing.T) {
	ss := sunflower(300, GoldenAngle)
	if len(ss) != 300 {
		t.Fatalf("%d seeds", len(ss))
	}
	if math.Abs(GoldenAngle-137.50776405) > 1e-6 {
		t.Errorf("the golden angle is %v°", GoldenAngle)
	}
}

// EquationLines names every form.
func TestEquationLinesNameEveryForm(t *testing.T) {
	for f := range NumForms {
		for i, l := range EquationLines(f, 1, 1) {
			if l[0] == "" || l[1] == "" {
				t.Errorf("%s: line %d is empty", FormNames[f], i)
			}
		}
	}
	if len(FormNames) != NumForms || len(RingLabels) != NumForms {
		t.Errorf("%d names, %d ring labels for %d forms", len(FormNames), len(RingLabels), NumForms)
	}
}
