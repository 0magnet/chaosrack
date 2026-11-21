//go:build js && wasm

package attractor

import (
	"strconv"

	"github.com/0magnet/chaosrack/pkg/chester"
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
)

// Drawing the Chestahedron model: Frank Chester's seven-faced solid and what
// he does with it (pkg/chester). FORM picks a demonstration, MORPH runs it
// (1 is the form as he shows it) and DETAIL adds what lies inside or around.

var chest = struct {
	formF, morphF, detailF float32
}{morphF: 1, detailF: 1}

// chestBuilt is the knob settings the mesh on the GPU was built from.
var chestBuilt [3]float32

func chestKnobs() (form int, morph, detail float64) {
	return int(chest.formF + 0.5), float64(chest.morphF), float64(chest.detailF)
}

// generateChestahedron draws the knobs' form as a wireframe.
func generateChestahedron() {
	want := [3]float32{chest.formF, chest.morphF, chest.detailF}
	if chestBuilt == want && gpu.staticGeomCached(glctx.Types.Line) {
		return
	}
	chestBuilt = want
	refreshChestEquation()
	g := chester.Form(chestKnobs()).Geom()
	gpu.uploadBuffersIndexed(g.Vertices, g.Indices, glctx.Types.Line)
}

// chestahedronSkinMesh wraps the spectrogram onto the solid the form is
// built on.
func chestahedronSkinMesh() ([]float32, []uint16) {
	form, morph, _ := chestKnobs()
	p := chester.Faces(form, morph)
	verts := make([]float32, 0, len(p.V)*3)
	for _, v := range p.V {
		verts = append(verts, float32(v[0]), float32(v[1]), float32(v[2]))
	}
	return faceSkinMesh(verts, p.F, false)
}

// refreshChestEquation rewrites the Equation module's lines when the form
// changes, if the module is showing the Chestahedron's.
func refreshChestEquation() {
	lines := chester.Info(chestKnobs())
	for i, l := range lines {
		inp := dom.Doc.Call("getElementById", "eqv-"+strconv.Itoa(i))
		if !inp.Truthy() || inp.Call("getAttribute", "data-model").String() != "chestahedron" {
			return
		}
		if lbl := inp.Get("previousSibling"); lbl.Truthy() && lbl.Get("textContent").String() != l[0] {
			lbl.Set("textContent", l[0])
		}
		if inp.Get("value").String() != l[1] {
			inp.Set("value", l[1])
		}
	}
}
