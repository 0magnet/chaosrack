//go:build js && wasm

package attractor

import (
	"strconv"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"github.com/0magnet/chaosrack/pkg/sacred"
)

// Drawing the Sacred Geometry model (pkg/sacred): FORM picks a figure, MORPH
// draws its construction (1 is the figure as it is usually shown, past it a
// vortex) and DETAIL is how much there is.

var sacredK = struct {
	formF, morphF, detailF float32
}{formF: float32(SacredDefaults[0]), morphF: float32(SacredDefaults[1]), detailF: float32(SacredDefaults[2])}

// sacredBuilt is the knob settings the mesh on the GPU was built from.
var sacredBuilt [3]float32

func sacredKnobs() (form int, morph, detail float64) {
	return int(sacredK.formF + 0.5), float64(sacredK.morphF), float64(sacredK.detailF)
}

// generateSacred draws the knobs' figure as a wireframe.
func generateSacred() {
	want := [3]float32{sacredK.formF, sacredK.morphF, sacredK.detailF}
	if sacredBuilt == want && gpu.staticGeomCached(glctx.Types.Line) {
		return
	}
	sacredBuilt = want
	refreshSacredEquation()
	g := sacred.Form(sacredKnobs()).Geom()
	gpu.uploadBuffersIndexed(g.Vertices, g.Indices, glctx.Types.Line)
}

// refreshSacredEquation rewrites the Equation module's lines when the figure
// changes, if the module is showing the Sacred Geometry's.
func refreshSacredEquation() {
	lines := sacred.EquationLines(sacredKnobs())
	for i, l := range lines {
		inp := dom.Doc.Call("getElementById", "eqv-"+strconv.Itoa(i))
		if !inp.Truthy() || inp.Call("getAttribute", "data-model").String() != "sacred" {
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
