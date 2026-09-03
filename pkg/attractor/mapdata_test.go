package attractor

import (
	"math"
	"testing"
)

// A map that escapes to infinity draws nothing, and a map that collapses to a
// point draws one dot. Both are what a wrong coefficient looks like, so the
// test is that every registered map stays bounded AND keeps moving.
func TestEveryMapIsBoundedAndAlive(t *testing.T) {
	for _, k := range MapKeys() {
		step, ic, ok := MapStep(k)
		if !ok {
			t.Fatalf("%s: not registered", k)
		}
		p := ic
		// A LONG transient. 500 iterates was not enough and let a bad default
		// ship: Ikeda at u=0.918 sits in a periodic window and spirals onto a
		// fixed point, but the spiral in is wide and slow, so a short warmup
		// measured the approach and called it an attractor. On screen it was a
		// single dot. Settling has to be given time to happen before the
		// question "is there anything here" can be asked.
		for i := 0; i < 200000; i++ {
			p[0], p[1], p[2] = step(p[0], p[1], p[2])
		}
		var minX, maxX, minY, maxY = math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
		for i := 0; i < 20000; i++ {
			p[0], p[1], p[2] = step(p[0], p[1], p[2])
			for _, v := range p {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					t.Fatalf("%s: left the reals after %d iterates: %v", k, i, p)
				}
			}
			if math.Abs(p[0]) > 1e4 || math.Abs(p[1]) > 1e4 {
				t.Fatalf("%s: escaped to %v after %d iterates", k, p, i)
			}
			minX, maxX = math.Min(minX, p[0]), math.Max(maxX, p[0])
			minY, maxY = math.Min(minY, p[1]), math.Max(maxY, p[1])
		}
		// A fixed point or a tiny cycle is not a picture.
		if w, h := maxX-minX, maxY-minY; w < 0.05 || h < 0.05 {
			t.Errorf("%s: collapsed to %.4f x %.4f — a point, not an attractor", k, w, h)
		}
	}
}

// Hénon's is the one map here with an exactly known invariant: the Jacobian
// determinant is −b everywhere, so area contracts by exactly b per iterate.
// If the equation were mistyped this would not hold.
func TestHenonContractsAreaByB(t *testing.T) {
	step, _, _ := MapStep("henon")
	const h = 1e-6
	for _, at := range [][2]float64{{0, 0}, {0.5, 0.1}, {-0.8, 0.2}, {1.0, -0.3}} {
		x, y := at[0], at[1]
		x0, y0, _ := step(x, y, 0)
		x1, y1, _ := step(x+h, y, 0)
		x2, y2, _ := step(x, y+h, 0)
		// Jacobian by finite difference.
		a, c := (x1-x0)/h, (y1-y0)/h
		b, d := (x2-x0)/h, (y2-y0)/h
		det := a*d - b*c
		if want := -float64(henonB); math.Abs(det-want) > 1e-4 {
			t.Errorf("at %v the Jacobian determinant is %.6f, want %.6f (= −b)", at, det, want)
		}
	}
}

// The standard map is AREA-PRESERVING — that is its defining property and the
// reason it is drawn as an ensemble rather than as one orbit. A determinant
// that is not 1 means it has been written as a dissipative map by mistake,
// and the phase portrait would be a lie.
func TestStandardMapPreservesArea(t *testing.T) {
	step, _, _ := MapStep("standardmap")
	const h = 1e-7
	// Away from the 2π fold, where the finite difference would straddle a wrap.
	for _, at := range [][2]float64{{1.0, 1.0}, {2.0, 3.0}, {0.5, 4.0}, {3.0, 2.0}} {
		th, p := at[0], at[1]
		t0, p0, _ := step(th, p, 0)
		t1, p1, _ := step(th+h, p, 0)
		t2, p2, _ := step(th, p+h, 0)
		a, c := (t1-t0)/h, (p1-p0)/h
		b, d := (t2-t0)/h, (p2-p0)/h
		if det := a*d - b*c; math.Abs(det-1) > 1e-3 {
			t.Errorf("at %v the Jacobian determinant is %.6f, want 1 — the map is not area-preserving", at, det)
		}
	}
}

// The standard map lives on a torus. Without the fold a chaotic orbit's
// momentum walks off without bound and the figure becomes a diagonal smear
// instead of a phase portrait.
func TestStandardMapStaysOnTheTorus(t *testing.T) {
	step, _, _ := MapStep("standardmap")
	th, p := 1.0, 2.0
	for i := 0; i < 50000; i++ {
		th, p, _ = step(th, p, 0)
		if th < 0 || th >= 2*math.Pi+1e-9 || p < 0 || p >= 2*math.Pi+1e-9 {
			t.Fatalf("iterate %d left [0,2π): θ=%v p=%v", i, th, p)
		}
	}
}

// Sensitive dependence is what makes these worth drawing: two orbits a
// whisker apart must separate. A map that fails this is periodic, and the
// "strange" in strange attractor is unearned.
func TestDissipativeMapsAreSensitive(t *testing.T) {
	// Two are excluded on their merits rather than to make the test pass.
	//
	// standardmap is area-preserving and has no attractor: whether nearby
	// orbits separate depends entirely on whether you started in an island or
	// in the chaotic sea, so a single pass/fail is meaningless.
	//
	// mira is quasi-periodic at its defaults, and that is not a defect. From
	// 1e-12 its orbits reach only ~1e-8 after a thousand iterates and never
	// saturate, where Hénon, Clifford and Tinkerbell all reach O(1) within a
	// few hundred. Its famous figures ARE invariant curves — nested organic
	// rings, not a fractal dust — so requiring chaos of it would be requiring
	// the wrong thing.
	notChaotic := map[string]bool{"standardmap": true, "mira": true}
	for _, k := range MapKeys() {
		if notChaotic[k] {
			continue
		}
		step, ic, _ := MapStep(k)
		a := ic
		// Settle onto the attractor before asking whether nearby orbits
		// separate — the same trap as above: the approach to a fixed point
		// looks like motion, and two orbits converging to it look like they
		// are diverging while they still have far to fall.
		for i := 0; i < 200000; i++ {
			a[0], a[1], a[2] = step(a[0], a[1], a[2])
		}
		b := a
		b[0] += 1e-9
		for i := 0; i < 500; i++ {
			a[0], a[1], a[2] = step(a[0], a[1], a[2])
			b[0], b[1], b[2] = step(b[0], b[1], b[2])
		}
		sep := math.Hypot(a[0]-b[0], a[1]-b[1])
		if sep < 1e-6 {
			t.Errorf("%s: orbits 1e-9 apart are still %.2e apart after 500 iterates — not chaotic at its defaults", k, sep)
		}
	}
}

// Every map must be in the catalog, and every catalog entry claiming to be a
// map must actually be one. The two lists drifting apart is how a mode ends
// up unreachable.
func TestMapsAreInTheCatalogAsMaps(t *testing.T) {
	inCatalog := map[string]bool{}
	for _, g := range Catalog() {
		for _, m := range g.Models {
			if m.Class == ClassMap {
				inCatalog[m.Key] = true
				if !IsMap(m.Key) {
					t.Errorf("%q is cataloged as a map but has no registered step function", m.Key)
				}
				if m.Description == "" {
					t.Errorf("%q has no description", m.Key)
				}
			}
		}
	}
	for _, k := range MapKeys() {
		if !inCatalog[k] {
			t.Errorf("map %q is registered but not in the catalog — nothing can select it", k)
		}
	}
	if len(inCatalog) < 7 {
		t.Errorf("only %d maps cataloged", len(inCatalog))
	}
}
