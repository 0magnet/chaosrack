package sacred

import (
	"math"
	"testing"
)

// Each triangle's apex lies on another triangle's base line, except the two
// largest, whose apexes touch the circle.
func TestSriYantraApexesOnBases(t *testing.T) {
	bases := map[float64]bool{}
	for _, tr := range sriTriangles {
		bases[tr.base] = true
	}
	onCircle := 0
	for i, tr := range sriTriangles {
		switch {
		case bases[tr.apex]:
		case math.Abs(math.Abs(tr.apex-150)-100) < 1e-9:
			onCircle++
		default:
			t.Errorf("triangle %d's apex at %v is on no base", i, tr.apex)
		}
	}
	if onCircle != 2 {
		t.Errorf("%d apexes on the circle, want the two largest", onCircle)
	}
}

// The two largest triangles' bases end on the circle.
func TestSriYantraBasesOnTheCircle(t *testing.T) {
	for _, tr := range sriTriangles[:2] {
		if r := math.Hypot(tr.left-150, tr.base-150); math.Abs(r-100) > 1e-9 {
			t.Errorf("a base ends %v from the center, not on the circle", r)
		}
	}
}

// The sides meet three at a time: the marmas. Every point where two sides
// cross is counted, and the crossings that a third side also passes within
// tol of are the triple points.
func TestSriYantraSidesMeetThreeAtATime(t *testing.T) {
	type seg struct{ a, b V }
	var segs []seg
	for _, tr := range sriSides() {
		segs = append(segs, seg{tr[0], tr[1]}, seg{tr[1], tr[2]}, seg{tr[2], tr[0]})
	}
	cross := func(p, q seg) (V, bool) {
		d1 := V{p.b[0] - p.a[0], p.b[1] - p.a[1], 0}
		d2 := V{q.b[0] - q.a[0], q.b[1] - q.a[1], 0}
		den := d1[0]*d2[1] - d1[1]*d2[0]
		if math.Abs(den) < 1e-12 {
			return V{}, false
		}
		s := ((q.a[0]-p.a[0])*d2[1] - (q.a[1]-p.a[1])*d2[0]) / den
		u := ((q.a[0]-p.a[0])*d1[1] - (q.a[1]-p.a[1])*d1[0]) / den
		if s < -1e-9 || s > 1+1e-9 || u < -1e-9 || u > 1+1e-9 {
			return V{}, false
		}
		return V{p.a[0] + s*d1[0], p.a[1] + s*d1[1], 0}, true
	}
	distTo := func(p V, q seg) float64 {
		dx, dy := q.b[0]-q.a[0], q.b[1]-q.a[1]
		s := max(0, min(1, ((p[0]-q.a[0])*dx+(p[1]-q.a[1])*dy)/(dx*dx+dy*dy)))
		return math.Hypot(p[0]-q.a[0]-s*dx, p[1]-q.a[1]-s*dy)
	}
	const tol = 1e-6 // the unit circle's millionth
	var triples []V
	for i := range segs {
		for j := i + 1; j < len(segs); j++ {
			if i/3 == j/3 {
				continue // a triangle's own corners
			}
			p, ok := cross(segs[i], segs[j])
			if !ok {
				continue
			}
			for k := range segs {
				if k/3 == i/3 || k/3 == j/3 {
					continue
				}
				if distTo(p, segs[k]) < tol {
					dup := false
					for _, q := range triples {
						if dist(p, q) < 1e-4 {
							dup = true
						}
					}
					if !dup {
						triples = append(triples, p)
					}
					break
				}
			}
		}
	}
	t.Logf("%d triple points to within %g of the circle's radius", len(triples), tol)
	// Sixteen, as these coordinates were solved: the literature's eighteen
	// marmas also count where an apex meets another's base, left out here.
	if len(triples) < 16 {
		t.Errorf("only %d triple points, want 16: the triangles do not interlock", len(triples))
	}
}
