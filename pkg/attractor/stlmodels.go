package attractor

import (
	"bytes"
	"fmt"

	"github.com/0magnet/chaosrack/pkg/meshstl"
	"github.com/0magnet/chaosrack/pkg/rackspec"
)

// The built-in solid models: the rack as a thing you could hold, the geometry
// as closed solids rather than wireframes, and every registered flow swept as
// a tube along its own trajectory.
//
// They live here, next to the flow registry, because the attractor models are
// integrated from it — and because putting them in meshstl would be a cycle:
// this package imports meshstl to build them, and the STL mode renders them.
// The same list backs `cmd/stlgen`, so what is written to a file and what the
// app offers in its model picker cannot be different sets.

// STLModel is one built-in solid.
type STLModel struct {
	Name        string // file name stem, and the picker's key
	Label       string // what the picker shows
	Group       string // Rack / Geometry / Attractors
	Description string // one line, also the STL header
	build       func(seg int) meshstl.Mesh
}

// Build returns the mesh. seg overrides the segments-per-revolution when
// positive — more for a printed model, fewer for one being rotated live.
func (m STLModel) Build(seg int) meshstl.Mesh { return m.build(seg) }

// Bytes builds the model and encodes it as a binary STL, which is the form
// the viewer's loader takes. It is how the app shows a built-in without
// fetching anything.
func (m STLModel) Bytes(seg int) ([]byte, error) {
	var buf bytes.Buffer
	if err := meshstl.WriteBinarySTL(&buf, m.Build(seg), m.Description); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func segOr(seg, def int) int {
	if seg > 0 {
		return seg
	}
	return def
}

// segRound is segOr with a FLOOR: never fewer segments than def.
//
// STLViewerSeg exists to keep a swept attractor tube inside the viewer's
// triangle budget, and passing it to every model made the rack's knobs
// hexagons — a module is about 1200 triangles, so nothing about it needs
// coarsening, and a six-sided knob is just wrong. Tubes still honor a low
// count through segOr; parts do not.
func segRound(seg, def int) int {
	if seg > def {
		return seg
	}
	return def
}

// STLModels returns every built-in, rack first, then geometry, then one per
// registered flow in catalog order.
func STLModels() []STLModel {
	out := []STLModel{
		{
			Name: "module-1slot", Label: "Module, 1 slot", Group: "Rack",
			Description: fmt.Sprintf("%d HP 3U module panel with body", rackspec.ModuleHP),
			build: func(seg int) meshstl.Mesh {
				return demoPanel(rackspec.ModuleHP, segRound(seg, 24))
			},
		},
		{
			Name: "module-2slot", Label: "Module, 2 slots", Group: "Rack",
			Description: fmt.Sprintf("%d HP 3U module panel with body", 2*rackspec.ModuleHP),
			build: func(seg int) meshstl.Mesh {
				return demoPanel(2*rackspec.ModuleHP, segRound(seg, 24))
			},
		},
		{
			Name: "module-3slot", Label: "Module, 3 slots", Group: "Rack",
			Description: fmt.Sprintf("%d HP 3U module panel with body", 3*rackspec.ModuleHP),
			build: func(seg int) meshstl.Mesh {
				return demoPanel(3*rackspec.ModuleHP, segRound(seg, 24))
			},
		},
		{
			Name: "module-blank", Label: "Blank panel", Group: "Rack",
			Description: "1 slot, no controls — the panel alone",
			build: func(int) meshstl.Mesh {
				return meshstl.ModulePanel(meshstl.PanelOptions{Body: true})
			},
		},
		{
			Name: "rack-frame", Label: "19-inch frame", Group: "Rack",
			Description: "single 3U bay of a 19-inch frame",
			build:       func(int) meshstl.Mesh { return meshstl.Rack(1) },
		},
		{
			Name: "rack-filled", Label: "Filled rack row", Group: "Rack",
			Description: "a 3U bay with its row of modules in it",
			build:       func(seg int) meshstl.Mesh { return filledRack(segRound(seg, 20)) },
		},
		// The same two, carrying their measurements as geometry. Optional
		// because an annotated model is a drawing rather than a part: you
		// want it to read the rack, not to print it.
		{
			Name: "module-dimensioned", Label: "Module, dimensioned", Group: "Rack",
			Description: "a 1-slot module with its width, height, depth, HP and U marked",
			build: func(seg int) meshstl.Mesh {
				return dimensionedModule(rackspec.ModuleHP, segRound(seg, 24))
			},
		},
		{
			Name: "rack-dimensioned", Label: "Rack, dimensioned", Group: "Rack",
			Description: "the filled bay with the 19-inch width, the 84 HP row and the 3U height marked",
			build:       func(seg int) meshstl.Mesh { return dimensionedRack(segRound(seg, 16)) },
		},
	}

	// Geometry, matching the models the app draws as wireframes.
	geom := []struct {
		name, label, desc string
		build             func(seg int) meshstl.Mesh
	}{
		{"tetrahedron", "Tetrahedron", "Platonic solid, 4 faces",
			func(int) meshstl.Mesh { return meshstl.Tetrahedron(30) }},
		{"cube", "Cube", "Platonic solid, 6 faces",
			func(int) meshstl.Mesh { return meshstl.Hexahedron(30) }},
		{"octahedron", "Octahedron", "Platonic solid, 8 faces",
			func(int) meshstl.Mesh { return meshstl.Octahedron(30) }},
		{"dodecahedron", "Dodecahedron", "Platonic solid, 12 faces",
			func(int) meshstl.Mesh { return meshstl.Dodecahedron(30) }},
		{"icosahedron", "Icosahedron", "Platonic solid, 20 faces",
			func(int) meshstl.Mesh { return meshstl.Icosahedron(30) }},
		{"nestedcube", "Nested cube", "a cube inside a cube, joined at the corners",
			func(int) meshstl.Mesh { return meshstl.NestedCube(30, 0.5, 1.2) }},
		{"sphere", "Sphere", "UV sphere",
			func(seg int) meshstl.Mesh {
				s := segRound(seg, 48)
				return meshstl.UVSphere(meshstl.V3{}, 30, s/2, s)
			}},
		{"torus", "Torus", "R 30, r 10",
			func(seg int) meshstl.Mesh {
				s := segRound(seg, 64)
				return meshstl.Torus(meshstl.V3{}, 30, 10, s, s/2)
			}},
	}
	for _, g := range geom {
		out = append(out, STLModel{
			Name: g.name, Label: g.label, Group: "Geometry",
			Description: g.desc, build: g.build,
		})
	}

	// One per registered flow, swept as a tube along its own trajectory.
	for _, key := range FlowKeys() {
		k := key
		info := modeInfo[k]
		label := info.Label
		if label == "" {
			label = k
		}
		out = append(out, STLModel{
			Name: k, Label: label, Group: "Attractors",
			Description: label + " — trajectory swept as a tube",
			build:       func(seg int) meshstl.Mesh { return flowTube(k, segOr(seg, 8)) },
		})
	}
	return out
}

// STLModel lookup by name.
func STLModelByName(name string) (STLModel, bool) {
	for _, m := range STLModels() {
		if m.Name == name {
			return m, true
		}
	}
	return STLModel{}, false
}

// stlFileMaxTris is the viewer's triangle budget: three vertices per
// triangle and 16-bit indices cap the pipeline at 65535 vertices, so larger
// meshes are decimated by triangle stride to fit. Untagged, because the model
// generators below size themselves against it and they must build on the host
// too.
const stlFileMaxTris = 21845

// STLViewerSeg is the segment count to build a model at for the app's own STL
// mode rather than for a file.
//
// It matters because the viewer indexes with 16 bits and decimates anything
// over stlFileMaxTris by dropping every Nth TRIANGLE — which on a swept tube
// does not simplify the surface, it punches holes in it. A model built at
// this detail comes in under the budget and is shown whole. A file gets the
// finer default, since a slicer has no such limit.
const STLViewerSeg = 6

// flowTube integrates a flow and sweeps a tube along it, normalized to a
// printable size. The tube radius is a fraction of the figure so that a large
// attractor and a small one both come out with walls you could print.
//
// The path length follows the segment count: the triangle budget is points ×
// seg × 2, so a coarse tube is allowed proportionally more of the trajectory
// and both ends of the range land near the same total.
func flowTube(mode string, seg int) meshstl.Mesh {
	o := DefaultTrajectory()
	if seg <= STLViewerSeg {
		o.MaxPoints = stlFileMaxTris / (seg * 2)
	}
	path := Trajectory(mode, o)
	if len(path) < 2 {
		return meshstl.Mesh{}
	}
	pts := make([]meshstl.V3, len(path))
	for i, p := range path {
		pts[i] = meshstl.V3{p[0], p[1], p[2]}
	}
	// Normalize first, then sweep: the systems live at wildly different
	// scales (Thomas fits in a box of 5, Halvorsen in one of 60), and a fixed
	// tube radius would be a thread on one and a sausage on the other.
	box := pathBounds(pts)
	biggest := box[0]
	for _, v := range box {
		if v > biggest {
			biggest = v
		}
	}
	if biggest <= 0 {
		return meshstl.Mesh{}
	}
	const targetSize = 60.0 // mm across, so a model prints at a sane size
	s := targetSize / biggest
	for i := range pts {
		pts[i] = pts[i].Mul(s)
	}
	return meshstl.Tube(pts, targetSize*0.012, seg, true).CenterXY()
}

func pathBounds(p []meshstl.V3) meshstl.V3 {
	min, max := p[0], p[0]
	for _, v := range p {
		for i := 0; i < 3; i++ {
			if v[i] < min[i] {
				min[i] = v[i]
			}
			if v[i] > max[i] {
				max[i] = v[i]
			}
		}
	}
	return max.Sub(min)
}

// demoPanel is a module with a plausible set of controls on it: a column of
// knobs down the middle, a pin matrix, and a row of buttons — enough that the
// printed panel reads as a module rather than a rectangle.
func demoPanel(hp, seg int) meshstl.Mesh {
	w := float64(hp)*rackspec.HP - rackspec.Seam
	h := rackspec.PanelHeight3U

	var ctl []meshstl.Control
	// Knobs on the same 38 mm row pitch the screen panel uses.
	const rowPitch = 38.0
	for i := 0; i < 3; i++ {
		ctl = append(ctl, meshstl.Control{
			X: w / 2, Y: h - 20 - float64(i)*rowPitch,
			Kind: meshstl.KnobControl, Diam: rackspec.KnobLarge,
		})
	}
	if hp >= 2*rackspec.ModuleHP {
		// A wider panel gets the patch matrix and a button row beside them.
		ctl = append(ctl, meshstl.PinMatrix(w*0.75, h*0.55, 6, 6)...)
		for i := 0; i < 4; i++ {
			ctl = append(ctl, meshstl.Control{
				X: w*0.75 - 12 + float64(i)*8, Y: 14,
				Kind: meshstl.ButtonControl,
			})
		}
	}
	ctl = append(ctl, meshstl.Control{X: 6, Y: h - 6, Kind: meshstl.LEDControl})
	return meshstl.ModulePanel(meshstl.PanelOptions{
		HP: hp, Controls: ctl, Body: true, Seg: seg,
	})
}

// filledRack puts a row of modules into a frame — the picture of what 84 HP
// means, as a solid.
func filledRack(seg int) meshstl.Mesh {
	m := meshstl.Rack(1)
	inset := (rackspec.PanelWidth19 - rackspec.RowWidth()) / 2
	for i := 0; i < rackspec.SlotsPerRow(); i++ {
		panel := demoPanel(rackspec.ModuleHP, seg)
		m.Append(panel.Translate(meshstl.V3{
			inset + float64(i)*rackspec.SlotPitch, meshstl.RailHeight, 0,
		}))
	}
	return m
}
