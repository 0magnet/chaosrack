package stlmodels

import (
	"fmt"
	"strings"

	"github.com/0magnet/chaosrack/pkg/meshstl"
	"github.com/0magnet/chaosrack/pkg/rackspec"
	"github.com/0magnet/chaosrack/pkg/segfont"
)

// Dimensioned rack models: the same panels and frames, carrying their own
// measurements as geometry.
//
// The numbers were only ever in pkg/rackspec and a README table, which means
// the model and its dimensions were two things you had to hold side by side.
// A dimensioned model is one thing: open it in any viewer or slicer and the
// panel says 35.06 wide, 128.5 tall, 162 deep, in the millimeters everything
// here is drawn in.
//
// The text is the app's own 16-segment stroke font, pkg/segfont — the one the
// Fourier Text mode draws its banner with. It is already a set of line
// segments, which is exactly what an annotation needs; meshstl has no font of
// its own and no opinion about units, so this file is where the two meet.

// dimTextStrokes turns a string into unit-square segments for meshstl. The
// cell is 2 wide by 3 tall in the font, normalized here to a cap height of 1,
// with a gap between characters.
func dimTextStrokes(s string) [][2]meshstl.V3 {
	const (
		cellW   = 2.0 / 3.0 // width of one cell, in cap heights
		spacing = 0.28      // gap between cells
	)
	var out [][2]meshstl.V3
	x := 0.0
	for _, r := range strings.ToUpper(s) {
		if r == ' ' {
			x += cellW + spacing
			continue
		}
		glyph, ok := segfont.Segments(r)
		if !ok {
			// A rune the font cannot draw is skipped rather than drawn as a
			// wrong glyph — a dimension that reads 3S.06 is worse than one
			// that reads 3.06 and is obviously incomplete.
			x += cellW + spacing
			continue
		}
		for _, e := range glyph {
			out = append(out, [2]meshstl.V3{
				{x + e[0]/3, e[1] / 3, 0},
				{x + e[2]/3, e[3] / 3, 0},
			})
		}
		x += cellW + spacing
	}
	return out
}

// dimLabel formats a measurement the way a drawing does.
func dimLabel(mm float64) string {
	if mm == float64(int(mm)) {
		return fmt.Sprintf("%d", int(mm))
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", mm), "0"), ".")
}

// dimensioned wraps a mesh with width/height/depth annotations read off its
// own bounding box, so the numbers cannot disagree with the model.
func dimensioned(m meshstl.Mesh, size float64) meshstl.Mesh {
	lo, hi := m.Bounds()
	span := hi.Sub(lo)
	if size <= 0 {
		size = 6
	}
	labels := [3][][2]meshstl.V3{
		dimTextStrokes(dimLabel(span[0])),
		dimTextStrokes(dimLabel(span[1])),
		dimTextStrokes(dimLabel(span[2])),
	}
	out := meshstl.Mesh{Tris: append([]meshstl.Tri(nil), m.Tris...)}
	out.Append(meshstl.BoxDimensions(lo, hi, labels, size, meshstl.RodRadius))
	return out
}

// dimensionedModule is a module panel with its measurements, plus the two
// figures that are not simply its bounding box: how many HP wide it is, and
// that the panel is 3U.
func dimensionedModule(hp, seg int) meshstl.Mesh {
	m := demoPanel(hp, seg)
	out := dimensioned(m, 6)

	lo, hi := m.Bounds()
	// "7HP" against the top edge, and "3U" beside the panel — the two facts a
	// bare measurement does not tell you, because they are what the numbers
	// MEAN. 35.06 mm is only interesting once you know it is 7 HP.
	out.Append(meshstl.Strokes(
		dimTextStrokes(fmt.Sprintf("%dHP", hp)),
		meshstl.V3{lo[0], hi[1] + 4, hi[2]},
		meshstl.V3{1, 0, 0}, meshstl.V3{0, 1, 0}, 6, meshstl.RodRadius))
	out.Append(meshstl.Strokes(
		dimTextStrokes("3U"),
		meshstl.V3{hi[0] + 4, lo[1], hi[2]},
		meshstl.V3{1, 0, 0}, meshstl.V3{0, 1, 0}, 6, meshstl.RodRadius))
	return out
}

// dimensionedRack is the filled frame with the three figures that define it:
// the 19-inch panel width, the 84 HP row inside it, and the 3U opening.
func dimensionedRack(seg int) meshstl.Mesh {
	m := filledRack(seg)
	lo, hi := m.Bounds()
	inset := (rackspec.PanelWidth19 - rackspec.RowWidth()) / 2

	out := meshstl.Mesh{Tris: append([]meshstl.Tri(nil), m.Tris...)}
	const size = 10
	// The full 19 inches, below everything.
	out.Append(meshstl.Dimension(
		meshstl.V3{lo[0], lo[1], hi[2]}, meshstl.V3{hi[0], lo[1], hi[2]},
		meshstl.V3{0, -1, 0}, 34, dimTextStrokes(dimLabel(rackspec.PanelWidth19)), size, meshstl.RodRadius))
	// The 84 HP row inside it, closer in — the distinction between the two is
	// the whole reason a 19-inch rack holds 84 HP and not 95.
	out.Append(meshstl.Dimension(
		meshstl.V3{lo[0] + inset, lo[1], hi[2]}, meshstl.V3{hi[0] - inset, lo[1], hi[2]},
		meshstl.V3{0, -1, 0}, 14,
		dimTextStrokes(fmt.Sprintf("%dHP", rackspec.RowHP)), size, meshstl.RodRadius))
	// The 3U panel height, up the left.
	out.Append(meshstl.Dimension(
		meshstl.V3{lo[0], lo[1] + meshstl.RailHeight, hi[2]},
		meshstl.V3{lo[0], lo[1] + meshstl.RailHeight + rackspec.PanelHeight3U, hi[2]},
		meshstl.V3{-1, 0, 0}, 14,
		dimTextStrokes(dimLabel(rackspec.PanelHeight3U)), size, meshstl.RodRadius))
	// The depth, along the side.
	out.Append(meshstl.Dimension(
		meshstl.V3{hi[0], lo[1], hi[2]}, meshstl.V3{hi[0], lo[1], lo[2]},
		meshstl.V3{0, -1, 0}, 14, dimTextStrokes(dimLabel(hi[2]-lo[2])), size, meshstl.RodRadius))
	return out
}
