//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/glctx"

	"github.com/0magnet/chaosrack/pkg/conway"
)

// Drawing a generated polyhedron.
//
// The five Platonic solids stay as the five models they always were — the
// seed IS the model — and each of them gains one knob: the Conway operator
// applied to it. Five models times seven operators reaches seventeen
// distinct solids, including eleven of the thirteen Archimedeans and
// several Catalan duals, from a row that had six fixed models and no knob
// at all. See pkg/conway for the operators and what each one does.

// polyOpF is the operator knob, shared by the five seeds: it is the same
// question on each of them, so it is the same control and keeps its
// position when the seed changes.
var polyOpF float32

// polyBuilt is what is currently on the GPU, as seed*100+op, so a turn of
// the operator knob rebuilds and nothing else does.
//
// Tracked here rather than through gpu.staticDirty because this mesh
// depends on a parameter that the generic invalidation does not know about,
// and a wireframe that ignores its own knob is worse than one that rebuilds
// a little too often.
var polyBuilt = -1

// generateSeed draws seed s with the operator knob applied.
func generateSeed(s int) {
	want := s*100 + int(polyOpF)
	if polyBuilt == want && gpu.staticGeomCached(glctx.Types.Line) {
		return
	}
	polyBuilt = want

	p := conway.Build(s, int(polyOpF))
	verts := make([]float32, 0, len(p.Verts)*3)
	for _, v := range p.Verts {
		verts = append(verts, float32(v.X), float32(v.Y), float32(v.Z))
	}
	edges := p.Edges()
	idx := make([]uint16, 0, len(edges)*2)
	for _, e := range edges {
		idx = append(idx, uint16(e[0]), uint16(e[1])) //nolint:gosec // a generated solid is far below 65535 vertices; the largest here is bD at 120
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
