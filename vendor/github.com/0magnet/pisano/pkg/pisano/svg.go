package pisano

import (
	"fmt"
	"io"
	"math"
	"strings"
)

// SVGOptions controls the circular renderer.
type SVGOptions struct {
	Sheet
	Points bool // mark the m points around each circle
	Rings  bool // draw the circle the points sit on
}

// DefaultSVG is a readable contact sheet.
func DefaultSVG() SVGOptions {
	return SVGOptions{
		Sheet:  Sheet{Cell: 180, Labels: true},
		Points: true,
		Rings:  true,
	}
}

// Point returns where remainder k sits on a unit circle. Zero is at the top and
// the numbering runs clockwise, so a design reads the way a clock face does.
func Point(k, m int) (x, y float64) {
	if m < 1 {
		return 0, 0
	}
	t := 2 * math.Pi * float64(k) / float64(m)
	return math.Sin(t), -math.Cos(t)
}

// CircleTile renders one circular design into a tile.
func CircleTile(d Design, opt SVGOptions) Tile {
	size := opt.TileSize()
	r := size / 2
	cx, cy := r, r
	stroke := opt.Stroke()

	var b strings.Builder
	if opt.Rings {
		fmt.Fprintf(&b, `<circle class="ring" cx="%.2f" cy="%.2f" r="%.2f" stroke-width="%.2f"/>`+"\n",
			cx, cy, r, stroke*0.7)
	}
	if len(d.Chords) > 0 {
		fmt.Fprintf(&b, `<path class="chord" stroke-width="%.2f" d="`, stroke)
		for _, c := range d.Chords {
			ax, ay := Point(c.A, d.Modulus)
			bx, by := Point(c.B, d.Modulus)
			fmt.Fprintf(&b, "M%.2f %.2fL%.2f %.2f", cx+ax*r, cy+ay*r, cx+bx*r, cy+by*r)
		}
		fmt.Fprint(&b, `"/>`+"\n")
	}
	// Marking the points is only useful while you can still tell them apart;
	// past that they crowd into the ring and add nothing.
	if opt.Points && d.Modulus > 1 && d.Modulus <= 64 {
		dot := math.Max(0.9, r*0.022)
		for k := 0; k < d.Modulus; k++ {
			px, py := Point(k, d.Modulus)
			fmt.Fprintf(&b, `<circle class="pt" cx="%.2f" cy="%.2f" r="%.2f"/>`+"\n",
				cx+px*r, cy+py*r, dot)
		}
	}

	return Tile{Caption: CircleCaption(d), Body: b.String(), Size: size}
}

// CircleTiles renders a run of designs.
func CircleTiles(designs []Design, opt SVGOptions) []Tile {
	tiles := make([]Tile, 0, len(designs))
	for _, d := range designs {
		tiles = append(tiles, CircleTile(d, opt))
	}
	return tiles
}

// CircleCaption is the line under each figure: which modulus, how long the
// period is, how many zeros it holds and whether the figure is symmetrical —
// the four numbers you need to check the zero-count claim by eye. The zero
// count is suffixed rather than bare because "4" and "0 zeros" read the same
// way round when they sit next to each other.
//
// A sequence that never repeats gets none of it. Its figure is a prefix that
// was drawn, not a design, and captioning the scan limit as though it were a
// period would be a straightforward lie.
func CircleCaption(d Design) string {
	if !d.Bounded {
		return fmt.Sprintf("m=%d · no period", d.Modulus)
	}
	sym := ""
	if d.Symmetric() {
		sym = " · sym"
	}
	return fmt.Sprintf("m=%d · π=%d · %dz%s", d.Modulus, d.Period, d.Zeros, sym)
}

// WriteSVG renders designs as a grid of tiles in a single SVG document. One
// design gives a single figure; a range gives the contact sheet where the
// families actually become visible — the near-duplicate designs at Fibonacci
// moduli 8/21/55 and 13/34/89 are only obvious side by side.
func WriteSVG(w io.Writer, designs []Design, opt SVGOptions) error {
	return opt.Sheet.WriteSVG(w, CircleTiles(designs, opt))
}
