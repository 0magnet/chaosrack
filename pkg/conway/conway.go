package conway

import (
	"math"
	"sort"
)

// Polyhedra as faces, and the Conway operators over them.
//
// The polyhedra in this rack were six hand-written lists of vertices and
// EDGE pairs — a wireframe and nothing more. Six models, no tunable
// constant between them, and a rack row with a selector and no knobs.
//
// A polyhedron with FACES is a different object: Conway's operators turn one
// solid into another, and the five Platonic seeds under a handful of them
// give every Archimedean solid. The same row becomes a generator — seed on
// one knob, operator on the other — and the six fixed models become a few
// dozen reachable solids, which is what the rack's other rows already are:
// a system and the knobs that move it through its family.
//
// The notation is Conway's, read right to left, seed last: tC is a truncated
// cube, aC a cuboctahedron, taC a truncated cuboctahedron. Implemented here:
//
//	d  dual      faces become vertices and vertices faces
//	a  ambo      vertices at edge midpoints; the "rectified" solid
//	k  kis       raise a pyramid on every face
//	t  truncate  cut every vertex off  (t = dkd)
//	e  expand    a = aa;  pull faces apart, squares in the gaps
//	b  bevel     t of a
//
// This file is pure and has no browser in it: a polyhedron is arithmetic,
// and Euler's formula is a test that does not need a screen.

// Vert is a point on the unit-ish sphere. Not normalized in general —
// the operators keep whatever scale the seed had, and the renderer fits the
// camera to the result.
type Vert struct{ X, Y, Z float64 }

// Solid is vertices plus faces, each face a loop of vertex indices
// wound consistently. Edges are derived rather than stored, because an edge
// list that disagrees with the faces is the bug this type exists to prevent.
type Solid struct {
	Verts []Vert
	Faces [][]int
}

// Edges returns each undirected edge once, as an index pair, in a stable
// order. This is what the wireframe is drawn from.
func (p Solid) Edges() [][2]int {
	seen := map[[2]int]bool{}
	var out [][2]int
	for _, f := range p.Faces {
		for i := range f {
			a, b := f[i], f[(i+1)%len(f)]
			if a > b {
				a, b = b, a
			}
			if a == b || seen[[2]int{a, b}] {
				continue
			}
			seen[[2]int{a, b}] = true
			out = append(out, [2]int{a, b})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}

// Euler is V - E + F, which is 2 for any polyhedron homeomorphic to a
// sphere. Every operator here preserves it, so it is the one number that
// says a result is a solid rather than a bag of triangles.
func (p Solid) Euler() int { return len(p.Verts) - len(p.Edges()) + len(p.Faces) }

// center is a face's centroid.
func (p Solid) center(f []int) Vert {
	var c Vert
	if len(f) == 0 {
		return c
	}
	for _, i := range f {
		c.X += p.Verts[i].X
		c.Y += p.Verts[i].Y
		c.Z += p.Verts[i].Z
	}
	n := float64(len(f))
	return Vert{c.X / n, c.Y / n, c.Z / n}
}

func lerp(a, b Vert, t float64) Vert {
	return Vert{a.X + (b.X-a.X)*t, a.Y + (b.Y-a.Y)*t, a.Z + (b.Z-a.Z)*t}
}

// normalize puts every vertex on the unit sphere. Conway operators do not
// produce canonical solids — a truncated cube from this code has the right
// combinatorics but not equal edge lengths — and projecting to the sphere is
// what makes the wireframe look like the solid it names rather than like a
// lumpy version of it.
func (p Solid) normalize() Solid {
	out := Solid{Verts: make([]Vert, len(p.Verts)), Faces: p.Faces}
	for i, v := range p.Verts {
		d := math.Sqrt(v.X*v.X + v.Y*v.Y + v.Z*v.Z)
		if d == 0 {
			out.Verts[i] = v
			continue
		}
		out.Verts[i] = Vert{v.X / d, v.Y / d, v.Z / d}
	}
	return out
}

// reversed is a face loop wound the other way.
//
// A loop walked around a VERTEX visits its faces in the opposite rotational
// sense from a loop walked around a FACE. Both kinds appear in the same
// polyhedron, so one of them has to be turned around or the result is not
// consistently wound — and a polyhedron that is not consistently wound
// breaks the next operator rather than itself, because the vertex walk
// works by finding the face holding the reverse of each directed edge.
func reversed(loop []int) []int {
	out := make([]int, len(loop))
	for i, v := range loop {
		out[len(loop)-1-i] = v
	}
	return out
}

// ── the operators ─────────────────────────────────────────────────────────

// dual: one vertex per face, one face per vertex.
//
// The new face around an old vertex has to be WOUND, not just collected, or
// it is a set of points and not a polygon. The winding comes from walking
// the faces around the vertex through their shared edges.
func (p Solid) dual() Solid {
	out := Solid{Verts: make([]Vert, len(p.Faces))}
	for i, f := range p.Faces {
		out.Verts[i] = p.center(f)
	}
	// The face holding each directed edge, so the faces around a vertex
	// can be walked in order.
	owner := map[[2]int]int{}
	for fi, f := range p.Faces {
		for i := range f {
			owner[[2]int{f[i], f[(i+1)%len(f)]}] = fi
		}
	}
	for v := range p.Verts {
		// Start at any face containing v, then repeatedly cross the edge
		// leaving v to reach the next face around it.
		start := -1
		for fi, f := range p.Faces {
			for _, x := range f {
				if x == v {
					start = fi
					break
				}
			}
			if start >= 0 {
				break
			}
		}
		if start < 0 {
			continue // a vertex no face uses: nothing to make a face from
		}
		var loop []int
		cur := start
		for range p.Faces {
			loop = append(loop, cur)
			f := p.Faces[cur]
			// the edge of this face that LEAVES v
			nxt := -1
			for i := range f {
				if f[i] == v {
					nxt = f[(i+1)%len(f)]
					break
				}
			}
			if nxt < 0 {
				break
			}
			adj, ok := owner[[2]int{nxt, v}] // the face with the reverse edge
			if !ok || adj == start {
				break
			}
			cur = adj
		}
		if len(loop) >= 3 {
			out.Faces = append(out.Faces, reversed(loop))
		}
	}
	return out
}

// kis: a pyramid on every face. The new apex is the face centroid, pushed
// out by h times the centroid's own length.
func (p Solid) kis(h float64) Solid {
	out := Solid{Verts: append([]Vert(nil), p.Verts...)}
	for _, f := range p.Faces {
		c := p.center(f)
		apex := Vert{c.X * (1 + h), c.Y * (1 + h), c.Z * (1 + h)}
		out.Verts = append(out.Verts, apex)
		a := len(out.Verts) - 1
		for i := range f {
			out.Faces = append(out.Faces, []int{f[i], f[(i+1)%len(f)], a})
		}
	}
	return out
}

// ambo: a vertex at every edge midpoint. Each original face becomes a
// smaller face through its edge midpoints, and each original vertex becomes
// a face through the midpoints of the edges meeting it.
func (p Solid) ambo() Solid {
	var out Solid
	mid := map[[2]int]int{}
	id := func(a, b int) int {
		k := [2]int{a, b}
		if a > b {
			k = [2]int{b, a}
		}
		if i, ok := mid[k]; ok {
			return i
		}
		out.Verts = append(out.Verts, lerp(p.Verts[k[0]], p.Verts[k[1]], 0.5))
		mid[k] = len(out.Verts) - 1
		return mid[k]
	}
	// One face per original face.
	for _, f := range p.Faces {
		var nf []int
		for i := range f {
			nf = append(nf, id(f[i], f[(i+1)%len(f)]))
		}
		if len(nf) >= 3 {
			out.Faces = append(out.Faces, nf)
		}
	}
	// One face per original vertex, wound by walking faces around it.
	owner := map[[2]int]int{}
	for fi, f := range p.Faces {
		for i := range f {
			owner[[2]int{f[i], f[(i+1)%len(f)]}] = fi
		}
	}
	for v := range p.Verts {
		start := -1
		for fi, f := range p.Faces {
			for _, x := range f {
				if x == v {
					start = fi
					break
				}
			}
			if start >= 0 {
				break
			}
		}
		if start < 0 {
			continue
		}
		var loop []int
		cur := start
		for range p.Faces {
			f := p.Faces[cur]
			nxt := -1
			for i := range f {
				if f[i] == v {
					nxt = f[(i+1)%len(f)]
					break
				}
			}
			if nxt < 0 {
				break
			}
			loop = append(loop, id(v, nxt))
			adj, ok := owner[[2]int{nxt, v}]
			if !ok || adj == start {
				break
			}
			cur = adj
		}
		if len(loop) >= 3 {
			out.Faces = append(out.Faces, reversed(loop))
		}
	}
	return out
}

// truncate cuts every vertex off, at t of the way along each edge.
//
// Done directly rather than as dkd: the direct construction keeps the
// original faces recognizable, and dkd through this code's kis would need a
// height that happens to land on the right plane.
func (p Solid) truncate(t float64) Solid {
	if t <= 0 || t >= 0.5 {
		t = 1.0 / 3.0
	}
	var out Solid
	// One new vertex per DIRECTED edge: the point t of the way from a to b.
	cut := map[[2]int]int{}
	id := func(a, b int) int {
		if i, ok := cut[[2]int{a, b}]; ok {
			return i
		}
		out.Verts = append(out.Verts, lerp(p.Verts[a], p.Verts[b], t))
		cut[[2]int{a, b}] = len(out.Verts) - 1
		return cut[[2]int{a, b}]
	}
	// Each original face, with its corners cut.
	for _, f := range p.Faces {
		var nf []int
		for i := range f {
			prev, cur2, next := f[(i+len(f)-1)%len(f)], f[i], f[(i+1)%len(f)]
			nf = append(nf, id(cur2, prev), id(cur2, next))
		}
		if len(nf) >= 3 {
			out.Faces = append(out.Faces, nf)
		}
	}
	// A new face where each vertex was, wound around it.
	owner := map[[2]int]int{}
	for fi, f := range p.Faces {
		for i := range f {
			owner[[2]int{f[i], f[(i+1)%len(f)]}] = fi
		}
	}
	for v := range p.Verts {
		start := -1
		for fi, f := range p.Faces {
			for _, x := range f {
				if x == v {
					start = fi
					break
				}
			}
			if start >= 0 {
				break
			}
		}
		if start < 0 {
			continue
		}
		var loop []int
		cur := start
		for range p.Faces {
			f := p.Faces[cur]
			nxt := -1
			for i := range f {
				if f[i] == v {
					nxt = f[(i+1)%len(f)]
					break
				}
			}
			if nxt < 0 {
				break
			}
			loop = append(loop, id(v, nxt))
			adj, ok := owner[[2]int{nxt, v}]
			if !ok || adj == start {
				break
			}
			cur = adj
		}
		if len(loop) >= 3 {
			out.Faces = append(out.Faces, reversed(loop))
		}
	}
	return out
}

// expand is ambo twice: faces pulled apart with squares in the gaps.
func (p Solid) expand() Solid { return p.ambo().ambo() }

// bevel is truncate of ambo.
func (p Solid) bevel() Solid { return p.ambo().truncate(1.0 / 3.0) }

// ── the seeds ─────────────────────────────────────────────────────────────

// The five Platonic solids, as faces wound consistently outward. These are
// the only hand-written polyhedra; everything else is generated.

func seedTetrahedron() Solid {
	return Solid{
		Verts: []Vert{{1, 1, 1}, {1, -1, -1}, {-1, 1, -1}, {-1, -1, 1}},
		Faces: [][]int{{0, 1, 2}, {0, 3, 1}, {0, 2, 3}, {1, 3, 2}},
	}
}

func seedCube() Solid {
	return Solid{
		Verts: []Vert{
			{-1, -1, -1}, {1, -1, -1}, {1, 1, -1}, {-1, 1, -1},
			{-1, -1, 1}, {1, -1, 1}, {1, 1, 1}, {-1, 1, 1},
		},
		Faces: [][]int{
			{0, 3, 2, 1}, {4, 5, 6, 7}, // -z, +z
			{0, 1, 5, 4}, {2, 3, 7, 6}, // -y, +y
			{1, 2, 6, 5}, {0, 4, 7, 3}, // +x, -x
		},
	}
}

func seedOctahedron() Solid {
	return Solid{
		Verts: []Vert{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}},
		Faces: [][]int{
			{0, 2, 4}, {2, 1, 4}, {1, 3, 4}, {3, 0, 4},
			{2, 0, 5}, {1, 2, 5}, {3, 1, 5}, {0, 3, 5},
		},
	}
}

func seedIcosahedron() Solid {
	g := (1 + math.Sqrt(5)) / 2
	v := []Vert{
		{-1, g, 0}, {1, g, 0}, {-1, -g, 0}, {1, -g, 0},
		{0, -1, g}, {0, 1, g}, {0, -1, -g}, {0, 1, -g},
		{g, 0, -1}, {g, 0, 1}, {-g, 0, -1}, {-g, 0, 1},
	}
	return Solid{Verts: v, Faces: [][]int{
		{0, 11, 5}, {0, 5, 1}, {0, 1, 7}, {0, 7, 10}, {0, 10, 11},
		{1, 5, 9}, {5, 11, 4}, {11, 10, 2}, {10, 7, 6}, {7, 1, 8},
		{3, 9, 4}, {3, 4, 2}, {3, 2, 6}, {3, 6, 8}, {3, 8, 9},
		{4, 9, 5}, {2, 4, 11}, {6, 2, 10}, {8, 6, 7}, {9, 8, 1},
	}}
}

// The dodecahedron is the icosahedron's dual, so it is derived rather than
// written out — one fewer hand-typed vertex table to get wrong.
func seedDodecahedron() Solid { return seedIcosahedron().normalize().dual() }

// Seeds are the seeds a generator row offers, in Conway's own letters.
var Seeds = []struct {
	Letter string
	Name   string
	Make   func() Solid
}{
	{"T", "tetrahedron", seedTetrahedron},
	{"C", "cube", seedCube},
	{"O", "octahedron", seedOctahedron},
	{"D", "dodecahedron", seedDodecahedron},
	{"I", "icosahedron", seedIcosahedron},
}

// Ops are the operators a generator row offers.
var Ops = []struct {
	Letter string
	Name   string
	Apply  func(Solid) Solid
}{
	{"", "none", func(p Solid) Solid { return p }},
	{"d", "dual", func(p Solid) Solid { return p.dual() }},
	{"a", "ambo", func(p Solid) Solid { return p.ambo() }},
	{"t", "truncate", func(p Solid) Solid { return p.truncate(1.0 / 3.0) }},
	{"k", "kis", func(p Solid) Solid { return p.kis(0.25) }},
	{"e", "expand", func(p Solid) Solid { return p.expand() }},
	{"b", "bevel", func(p Solid) Solid { return p.bevel() }},
}

// Build is seed index s with operator index o applied, projected
// to the unit sphere so the wireframe reads as the solid it names.
func Build(s, o int) Solid {
	if s < 0 || s >= len(Seeds) {
		s = 1 // the cube, Conway's usual example
	}
	if o < 0 || o >= len(Ops) {
		o = 0
	}
	return Ops[o].Apply(Seeds[s].Make().normalize()).normalize()
}

// Name is the notation for a seed and operator, read right to left:
// tC is a truncated cube.
func Name(s, o int) string {
	if s < 0 || s >= len(Seeds) {
		s = 1
	}
	if o < 0 || o >= len(Ops) {
		o = 0
	}
	return Ops[o].Letter + Seeds[s].Letter
}
