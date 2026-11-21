package chester

import (
	"math"
	"testing"
)

// The lower hexagon is regular, level, has its corners on the side
// triangles' planes, its other three edges inside the solid, and is a little
// smaller than the upper one.
func TestLowerHexagon(t *testing.T) {
	p := solidModel(Chestahedron())
	ps := facePlanes(p)
	y := LowerHexagonY()
	h := LowerHexagon(p, y)
	for i, v := range h {
		if math.Abs(v[1]-y) > 1e-9 {
			t.Fatalf("corner %d at height %v, want %v", i, v[1], y)
		}
		on := false
		for _, q := range ps[3:6] {
			if math.Abs(q.n.dot(v)-q.d) < 1e-9 {
				on = true
			}
		}
		if !on {
			t.Errorf("corner %d is on no side triangle's plane", i)
		}
		if s, s0 := v.sub(h[(i+1)%6]).len(), h[0].sub(h[1]).len(); math.Abs(s-s0) > 1e-9 {
			t.Errorf("side %d is %v, side 0 %v: not regular", i, s, s0)
		}
	}
	inside := 0
	for i := range h {
		mid := lerp(h[i], h[(i+1)%6], 0.5)
		in := true
		for _, q := range ps {
			if q.n.dot(mid) > q.d-1e-6 {
				in = false
			}
		}
		if in {
			inside++
		}
	}
	if inside != 3 {
		t.Errorf("%d edges run inside the solid, want 3", inside)
	}
	if r := LowerHexagonRatio(); r < 0.95 || r >= 1 {
		t.Errorf("lower over upper = %v: want slightly smaller", r)
	}
	if b := Band(p, HexagonAt()); y >= b[0][1] {
		t.Errorf("lower hexagon at %v is not below the upper at %v", y, b[0][1])
	}
}

// The inner and outer cones share one angle, near the book's 26°; the
// inner touches the swept surface and the outer runs through its rims.
func TestBellCones(t *testing.T) {
	c := BellCones()
	full := 2 * c.Half * 180 / math.Pi
	if full < 26 || full > 29 {
		t.Errorf("cones %.3f°, want near the book's 26°", full)
	}
	if c.Wall <= 0 || c.Wall > 0.2 {
		t.Errorf("wall %v", c.Wall)
	}
	if math.Abs(c.outer(c.tip[0])-c.tip[1]) > 1e-9 || math.Abs(c.outer(c.base[0])-c.base[1]) > 1e-9 {
		t.Error("the outer cone misses the rims")
	}
	// The swept surface stays between the two cones.
	p := solidModel(Chestahedron())
	e, tp := p.V[vE1], p.V[vP12]
	for i := range 101 {
		q := lerp(e, tp, float64(i)/100)
		r := math.Hypot(q[0], q[2])
		if r > c.outer(q[1])+1e-9 || r < c.inner(q[1])-1e-9 {
			t.Fatalf("swept radius %v at %v outside the cones %v..%v", r, q[1], c.inner(q[1]), c.outer(q[1]))
		}
	}
	// The tip cone runs through the tips to the peak.
	if math.Abs(c.tipCone(c.tip[0])-c.tip[1]) > 1e-9 {
		t.Error("the tip cone misses the tips")
	}
}

// The fire form is one body: the two hearts share the waist's circle, and
// end on they make a cross of unequal arms, the big heart's lobes one way
// and the small one's the other.
func TestFireBody(t *testing.T) {
	big, small := fireEnds(1)
	for _, e := range []fireEnd{big, small} {
		for _, k := range []float64{1, -1} {
			for i := range 12 {
				v := e.at(0, k, 2*math.Pi*float64(i)/12)
				if math.Abs(v[1]-waistY) > 1e-12 || math.Abs(math.Hypot(v[0], v[2])-waistR) > 1e-12 {
					t.Fatalf("waist point %v is off the waist circle", v)
				}
			}
		}
		if c, rho := e.section(1); rho != 0 || c <= 0 {
			t.Errorf("the end closes at c %v, radius %v", c, rho)
		}
	}
	var wx, wz float64
	for _, v := range Form(FormFire, 1, 2).P {
		if v[1] < waistY {
			wx = math.Max(wx, math.Abs(v[0]))
		} else {
			wz = math.Max(wz, math.Abs(v[2]))
		}
	}
	if r := wx / wz; r < 1.4 || r > 1.9 {
		t.Errorf("arms %v and %v, ratio %v: want the big about φ the small", wx, wz, r)
	}
	if b, s := fireEnds(0); b.w.sub(s.w).len() > 1e-12 {
		t.Error("at 0 the two hearts' lobes should lie in one plane")
	}
	// The body's two ends are the same distance from the center.
	if top, bottom := waistY+fireLen/phi, waistY-fireLen; math.Abs(top+bottom) > 1e-12 {
		t.Error("the body is not centered")
	}
}

// At 1 the halves' base corners make a six-pointed star (six directions 60°
// apart); at 2 they are one solid again.
func TestBeat(t *testing.T) {
	p := solidModel(Chestahedron())
	az := map[int]bool{}
	for _, sg := range []float64{1, -1} {
		q := turnPoly(p, sg*math.Pi/6)
		for _, v := range q.V[:3] {
			a := math.Atan2(v[2], v[0]) * 180 / math.Pi
			az[int(math.Round(a+360))%360] = true
		}
	}
	if len(az) != 6 {
		t.Fatalf("base corners at %v, want six directions", az)
	}
	for a := range az {
		if !az[(a+60)%360] {
			t.Errorf("no corner 60° on from %d", a)
		}
	}
	a, b := turnPoly(p, math.Pi/3), turnPoly(p, -math.Pi/3)
	for _, v := range a.V {
		found := false
		for _, w := range b.V {
			if v.sub(w).len() < 1e-9 {
				found = true
			}
		}
		if !found {
			t.Fatalf("at 2 the halves differ: %v", v)
		}
	}
}

// The bubble is the solid when sharp, inside it always, and the right
// ventricle holds the left's volume.
func TestBubble(t *testing.T) {
	p := solidModel(Chestahedron())
	ps := facePlanes(p)
	for _, m := range []float64{0, 0.5, 1, 2} {
		pw := bubblePower(m)
		for i := range 13 {
			for j := range 24 {
				u := bubbleDir(math.Pi*float64(i)/12, 2*math.Pi*float64(j)/24)
				v := u.mul(bubbleR(ps, u, pw))
				for _, q := range ps {
					if q.n.dot(v) > q.d+1e-9 {
						t.Fatalf("m %v: bubble point %v outside the solid", m, v)
					}
				}
			}
		}
		lv, rv, _ := Ventricles(m)
		if math.Abs(lv-rv) > 1e-6*lv {
			t.Errorf("m %v: left %v, right %v", m, lv, rv)
		}
	}
	// Sharp, it reaches the peak.
	peak := p.V[vPeak]
	if r := bubbleR(ps, peak.unit(), bubblePower(0)); math.Abs(r-peak.len()) > 0.05*peak.len() {
		t.Errorf("sharp bubble reaches %v toward the peak, the peak is at %v", r, peak.len())
	}
}
