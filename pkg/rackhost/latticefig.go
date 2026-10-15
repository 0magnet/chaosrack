package rackhost

import (
	"github.com/0magnet/lattice"

	"github.com/0magnet/chaosrack/pkg/attractor"
)

// Any model in the Lattice, natively: ROWS's L. The model runs as it always
// does, and what it would draw — a trail, a mesh of lines, a cloud of dots —
// is handed to the Lattice's programs as a figure, which they draw as they
// draw a solid, each its own slice, as the page hands on what its model
// uploads (latticefigure_js.go).

// latticeFig is the figure handed on, its scale, and when it was last.
type latticeFig struct {
	fig lattice.Figure
	ext float64
	at  float64
}

// latticeOf is the Lattice drawing fig, the model's figure, and its frame.
// Called with r.mu held.
func (r *Rack) latticeOf(fig attractor.Figure) (attractor.Figure, [][3]float64) {
	lf := &r.latFig
	nowMs := float64(r.sound.now().UnixNano()) / 1e6
	// As often as the programs draw: a figure handed on more often is
	// voxelized and never shown.
	if nowMs-lf.at >= 1000.0/attractor.LatticeFPS {
		lf.at = nowMs
		lf.fig.Set(lf.lines(fig))
	}
	return r.latticeFigure(&lf.fig)
}

// lines is fig as the lattice takes a figure: runs of points, scaled into its
// volume. The scale follows the model's size, out at once and back in slowly,
// so a trail that grows and shrinks does not breathe in and out of it.
func (lf *latticeFig) lines(fig attractor.Figure) [][]lattice.Vec3 {
	most := 0.0
	for _, p := range fig.Points {
		most = max(most, abs(p[0]), abs(p[1]), abs(p[2]))
	}
	if most > lf.ext || lf.ext == 0 {
		lf.ext = most
	} else {
		lf.ext = max(most, lf.ext*0.99)
	}
	s := 0.92 / max(lf.ext, 1e-6)
	pt := func(i int) lattice.Vec3 {
		p := fig.Points[i]
		return lattice.Vec3{p[0] * s, p[1] * s, p[2] * s}
	}
	var out [][]lattice.Vec3
	switch fig.Kind {
	case attractor.FigurePath:
		line := make([]lattice.Vec3, len(fig.Points))
		for i := range fig.Points {
			line[i] = pt(i)
		}
		out = append(out, line)
	case attractor.FigureLines:
		for k := 0; k+1 < len(fig.Edges); k += 2 {
			a, b := int(fig.Edges[k]), int(fig.Edges[k+1])
			if a < len(fig.Points) && b < len(fig.Points) {
				out = append(out, []lattice.Vec3{pt(a), pt(b)})
			}
		}
	default: // dots, or anything else: its vertices
		for i := range fig.Points {
			out = append(out, []lattice.Vec3{pt(i)})
		}
	}
	return out
}
