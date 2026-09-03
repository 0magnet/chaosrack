package pisano

import (
	"fmt"
	"io"
	"math"
	"strings"
)

// TurtleSVGOptions controls the vector turtle renderer.
//
// The terminal renderer in canvas.go and this one draw the same walk. The
// terminal one is bound to a character grid, so a horizontal unit has to span
// two cells and the line weight is whatever the font gives you; here the path
// is just a polyline, so it scales to whatever box it is given and the strokes
// are square units on both axes.
type TurtleSVGOptions struct {
	Sheet
	Reps     int      // passes to draw for paths that never close
	ByPass   bool     // color the path rather than drawing it in one stroke
	Tint     TintMode // what a color means
	Grid     bool     // faint lattice behind the path
	Rounded  bool     // round the corners rather than mitring them
	Weight   float64
	CloseGap bool // join the last point back to the first on a closed path
}

// DefaultTurtleSVG is a readable contact sheet.
func DefaultTurtleSVG() TurtleSVGOptions {
	return TurtleSVGOptions{
		Sheet:   Sheet{Cell: 180, Labels: true},
		Reps:    3,
		Rounded: true,
	}
}

// TurtleTile renders one turtle path into a tile, scaled to fill the box while
// keeping the lattice square — a path that drifts sideways is wide and short,
// and stretching it to fit would misreport its shape.
func TurtleTile(p Period, opt TurtleSVGOptions) (Tile, Shape) {
	shape := Classify(p.Terms)
	steps := Walk(p, shape.Passes(opt.Reps))
	pts, _ := PathOf(p, shape.Passes(opt.Reps))
	size := opt.TileSize()
	if len(pts) < 2 {
		return Tile{Caption: TurtleCaption(p, shape), Size: size}, shape
	}

	minX, minY := pts[0].X, pts[0].Y
	maxX, maxY := minX, minY
	for _, q := range pts {
		minX, maxX = min(minX, q.X), max(maxX, q.X)
		minY, maxY = min(minY, q.Y), max(maxY, q.Y)
	}
	spanX, spanY := float64(maxX-minX), float64(maxY-minY)

	weight := opt.Weight
	if weight <= 0 {
		weight = opt.Stroke() * 1.6
	}
	// Leave room for the stroke so a line on the edge is not clipped in half.
	inset := weight
	usable := size - 2*inset
	if usable < 1 {
		usable = 1
	}
	scale := math.Inf(1)
	if spanX > 0 {
		scale = usable / spanX
	}
	if spanY > 0 {
		scale = math.Min(scale, usable/spanY)
	}
	if math.IsInf(scale, 1) {
		scale = usable // a path with no extent at all
	}

	// Center whatever the path's true aspect turns out to be.
	offX := inset + (usable-spanX*scale)/2 - float64(minX)*scale
	offY := inset + (usable-spanY*scale)/2 - float64(minY)*scale
	at := func(q Pt) (float64, float64) {
		return offX + float64(q.X)*scale, offY + float64(q.Y)*scale
	}

	var b strings.Builder

	if opt.Grid && scale > 6 {
		fmt.Fprintf(&b, `<g class="ring" stroke-width="%.2f">`, weight*0.18)
		for x := minX; x <= maxX; x++ {
			x0, y0 := at(Pt{x, minY})
			_, y1 := at(Pt{x, maxY})
			fmt.Fprintf(&b, `<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`, x0, y0, x0, y1)
		}
		for y := minY; y <= maxY; y++ {
			x0, y0 := at(Pt{minX, y})
			x1, _ := at(Pt{maxX, y})
			fmt.Fprintf(&b, `<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`, x0, y0, x1, y0)
		}
		fmt.Fprint(&b, "</g>\n")
	}

	join := "miter"
	if opt.Rounded {
		join = "round"
	}

	// One subpath per pass when tinting, otherwise the whole walk in one.
	// Passes share endpoints, so each run restarts at the point the previous
	// one ended and the joins stay closed.
	write := func(class string, from, to int) {
		if to <= from {
			return
		}
		fmt.Fprintf(&b, `<path class="path %s" stroke-width="%.2f" stroke-linejoin="%s" d="`,
			class, weight, join)
		x, y := at(pts[from])
		fmt.Fprintf(&b, "M%.2f %.2f", x, y)
		for i := from + 1; i <= to; i++ {
			x, y = at(pts[i])
			fmt.Fprintf(&b, "L%.2f %.2f", x, y)
		}
		fmt.Fprint(&b, `"/>`+"\n")
	}

	if !opt.ByPass {
		write("", 0, len(pts)-1)
	} else {
		// One subpath per run of steps sharing a color. The same tinter the
		// terminal uses decides what that color is, so --tint means the same
		// thing wherever the figure is drawn.
		circuits := 1
		if !shape.Closed {
			circuits = shape.Passes(opt.Reps)
		}
		tinter := NewTinter(opt.Tint, 6, len(steps)/circuits)
		tint := make([]int, len(steps))
		for i, st := range steps {
			tint[i] = tinter.Tint(st)
		}
		start := 0
		for i := range tint {
			if i+1 < len(tint) && tint[i+1] == tint[i] {
				continue
			}
			cls := ""
			if tint[i] >= 0 {
				cls = fmt.Sprintf("p%d", tint[i])
			}
			write(cls, start, i+1)
			start = i + 1
		}
	}

	return Tile{Caption: TurtleCaption(p, shape), Body: b.String(), Size: size}, shape
}

// TurtleTiles renders a run of periods.
func TurtleTiles(ps []Period, opt TurtleSVGOptions) []Tile {
	tiles := make([]Tile, 0, len(ps))
	for _, p := range ps {
		t, _ := TurtleTile(p, opt)
		tiles = append(tiles, t)
	}
	return tiles
}

// TurtleCaption names the figure and says whether the path closes, which is the
// one thing you cannot read off the picture when it is small. Unreduced paths
// are captioned by their sequence instead of a modulus, since a page of them is
// one figure per sequence and "unreduced" four times over says nothing.
func TurtleCaption(p Period, s Shape) string {
	what := fmt.Sprintf("m=%d", p.Modulus)
	if p.Modulus == 0 {
		what = p.Seq
		if i := strings.Index(what, " ("); i > 0 {
			what = what[:i] // drop the "(unreduced)" qualifier
		}
	}
	state := "open"
	if s.Closed {
		state = "closed"
	}
	return fmt.Sprintf("%s · %d · %s", what, p.Len(), state)
}

// WriteTurtleSVG renders turtle paths as a grid of tiles in one SVG document.
func WriteTurtleSVG(w io.Writer, ps []Period, opt TurtleSVGOptions) error {
	return opt.Sheet.WriteSVG(w, TurtleTiles(ps, opt))
}
