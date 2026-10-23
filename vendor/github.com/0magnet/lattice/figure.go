package lattice

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
)

// A Figure is what a lattice draws when the thing to be drawn is not a solid:
// lines and points, as an oscilloscope or a plotter would draw them — a
// trajectory, a wire-frame mesh, a cloud of dots. A voxel is lit where the
// figure passes through it, and its character is the way the figure runs
// across the sheet: ─ │ ╱ ╲, or · where it runs through the sheet instead of
// along it. A surface's character is the line it makes across the sheet; a
// line's is itself.
//
// One Figure may be shown by every program of a stack. It is voxelized once
// for a resolution and a turn, and every sheet reads its own slice of that.
type Figure struct {
	mu    sync.Mutex
	lines [][]Vec3
	// colors are each point's color, beside lines, when the figure brings its
	// own (SetColored); nil, its voxels are colored by the Coloring.
	colors [][]RGB
	gen    uint64

	// The last voxelization, and what it was made for.
	grid    *figGrid
	gridGen uint64
	gridN   int
	gridAs  turn
}

// Set replaces the figure: polylines in the volume's coordinates, ±1 on
// every axis (what lies outside is not drawn). A polyline of one point is a
// dot. Set keeps the slices it is given: they must not change until the
// figure has been drawn from, or Set again.
func (f *Figure) Set(lines [][]Vec3) {
	f.SetColored(lines, nil)
}

// RGB is a color, 0..255 a channel.
type RGB [3]uint8

// SetColored is Set with a color for every point: colors[i][k] is the color of
// lines[i][k], and a voxel is drawn in the mean of the colors of the figure
// where it passes through it, a segment's color running from one end's to the
// other's. A host draws a figure in its own colors so: a trajectory colored
// by height, say, is that color in the lattice too, not the volume's. A line
// with no colors, or too few, is colored by the Coloring. Set keeps the slices
// it is given, as Set does.
func (f *Figure) SetColored(lines [][]Vec3, colors [][]RGB) {
	f.mu.Lock()
	f.lines, f.colors = lines, colors
	f.gen++
	f.mu.Unlock()
}

// generation counts the Sets, so a program knows when its picture is stale.
func (f *Figure) generation() uint64 {
	f.mu.Lock()
	g := f.gen
	f.mu.Unlock()
	return g
}

// figGrid is a figure in voxels: whether each is lit, and the direction the
// figure runs through it, summed over every piece of it that passes.
type figGrid struct {
	n   int
	lit []bool
	dir []Vec3
	// rgb sums the colors that passed through a voxel and hits counts them,
	// for a colored figure; hits is 0 where it brought none.
	rgb  [][3]float32
	hits []float32
}

func (g *figGrid) at(i [3]int) int { return (i[0]*g.n+i[1])*g.n + i[2] }

// voxels is the figure turned as tu says, in voxels n to a side.
func (f *Figure) voxels(n int, tu turn) *figGrid {
	f.mu.Lock()
	g := f.voxelize(n, tu)
	f.mu.Unlock() // not deferred: TinyGo wraps a deferring function in a panic catch that cost as much as this
	return g
}

// voxelize is voxels, with f locked.
func (f *Figure) voxelize(n int, tu turn) *figGrid {
	if f.grid != nil && f.gridGen == f.gen && f.gridN == n && f.gridAs == tu {
		return f.grid
	}
	g := f.grid
	if g == nil || g.n != n {
		g = &figGrid{n: n, lit: make([]bool, n*n*n), dir: make([]Vec3, n*n*n),
			rgb: make([][3]float32, n*n*n), hits: make([]float32, n*n*n)}
	} else {
		clear(g.lit)
		clear(g.dir)
		clear(g.rgb)
		clear(g.hits)
	}
	place := tu.forward()
	if tu == (turn{pose: Identity}) {
		place = func(p Vec3) Vec3 { return p } // the usual case: a host that turns the stack itself
	}
	half := float64(n) / 2
	mark := func(p, d Vec3, c [3]float32, colored bool) {
		var j int
		for a := range 3 {
			x := (p[a] + 1) * half // voxels from the low edge
			if !(x >= 0) || x >= float64(n) {
				return
			}
			j = j*n + int(x)
		}
		g.lit[j] = true
		if colored {
			g.rgb[j][0] += c[0]
			g.rgb[j][1] += c[1]
			g.rgb[j][2] += c[2]
			g.hits[j]++
		}
		// A line has a direction but no sense: one running back over the
		// same voxel adds to it rather than canceling it.
		if g.dir[j].Dot(d) < 0 {
			d = d.Scale(-1)
		}
		g.dir[j] = g.dir[j].Add(d)
	}
	for i, l := range f.lines {
		if len(l) == 0 {
			continue
		}
		// This line's colors, if it brought one for every point.
		var cs []RGB
		if i < len(f.colors) && len(f.colors[i]) >= len(l) {
			cs = f.colors[i]
		}
		colored := cs != nil
		at := func(k int) (c [3]float32) {
			if colored {
				c = [3]float32{float32(cs[k][0]), float32(cs[k][1]), float32(cs[k][2])}
			}
			return c
		}
		lerp := func(x, y [3]float32, t float32) [3]float32 {
			return [3]float32{x[0] + (y[0]-x[0])*t, x[1] + (y[1]-x[1])*t, x[2] + (y[2]-x[2])*t}
		}
		a := place(l[0])
		if len(l) == 1 {
			mark(a, Vec3{}, at(0), colored)
			continue
		}
		for k := 1; k < len(l); k++ {
			b := place(l[k])
			ca, cb := at(k-1), at(k)
			d := b.Add(a.Scale(-1))
			// Sampled at no more than half a voxel apart, so no voxel the
			// segment crosses is stepped over: a short segment, as most of a
			// trail's are, is its far end alone.
			steps := max(1, int(math.Ceil(d.Len()*half*2)))
			steps = min(steps, 4*n) // a jump across the volume, not a line through it
			if k == 1 {
				mark(a, d, ca, colored)
			}
			for s := 1; s <= steps; s++ {
				t := float64(s) / float64(steps)
				mark(a.Add(d.Scale(t)), d, lerp(ca, cb, float32(t)), colored)
			}
			a = b
		}
	}
	f.grid, f.gridGen, f.gridN, f.gridAs = g, f.gen, n, tu
	return g
}

// renderFigure draws sheet sh of figure f, turned as tu says.
func renderFigure(sh Sheet, f *Figure, tu turn, st Style, co Coloring, cells []Cell) Frame {
	fr := blank(sh, cells)
	g := f.voxels(sh.N, tu)
	b := BasisOf(sh.Axis)
	for v := range sh.N {
		for u := range sh.N {
			j := g.at(sh.index(u, v))
			if !g.lit[j] {
				continue
			}
			p := sh.Voxel(u, v)
			r, gr, bl := color(p, co)
			if h := g.hits[j]; h > 0 && co != Mono {
				// The figure's own colors, met here (SetColored).
				r, gr, bl = uint8(g.rgb[j][0]/h+0.5), uint8(g.rgb[j][1]/h+0.5), uint8(g.rgb[j][2]/h+0.5)
			}
			c := Cell{Ch: lineGlyph(g.dir[j], b, st), R: r, G: gr, B: bl}
			fr.Cells[v*fr.Cols+2*u] = c
			fr.Cells[v*fr.Cols+2*u+1] = c
		}
	}
	return fr
}

// lineGlyph is the character for a figure running in direction d through a
// sheet of basis b.
func lineGlyph(d Vec3, b Basis, st Style) rune {
	if st == Solid {
		return '█'
	}
	dl := d.Len()
	if dl == 0 { // a dot, with no direction at all
		if st == ASCII {
			return 'o'
		}
		return '•'
	}
	du, dv := d.Dot(b.Right), d.Dot(b.Up)
	along := math.Hypot(du, dv) / dl // how much of it runs along the sheet
	if st == Shade {
		// As a surface's: how squarely the figure crosses the sheet, from
		// lying in it to running straight through.
		const ramp = ".:-=+*#"
		return rune(ramp[int(math.Round(math.Sqrt(math.Max(0, 1-along*along))*float64(len(ramp)-1)))])
	}
	lines := [5]rune{'·', '─', '╱', '│', '╲'}
	if st == ASCII {
		lines = [5]rune{'.', '-', '/', '|', '\\'}
	}
	if along < 0.4 {
		return lines[0]
	}
	a := math.Mod(math.Atan2(dv, du)+2*math.Pi, math.Pi)
	return lines[1+int(math.Round(a/(math.Pi/4)))%4]
}

// forward is the turn as it moves a point of the figure: the yaw about the
// vertical, then the tilt, then the pose. A solid is turned by moving the
// point it is asked about the other way (Turned, Posed); a figure is moved
// itself.
func (tu turn) forward() func(Vec3) Vec3 {
	cy, sy := math.Cos(tu.yaw), math.Sin(tu.yaw)
	cp, sp := math.Cos(tu.pitch), math.Sin(tu.pitch)
	return func(p Vec3) Vec3 {
		x := cy*p[0] + sy*p[2]
		z := -sy*p[0] + cy*p[2]
		y := cp*p[1] - sp*z
		z = sp*p[1] + cp*z
		return tu.pose.rotate(Vec3{x, y, z})
	}
}

// ReadFigure reads a figure as text: a point a line, "x y z", and a blank
// line between polylines. It is scaled to fill nine tenths of the volume, about
// the origin, whatever units it was written in.
func ReadFigure(r io.Reader) ([][]Vec3, error) {
	var lines [][]Vec3
	var cur []Vec3
	most := 0.0
	sc := bufio.NewScanner(r)
	for ln := 1; sc.Scan(); ln++ {
		t := strings.TrimSpace(sc.Text())
		if t == "" || t[0] == '#' {
			if t == "" && len(cur) > 0 {
				lines, cur = append(lines, cur), nil
			}
			continue
		}
		f := strings.Fields(t)
		if len(f) != 3 {
			return nil, fmt.Errorf("line %d: want x y z, got %q", ln, t)
		}
		var p Vec3
		for a := range 3 {
			v, err := strconv.ParseFloat(f[a], 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, fmt.Errorf("line %d: %q is not a number", ln, f[a])
			}
			p[a] = v
			most = math.Max(most, math.Abs(v))
		}
		cur = append(cur, p)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(cur) > 0 {
		lines = append(lines, cur)
	}
	if most > 0 {
		s := 0.9 / most
		for _, l := range lines {
			for i := range l {
				l[i] = l[i].Scale(s)
			}
		}
	}
	return lines, nil
}
