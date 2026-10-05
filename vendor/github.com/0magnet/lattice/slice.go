package lattice

import "math"

// Cell is one character of a sheet: a glyph and its color, or nothing (Ch
// 0), which a terminal shows as its background and a stack of sheets shows
// as clear.
type Cell struct {
	Ch      rune
	R, G, B uint8
}

// Frame is a sheet's screen, row by row.
type Frame struct {
	Cols, Rows int
	Cells      []Cell
}

// At is the cell at column c, row r.
func (f *Frame) At(c, r int) Cell { return f.Cells[r*f.Cols+c] }

// Style is how a surface is written in characters.
type Style int

// The styles.
const (
	// Lines draws the line the surface makes across the sheet, in box
	// drawing: ─ │ ╱ ╲, and · where it lies along the sheet instead.
	Lines Style = iota
	// ASCII is Lines in - | / \ and . for terminals without box drawing.
	ASCII
	// Shade writes how squarely the surface crosses the sheet, .:-=+*# from
	// lying in it to standing across it.
	Shade
	// Solid fills every lit voxel.
	Solid
)

// Styles are the styles by name.
var Styles = map[string]Style{"lines": Lines, "ascii": ASCII, "shade": Shade, "solid": Solid}

// Coloring is how a voxel is colored.
type Coloring int

// The colorings.
const (
	// ByPosition colors a voxel by where it is in the volume, so it is the
	// same color in all three sheets that cross it.
	ByPosition Coloring = iota
	// Mono is one green, a terminal's own.
	Mono
)

// Lit reports whether the surface of s passes through the voxel centered at
// p, n to a side: whether the distance changes sign between its corners.
// Decided per voxel and not per sheet, so the three sheets through a voxel
// agree about it.
func Lit(s Shape, p Vec3, n int) bool {
	h := 1 / float64(n) // half a voxel
	lo, hi := math.Inf(1), math.Inf(-1)
	for i := range 8 {
		c := Vec3{p[0] + h*sign(i&1), p[1] + h*sign(i&2), p[2] + h*sign(i&4)}
		d := s(c)
		lo, hi = math.Min(lo, d), math.Max(hi, d)
	}
	return lo <= 0 && hi >= 0
}

func sign(bit int) float64 {
	if bit != 0 {
		return 1
	}
	return -1
}

// Render draws sheet sh of shape s.
func Render(sh Sheet, s Shape, st Style, co Coloring) Frame {
	f := Frame{Cols: sh.Cols(), Rows: sh.Rows()}
	f.Cells = make([]Cell, f.Cols*f.Rows)
	b := BasisOf(sh.Axis)
	for v := range sh.N {
		for u := range sh.N {
			p := sh.Voxel(u, v)
			if !Lit(s, p, sh.N) {
				continue
			}
			ch := glyph(gradient(s, p, 0.5/float64(sh.N)), b, st)
			r, g, bl := color(p, co)
			c := Cell{Ch: ch, R: r, G: g, B: bl}
			f.Cells[v*f.Cols+2*u] = c
			f.Cells[v*f.Cols+2*u+1] = c
		}
	}
	return f
}

// glyph is the character for a surface with gradient g crossing a sheet of
// basis b.
func glyph(g Vec3, b Basis, st Style) rune {
	if st == Solid {
		return '█'
	}
	gl := g.Len()
	if gl == 0 {
		return '·'
	}
	// How squarely the surface stands across the sheet: 0 lying in it
	// (its normal along the sheet's), 1 standing straight across.
	across := math.Sqrt(math.Max(0, 1-math.Pow(g.Dot(b.Normal)/gl, 2)))
	if st == Shade {
		const ramp = ".:-=+*#"
		return rune(ramp[int(math.Round(across*float64(len(ramp)-1)))])
	}
	lines := [5]rune{'·', '─', '╱', '│', '╲'}
	if st == ASCII {
		lines = [5]rune{'.', '-', '/', '|', '\\'}
	}
	if across < 0.4 {
		return lines[0]
	}
	// The line runs perpendicular to the gradient's part in the sheet.
	gu, gv := g.Dot(b.Right), g.Dot(b.Up)
	a := math.Atan2(gu, -gv) // the line's direction, up the screen positive
	a = math.Mod(a+2*math.Pi, math.Pi)
	switch k := int(math.Round(a/(math.Pi/4))) % 4; k {
	case 0:
		return lines[1]
	case 1:
		return lines[2]
	case 2:
		return lines[3]
	default:
		return lines[4]
	}
}

// color is a voxel's color.
func color(p Vec3, co Coloring) (r, g, b uint8) {
	if co == Mono {
		return 0x33, 0xff, 0x66
	}
	c := func(x float64) uint8 { return uint8(math.Round(80 + 175*(x+1)/2)) }
	return c(p[0]), c(p[1]), c(p[2])
}
