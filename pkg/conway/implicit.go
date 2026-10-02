package conway

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// A convex solid as an equation.
//
// A convex polyhedron is where every one of its faces' half-spaces overlap:
// inside, n·p is below d for each face (n its outward normal, d its distance
// from the center). So max over the faces of n·p − d is negative inside,
// positive outside and zero on the surface — the whole solid in one line, in
// the language the Custom model reads. A cube is max(abs(x), abs(y), abs(z))
// against its half-width, an octahedron abs(x) + abs(y) + abs(z) against its.
//
// The abs terms are not decoration. A pair of faces n and −n is one term with
// abs, and a set of faces that runs through every sign of one normal — the
// octahedron's eight — collapses into a single sum of abs values. Written out
// face by face, the dodecahedron is twelve terms; this way it is three.

// plane is a face's plane: unit outward normal and distance.
type plane struct {
	n [3]float64
	d float64
}

// Implicit is the solid's equation, F(x, y, z) with F = 0 on its surface and
// negative inside. ok is false for a solid that is not convex (pyramids
// pressed into its faces), which no single max of planes describes.
func Implicit(s Solid) (eq string, ok bool) {
	var planes []plane
	seen := map[[4]int64]bool{}
	for _, f := range s.Faces {
		if len(f) < 3 {
			continue
		}
		p := facePlane(s, f)
		k := [4]int64{round6(p.n[0]), round6(p.n[1]), round6(p.n[2]), round6(p.d)}
		if seen[k] {
			continue // coplanar faces are one plane
		}
		seen[k] = true
		planes = append(planes, p)
	}
	if len(planes) == 0 {
		return "", false
	}
	// Convex if every vertex is on the inner side of every plane.
	for _, p := range planes {
		for _, v := range s.Verts {
			if p.n[0]*v.X+p.n[1]*v.Y+p.n[2]*v.Z > p.d+1e-6 {
				return "", false
			}
		}
	}
	terms, ds := groupPlanes(planes)
	// One distance for every term (a regular solid): factored out, and the
	// equation scaled so its first term's largest coefficient is 1.
	same := true
	for _, d := range ds[1:] {
		if math.Abs(d-ds[0]) > 1e-6 {
			same = false
		}
	}
	scale := 1 / maxAbs(terms[0].coef)
	var parts []string
	for i, t := range terms {
		s := t.String(scale)
		if !same {
			s += " - " + num(ds[i]*scale)
		}
		parts = append(parts, s)
	}
	body := parts[0]
	if len(parts) > 1 {
		body = "max(" + strings.Join(parts, ", ") + ")"
	}
	if same {
		body += " - " + num(ds[0]*scale)
	}
	return body, true
}

// facePlane is a face's plane, by Newell's method (exact for a planar face,
// the best fit for a slightly bent one), pointing away from the center.
func facePlane(s Solid, f []int) plane {
	var n [3]float64
	var c [3]float64
	for i := range f {
		a, b := s.Verts[f[i]], s.Verts[f[(i+1)%len(f)]]
		n[0] += (a.Y - b.Y) * (a.Z + b.Z)
		n[1] += (a.Z - b.Z) * (a.X + b.X)
		n[2] += (a.X - b.X) * (a.Y + b.Y)
		c[0], c[1], c[2] = c[0]+a.X, c[1]+a.Y, c[2]+a.Z
	}
	l := math.Sqrt(n[0]*n[0] + n[1]*n[1] + n[2]*n[2])
	if l == 0 {
		return plane{}
	}
	for i := range n {
		n[i] /= l
		c[i] /= float64(len(f))
	}
	d := n[0]*c[0] + n[1]*c[1] + n[2]*c[2]
	if d < 0 {
		n = [3]float64{-n[0], -n[1], -n[2]}
		d = -d
	}
	return plane{n, d}
}

// term is one argument of the max: coefficients on x, y and z, each either
// signed or taken as abs.
type term struct {
	coef [3]float64
	abs  [3]bool
}

// String writes the term, its coefficients multiplied by scale.
func (t term) String(scale float64) string {
	var b strings.Builder
	for i, v := range [3]string{"x", "y", "z"} {
		c := t.coef[i] * scale
		if math.Abs(c) < 1e-9 {
			continue
		}
		neg := c < 0
		c = math.Abs(c)
		switch {
		case b.Len() == 0 && neg:
			b.WriteString("-")
		case b.Len() > 0 && neg:
			b.WriteString(" - ")
		case b.Len() > 0:
			b.WriteString(" + ")
		}
		if math.Abs(c-1) > 5e-4 {
			b.WriteString(num(c) + "*")
		}
		if t.abs[i] {
			b.WriteString("abs(" + v + ")")
		} else {
			b.WriteString(v)
		}
	}
	return b.String()
}

// groupPlanes collapses planes that differ only in the signs of their
// normals, and have every combination of those signs, into abs terms; the
// rest stay as they are. The distances come back beside the terms.
func groupPlanes(planes []plane) ([]term, []float64) {
	type key [4]int64 // |n| components and d
	groups := map[key][]plane{}
	var order []key
	for _, p := range planes {
		k := key{round6(math.Abs(p.n[0])), round6(math.Abs(p.n[1])), round6(math.Abs(p.n[2])), round6(p.d)}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], p)
	}
	// In a reading order: x before y before z, whatever order the faces
	// came in.
	sort.SliceStable(order, func(a, b int) bool {
		for i := range 4 {
			if order[a][i] != order[b][i] {
				return order[a][i] > order[b][i]
			}
		}
		return false
	})
	var terms []term
	var ds []float64
	for _, k := range order {
		g := groups[k]
		nonzero := 0
		var t term
		for i := range 3 {
			t.coef[i] = float64(k[i]) / 1e6
			if k[i] != 0 {
				nonzero++
			}
		}
		if len(g) == 1<<nonzero {
			for i := range 3 {
				t.abs[i] = k[i] != 0
			}
			terms = append(terms, t)
			ds = append(ds, g[0].d)
			continue
		}
		// Not every sign: one term a face, in a stable order.
		sort.Slice(g, func(a, b int) bool {
			for i := range 3 {
				if g[a].n[i] != g[b].n[i] {
					return g[a].n[i] > g[b].n[i]
				}
			}
			return false
		})
		for _, p := range g {
			terms = append(terms, term{coef: p.n})
			ds = append(ds, p.d)
		}
	}
	return terms, ds
}

func round6(v float64) int64 { return int64(math.Round(v * 1e6)) }

func maxAbs(c [3]float64) float64 {
	return math.Max(math.Abs(c[0]), math.Max(math.Abs(c[1]), math.Abs(c[2])))
}

// num writes a coefficient to three decimals, without trailing zeros.
func num(v float64) string {
	s := strconv.FormatFloat(v, 'f', 3, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" {
		return "0"
	}
	return s
}
