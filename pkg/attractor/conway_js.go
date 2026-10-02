//go:build js && wasm

package attractor

import (
	"strconv"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"

	"github.com/0magnet/chaosrack/pkg/conway"
)

// Drawing the Polyhedron model.
//
// The five Platonic solids were five models, each with one knob. They are
// one model now, because they are one family with two numbers between them:
// the Schläfli symbol {p,q}, a face of p sides with q meeting at every
// corner. SIDES turns the tetrahedron's triangles into the cube's squares and
// the dodecahedron's pentagons; MEET turns the tetrahedron into the
// octahedron and the icosahedron. Where the pair is not a solid the model
// does not stop: it draws the tiling the pair is, flat on the plane or
// hyperbolic in Poincaré's disk (see pkg/conway, regular.go).
//
// On a solid, three more: MORPH cuts the corners off, continuously, through
// the rectified solid to the dual (the cube through the cuboctahedron to the
// octahedron), OP is a Conway operator on the result, and KIS raises a
// pyramid on every face, or presses one in.

// poly is the model's knobs. polyOpF is the operator knob, kept by the name
// the five seeds shared so an old link's operator still lands on it.
var poly = struct {
	pF, qF, morphF, kisF float32
}{pF: 3, qF: 3}

var polyOpF float32

// polyBuilt is the knob settings the mesh on the GPU was built from, so a
// turn rebuilds it and nothing else does. Tracked here rather than through
// gpu.staticDirty because the generic invalidation does not know these knobs.
var polyBuilt [5]float32

// polySolid is the solid (or tiling) the knobs describe, and whether it is a
// solid: morph, op and kis act only on a solid.
func polySolid() (conway.Solid, bool) {
	p, q := int(poly.pF+0.5), int(poly.qF+0.5)
	s := conway.Regular(p, q)
	if conway.CurvatureOf(p, q) != conway.Spherical {
		return s, false
	}
	s = conway.Morph(s, float64(poly.morphF)).Apply(int(polyOpF + 0.5))
	if poly.kisF != 0 {
		s = s.Kis(float64(poly.kisF))
	}
	return s, true
}

// generatePolyhedron draws the knobs' solid as a wireframe.
func generatePolyhedron() {
	want := [5]float32{poly.pF, poly.qF, poly.morphF, poly.kisF, polyOpF}
	if polyBuilt == want && gpu.staticGeomCached(glctx.Types.Line) {
		return
	}
	polyBuilt = want
	refreshPolyEquation() // the Equation module says what is being drawn
	s, _ := polySolid()
	verts := make([]float32, 0, len(s.Verts)*3)
	for _, v := range s.Verts {
		verts = append(verts, float32(v.X), float32(v.Y), float32(v.Z))
	}
	edges := s.Edges()
	idx := make([]uint16, 0, len(edges)*2)
	for _, e := range edges {
		idx = append(idx, uint16(e[0]), uint16(e[1])) //nolint:gosec // bounded by pkg/conway's patch cap, far under 65535
	}
	gpu.uploadBuffersIndexed(verts, idx, glctx.Types.Line)
}

// polyOpNames is the operator knob's positions, for the labeled rotary.
func polyOpNames() []string {
	out := make([]string, len(conway.Ops))
	for i, o := range conway.Ops {
		out[i] = o.Name
	}
	return out
}

// polyEquationLines are the Equation module's four lines for the Polyhedron,
// as label and text, and whether the second is an equation Custom can run.
//
// The first says which geometry {p,q} is in, by the test that decides it:
// the corners of q faces with p sides make less than a full turn (a solid),
// exactly one (flat) or more (hyperbolic) as 1/p + 1/q is above, at or below
// a half. The second is the solid as F(x,y,z) = 0 (conway.Implicit), the
// third its counts, the fourth what has been done to it.
func polyEquationLines() (lines [4][2]string, runnable bool) {
	p, q := int(poly.pF+0.5), int(poly.qF+0.5)
	sum := "1/" + strconv.Itoa(p) + " + 1/" + strconv.Itoa(q)
	s, solid := polySolid()
	switch conway.CurvatureOf(p, q) {
	case conway.Spherical:
		lines[0] = [2]string{"{p,q}", "{" + strconv.Itoa(p) + "," + strconv.Itoa(q) + "}: " + sum + " > 1/2, a solid"}
	case conway.Flat:
		lines[0] = [2]string{"{p,q}", "{" + strconv.Itoa(p) + "," + strconv.Itoa(q) + "}: " + sum + " = 1/2, flat: a tiling"}
	default:
		lines[0] = [2]string{"{p,q}", "{" + strconv.Itoa(p) + "," + strconv.Itoa(q) + "}: " + sum + " < 1/2, hyperbolic"}
	}
	lines[1] = [2]string{"F =", "a tiling is not the surface of a solid"}
	if solid {
		if eq, ok := conway.Implicit(s); ok {
			lines[1][1], runnable = eq, true
		} else {
			lines[1][1] = "not convex: no single max of planes"
		}
	}
	lines[2] = [2]string{"V E F", strconv.Itoa(len(s.Verts)) + "  " + strconv.Itoa(len(s.Edges())) + "  " + strconv.Itoa(len(s.Faces))}
	lines[3] = [2]string{"ops", "morph " + strconv.FormatFloat(float64(poly.morphF), 'f', 2, 64) +
		" · op " + polyOpNames()[max(0, min(len(polyOpNames())-1, int(polyOpF+0.5)))] +
		" · kis " + strconv.FormatFloat(float64(poly.kisF), 'f', 2, 64)}
	return lines, runnable
}

// refreshPolyEquation rewrites the Equation module's lines when the solid
// changes, if the module is showing the Polyhedron's.
func refreshPolyEquation() {
	lines, runnable := polyEquationLines()
	for i, l := range lines {
		inp := dom.Doc.Call("getElementById", "eqv-"+strconv.Itoa(i))
		if !inp.Truthy() || inp.Call("getAttribute", "data-model").String() != "polyhedron" {
			return
		}
		if inp.Get("value").String() != l[1] {
			inp.Set("value", l[1])
		}
		if i == 1 {
			inp.Set("disabled", !runnable)
		}
	}
}
