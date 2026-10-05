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
// agree about it — exactly, because a corner's coordinates come from its
// index alone (edge), the same arithmetic whichever sheet asks.
func Lit(s Shape, p Vec3, n int) bool {
	var i [3]int
	for a := range 3 {
		i[a] = max(0, min(n-1, int(math.Floor((p[a]+1)/2*float64(n)))))
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for c := range 8 {
		d := s(Vec3{edge(i[0]+c&1, n), edge(i[1]+c>>1&1, n), edge(i[2]+c>>2&1, n)})
		lo, hi = math.Min(lo, d), math.Max(hi, d)
	}
	return lo <= 0 && hi >= 0
}

// edge is the coordinate of the i-th of the n+1 planes between voxels.
func edge(i, n int) float64 { return -1 + 2*float64(i)/float64(n) }

// slab is a sheet's voxel corners, the distance at each evaluated once: a
// voxel's eight corners are shared with its neighbors', so finding every lit
// voxel of a sheet corner by corner would evaluate each corner up to eight
// times over.
type slab struct {
	n, k   int // the sheet's size, and its slice: its corners are at k and k+1 along the normal
	na     int // the normal's axis
	o1, o2 int // the other two, in order
	d      []float64
}

// newSlab evaluates s at every corner of sheet sh.
func newSlab(s Shape, sh Sheet) *slab {
	na := [...]int{X: 0, Y: 1, Z: 2}[sh.Axis]
	o := [3][2]int{{1, 2}, {0, 2}, {0, 1}}[na]
	m := sh.N + 1
	sl := &slab{n: sh.N, k: sh.Slice, na: na, o1: o[0], o2: o[1], d: make([]float64, 2*m*m)}
	for l := range 2 {
		for a := range m {
			for b := range m {
				var c [3]int
				c[na], c[sl.o1], c[sl.o2] = sh.Slice+l, a, b
				sl.d[(l*m+a)*m+b] = s(Vec3{edge(c[0], sh.N), edge(c[1], sh.N), edge(c[2], sh.N)})
			}
		}
	}
	return sl
}

// lit is Lit for the voxel at world indices i, from the slab's corners.
func (sl *slab) lit(i [3]int) bool {
	m := sl.n + 1
	lo, hi := math.Inf(1), math.Inf(-1)
	for c := range 8 {
		j := [3]int{i[0] + c&1, i[1] + c>>1&1, i[2] + c>>2&1}
		d := sl.d[((j[sl.na]-sl.k)*m+j[sl.o1])*m+j[sl.o2]]
		lo, hi = math.Min(lo, d), math.Max(hi, d)
	}
	return lo <= 0 && hi >= 0
}

// index is the voxel at column u, row v of sheet sh, as indices along x, y
// and z: the inverse of Voxel's placing, in whole voxels.
func (sh Sheet) index(u, v int) [3]int {
	n := sh.N
	switch sh.Axis {
	case X:
		return [3]int{sh.Slice, n - 1 - v, n - 1 - u}
	case Y:
		return [3]int{u, sh.Slice, v}
	}
	return [3]int{u, n - 1 - v, sh.Slice}
}

// Render draws sheet sh of shape s.
func Render(sh Sheet, s Shape, st Style, co Coloring) Frame {
	f := Frame{Cols: sh.Cols(), Rows: sh.Rows()}
	f.Cells = make([]Cell, f.Cols*f.Rows)
	b := BasisOf(sh.Axis)
	sl := newSlab(s, sh)
	for v := range sh.N {
		for u := range sh.N {
			if !sl.lit(sh.index(u, v)) {
				continue
			}
			p := sh.Voxel(u, v)
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
