package sacred

import "math"

// The Sri Yantra: nine triangles, four pointing up and five down, crossing
// in 43 smaller ones round a central point, inside two rings of lotus
// petals and a square frame with four gates (the bhupura).
//
// The triangles are a hard construction. Their sides must meet three at a
// time at the marmas, which over-determines the figure: Huet showed it a
// Euclidean problem with four free parameters, and Kulaichev reduced it to
// a polynomial. The nine below are the solved coordinates published with
// texample.net's TikZ drawing (after Kulaichev 1984 and Huet), each as its
// base's height, its base's half width and its apex's height, in a circle
// of radius 100 centered at (150, 150) with y downward. sriyantra_test.go
// checks what can be checked: each apex lies on another triangle's base, the
// two largest bases end on the circle, and the sides meet three at a time.

// sriTriangle is one of the nine, in the source's units: base y, left x,
// apex y. Up when the apex is above the base (smaller y).
type sriTriangle struct{ base, left, apex float64 }

var sriTriangles = []sriTriangle{
	{123.20508075688774, 53.65669559977147, 250},                 // D1
	{174.24660560764943, 52.984011026495736, 50},                 // U1
	{220.03828947357886, 98.71823312733801, 123.20508075688774},  // U3
	{197.92315674002487, 78.26467997914015, 78.10499177949904},   // U2
	{78.10499177949904, 90.4856922951427, 160.66014976539617},    // D3
	{103.12199145016105, 80.98384838952128, 220.03828947357886},  // D2
	{160.66014976539617, 114.9488500600036, 103.12199145016105},  // U4
	{134.30757626706648, 116.35142605010424, 197.92315674002487}, // D4
	{144.79777263138968, 124.61190803072795, 174.24660560764943}, // D5
}

// sriPoint is a point of the source in the unit circle, y up.
func sriPoint(x, y float64) V { return V{(x - 150) / 100, -(y - 150) / 100, 0} }

// sriSides are the nine triangles as their three corners, largest first.
func sriSides() [][3]V {
	out := make([][3]V, len(sriTriangles))
	for i, t := range sriTriangles {
		out[i] = [3]V{sriPoint(t.left, t.base), sriPoint(150, t.apex), sriPoint(300-t.left, t.base)}
	}
	return out
}

// sriYantra is the yantra flat: the nine triangles and the bindu at the
// center; with detail, the circle round them (1), the two lotus rings of
// eight and sixteen petals and the circles between (2), and the bhupura,
// three squares with a gate on each side (3).
func sriYantra(d int) []stroke {
	var out []stroke
	for _, t := range sriSides() {
		out = append(out, stroke{t[0], t[1], t[2], t[0]})
	}
	out = append(out, circle(V{0, sriBindu(), 0}, 0.02, 0, 12))
	if d >= 1 {
		out = append(out, circle(V{}, 1, math.Pi/2, 128))
	}
	if d >= 2 {
		out = append(out, petals(8, 1, 1.2, 0)...)
		out = append(out, circle(V{}, 1.2, math.Pi/2, 128))
		out = append(out, petals(16, 1.2, 1.4, 0)...)
		out = append(out, circle(V{}, 1.4, math.Pi/2, 128), circle(V{}, 1.5, math.Pi/2, 128))
	}
	if d >= 3 {
		for _, s := range []float64{1.65, 1.75, 1.85} {
			out = append(out, bhupura(s, 0))
		}
	}
	return out
}

// sriBindu is the height of the bindu, the point at the center of the
// innermost triangle (D5), in the unit circle.
func sriBindu() float64 {
	t := sriSides()[8]
	return (t[0][1] + t[1][1] + t[2][1]) / 3
}

// petals is a ring of n lotus petals between radii r0 and r1, each a curve
// from one point on the inner circle out to a tip and back, at height z.
func petals(n int, r0, r1, z float64) []stroke {
	out := make([]stroke, 0, n)
	half := math.Pi / float64(n)
	for k := range n {
		a := math.Pi/2 + 2*half*float64(k)
		s := make(stroke, 17)
		for j := range s {
			t := float64(j)/8 - 1 // −1 … 1 across the petal
			r := r0 + (r1-r0)*math.Sqrt(1-t*t)
			s[j] = V{r * math.Cos(a+half*t), r * math.Sin(a+half*t), z}
		}
		out = append(out, s)
	}
	return out
}

// bhupura is the square frame of half-side s at height z, with a T-shaped
// gate opening out of the middle of each side.
func bhupura(s, z float64) stroke {
	// One side's gate, along +y, left to right; turned for the others.
	gate := [][2]float64{{-s, s}, {-0.25, s}, {-0.25, s + 0.12}, {-0.45, s + 0.12}, {-0.45, s + 0.27},
		{0.45, s + 0.27}, {0.45, s + 0.12}, {0.25, s + 0.12}, {0.25, s}}
	var out stroke
	for side := range 4 {
		c, sn := math.Cos(-float64(side)*math.Pi/2), math.Sin(-float64(side)*math.Pi/2)
		for _, p := range gate {
			out = append(out, V{p[0]*c - p[1]*sn, p[0]*sn + p[1]*c, z})
		}
	}
	return append(out, out[0])
}

// meru is the Sri Yantra in space, the Meru: the same figure raised as a
// mountain, each enclosure a step above the one outside it — the bhupura on
// the ground, the petals and circles above, then the nine triangles, the
// larger lower, up to the bindu at the summit.
func meru(d int) []stroke {
	var out []stroke
	if d >= 1 {
		for i, s := range []float64{1.85, 1.75, 1.65} {
			out = append(out, bhupura(s, 0.05*float64(i)))
		}
		out = append(out, petals(16, 1.2, 1.4, 0.25)...)
		out = append(out, circle(V{0, 0, 0.25}, 1.4, math.Pi/2, 128))
		out = append(out, petals(8, 1, 1.2, 0.45)...)
		out = append(out, circle(V{0, 0, 0.45}, 1.2, math.Pi/2, 128))
	}
	sides := sriSides()
	for i, t := range sides {
		z := 0.6 + 0.12*float64(i)
		out = append(out, stroke{{t[0][0], t[0][1], z}, {t[1][0], t[1][1], z}, {t[2][0], t[2][1], z}, {t[0][0], t[0][1], z}})
	}
	top := 0.6 + 0.12*float64(len(sides))
	return append(out, line(V{0, sriBindu(), top - 0.12}, V{0, sriBindu(), top + 0.15}))
}
