package meshstl

import "math"

// The five Platonic solids, as closed solids rather than the wireframes the
// app draws. Each is built from its vertex set and its face list, and the
// faces are wound outward by testing each one against the center — which is
// sound because all five are convex and centered on the origin, and which
// beats transcribing thirty-six winding-correct triangles by hand.

// Tetrahedron with circumradius r.
func Tetrahedron(r float64) Mesh {
	v := []V3{{1, 1, 1}, {1, -1, -1}, {-1, 1, -1}, {-1, -1, 1}}
	return faces(scaleToRadius(v, r), [][]int{{0, 1, 2}, {0, 3, 1}, {0, 2, 3}, {1, 3, 2}})
}

// Hexahedron (cube) with circumradius r.
func Hexahedron(r float64) Mesh {
	v := []V3{
		{-1, -1, -1}, {1, -1, -1}, {1, 1, -1}, {-1, 1, -1},
		{-1, -1, 1}, {1, -1, 1}, {1, 1, 1}, {-1, 1, 1},
	}
	return faces(scaleToRadius(v, r), [][]int{
		{0, 1, 2, 3}, {4, 5, 6, 7}, {0, 1, 5, 4},
		{2, 3, 7, 6}, {1, 2, 6, 5}, {0, 3, 7, 4},
	})
}

// Octahedron with circumradius r.
func Octahedron(r float64) Mesh {
	v := []V3{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}}
	return faces(scaleToRadius(v, r), [][]int{
		{0, 2, 4}, {2, 1, 4}, {1, 3, 4}, {3, 0, 4},
		{2, 0, 5}, {1, 2, 5}, {3, 1, 5}, {0, 3, 5},
	})
}

// Icosahedron with circumradius r.
func Icosahedron(r float64) Mesh {
	p := (1 + math.Sqrt(5)) / 2
	v := []V3{
		{-1, p, 0}, {1, p, 0}, {-1, -p, 0}, {1, -p, 0},
		{0, -1, p}, {0, 1, p}, {0, -1, -p}, {0, 1, -p},
		{p, 0, -1}, {p, 0, 1}, {-p, 0, -1}, {-p, 0, 1},
	}
	return faces(scaleToRadius(v, r), [][]int{
		{0, 11, 5}, {0, 5, 1}, {0, 1, 7}, {0, 7, 10}, {0, 10, 11},
		{1, 5, 9}, {5, 11, 4}, {11, 10, 2}, {10, 7, 6}, {7, 1, 8},
		{3, 9, 4}, {3, 4, 2}, {3, 2, 6}, {3, 6, 8}, {3, 8, 9},
		{4, 9, 5}, {2, 4, 11}, {6, 2, 10}, {8, 6, 7}, {9, 8, 1},
	})
}

// Dodecahedron with circumradius r. Its twenty vertices are a cube's eight
// plus three rectangles in the coordinate planes, scaled by the golden ratio.
func Dodecahedron(r float64) Mesh {
	p := (1 + math.Sqrt(5)) / 2
	ip := 1 / p
	v := []V3{
		{1, 1, 1}, {1, 1, -1}, {1, -1, 1}, {1, -1, -1},
		{-1, 1, 1}, {-1, 1, -1}, {-1, -1, 1}, {-1, -1, -1},
		{0, ip, p}, {0, ip, -p}, {0, -ip, p}, {0, -ip, -p},
		{ip, p, 0}, {ip, -p, 0}, {-ip, p, 0}, {-ip, -p, 0},
		{p, 0, ip}, {p, 0, -ip}, {-p, 0, ip}, {-p, 0, -ip},
	}
	return faces(scaleToRadius(v, r), [][]int{
		{0, 8, 10, 2, 16}, {0, 16, 17, 1, 12}, {0, 12, 14, 4, 8},
		{1, 17, 3, 11, 9}, {1, 9, 5, 14, 12}, {2, 10, 6, 15, 13},
		{2, 13, 3, 17, 16}, {3, 13, 15, 7, 11}, {4, 14, 5, 19, 18},
		{4, 18, 6, 10, 8}, {5, 9, 11, 7, 19}, {6, 18, 19, 7, 15},
	})
}

// NestedCube is the app's nested-cube figure as a solid: an outer shell and
// an inner cube, with the eight struts that join their corresponding corners.
func NestedCube(r, inner, strut float64) Mesh {
	var m Mesh
	m.Append(Hexahedron(r))
	m.Append(Hexahedron(r * inner))
	// A cube's corners in the same order Hexahedron uses.
	corner := func(i int, s float64) V3 {
		v := []V3{
			{-1, -1, -1}, {1, -1, -1}, {1, 1, -1}, {-1, 1, -1},
			{-1, -1, 1}, {1, -1, 1}, {1, 1, 1}, {-1, 1, 1},
		}[i]
		return v.Mul(s / math.Sqrt(3))
	}
	for i := 0; i < 8; i++ {
		m.Append(Tube([]V3{corner(i, r*inner), corner(i, r)}, strut, 8, true))
	}
	return m
}

// scaleToRadius normalizes a vertex set onto a sphere of radius r. The
// coordinate sets above are the conventional integer ones, whose circumradius
// differs per solid; this makes the five directly comparable.
func scaleToRadius(v []V3, r float64) []V3 {
	out := make([]V3, len(v))
	for i, p := range v {
		out[i] = p.Norm().Mul(r)
	}
	return out
}

// faces triangulates a face list as fans and winds each triangle outward.
func faces(v []V3, list [][]int) Mesh {
	var m Mesh
	for _, f := range list {
		for i := 1; i+1 < len(f); i++ {
			t := Tri{v[f[0]], v[f[i]], v[f[i+1]]}
			// Outward means the normal points away from the origin, which the
			// solid is centered on.
			mid := t.A.Add(t.B).Add(t.C).Mul(1.0 / 3)
			if t.Normal().Dot(mid) < 0 {
				t = Tri{t.A, t.C, t.B}
			}
			m.Add(t)
		}
	}
	return m
}
