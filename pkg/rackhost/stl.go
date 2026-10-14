package rackhost

import (
	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/stlmodels"
)

// The STL File model, natively: a built-in solid the picker names, or a
// file loaded, drawn as its triangles' edges (attractor.STLWire), and the
// plain cube before either.

// stlView is what the model shows: the wireframe, what it is of, and the
// built-in it was built from.
type stlView struct {
	wire  attractor.STLWire
	label string
	built string // the built-in the wireframe is of, or "" for a file
}

// stlFigure is the STL File model's figure: the built-in the picker names,
// built when it changes, or the file loaded, or the stand-in cube. Called
// with r.mu held.
func (r *Rack) stlFigure() (attractor.Figure, error) {
	sv := &r.stl
	if name := r.value("stlfile-builtin"); name != "" && name != sv.built {
		m, ok := attractor.STLModelByName(name)
		if !ok {
			return attractor.STLStandIn(), nil
		}
		w, err := attractor.STLMeshWire(m.Build(stlmodels.ViewerSeg))
		if err != nil {
			return attractor.Figure{}, err
		}
		sv.wire, sv.label, sv.built = w, name, name
	}
	if len(sv.wire.Verts) == 0 {
		return attractor.STLStandIn(), nil
	}
	return attractor.STLWireFigure(sv.wire), nil
}
