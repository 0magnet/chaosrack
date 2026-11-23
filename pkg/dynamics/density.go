package dynamics

import "math"

// Density counts where a map's iterates land, cell by cell over a box fixed
// when it starts: the pixel-based density approximation of
// ratwolfzero/hopalong_python. A point plot of a map shows where the orbit
// has been; the counts show how often, and that is where the structure of a
// Clifford or a de Jong figure is, in the folds the orbit keeps returning to.
// Shown log-scaled, since a few cells take most of the hits.
type Density struct {
	W, H   int
	Lo, Hi [2]float64
	Count  []uint32
	Max    uint32
	// Hits is how many iterates have been counted, in the box or not.
	Hits uint64
	// sx and sy are cells per unit, so Add multiplies rather than divides.
	sx, sy float64
	// levelOf maps a count to its level byte, rebuilt by LevelBytes for the
	// current Max.
	levelOf []byte
}

// DensityMaxSide is the longer side of the grid in cells: fine enough that a
// cell is about a screen pixel at the size a figure is drawn, and small
// enough to tone-map and upload often.
const DensityMaxSide = 768

// Reset starts the counts over on a grid fitted to the box lo..hi, keeping
// its aspect, with DensityMaxSide cells on the longer side.
func (d *Density) Reset(lo, hi [2]float64) {
	w, h := hi[0]-lo[0], hi[1]-lo[1]
	if !(w > 0) {
		w = 1
	}
	if !(h > 0) {
		h = 1
	}
	if w >= h {
		d.W, d.H = DensityMaxSide, max(int(DensityMaxSide*h/w+0.5), 1)
	} else {
		d.W, d.H = max(int(DensityMaxSide*w/h+0.5), 1), DensityMaxSide
	}
	d.Lo, d.Hi = lo, [2]float64{lo[0] + w, lo[1] + h}
	if cap(d.Count) >= d.W*d.H {
		d.Count = d.Count[:d.W*d.H]
		clear(d.Count)
	} else {
		d.Count = make([]uint32, d.W*d.H)
	}
	d.Max, d.Hits = 0, 0
	d.sx, d.sy = float64(d.W)/w, float64(d.H)/h
}

// Add counts one iterate. One outside the box is counted in Hits only.
func (d *Density) Add(x, y float64) {
	d.Hits++
	i := int((x - d.Lo[0]) * d.sx)
	j := int((y - d.Lo[1]) * d.sy)
	if i < 0 || j < 0 || i >= d.W || j >= d.H {
		return
	}
	k := j*d.W + i
	d.Count[k]++
	if d.Count[k] > d.Max {
		d.Max = d.Count[k]
	}
}

// Level is cell k's count on a log scale, 0 for no hits and 1 for the most.
func (d *Density) Level(k int) float64 {
	c := d.Count[k]
	if c == 0 || d.Max == 0 {
		return 0
	}
	return math.Log1p(float64(c)) / math.Log1p(float64(d.Max))
}

// densityLevelTable is how many counts LevelBytes looks up rather than
// computes: nearly every cell's count is below it.
const densityLevelTable = 1 << 16

// levelByte is count c's Level scaled to 1..255, for a grid whose busiest
// cell has m hits.
func levelByte(c, m uint32) byte {
	return byte(max(math.Log1p(float64(c))/math.Log1p(float64(m))*255+0.5, 1))
}

// LevelBytes writes every cell's Level, scaled to 1..255 (0 for an empty
// cell), into dst, which must hold W×H bytes. The levels of the small counts
// are tabled first for the current Max, so the pass over the grid is a load
// per cell: a logarithm per cell was most of the frame at 768 cells square.
func (d *Density) LevelBytes(dst []byte) {
	if d.Max == 0 {
		clear(dst)
		return
	}
	n := min(int(d.Max)+1, densityLevelTable)
	if cap(d.levelOf) < densityLevelTable {
		d.levelOf = make([]byte, densityLevelTable)
	}
	tab := d.levelOf[:n]
	tab[0] = 0
	for c := 1; c < n; c++ {
		tab[c] = levelByte(uint32(c), d.Max)
	}
	for k, c := range d.Count {
		if int(c) < n {
			dst[k] = tab[c]
		} else {
			dst[k] = levelByte(c, d.Max)
		}
	}
}

// DensityBox is the box a map's figure fills: n iterates of each of its
// orbits past a transient, with a margin of 3% so the edges are not cut.
func DensityBox(m MapSys, n int) (lo, hi [2]float64) {
	lo = [2]float64{math.Inf(1), math.Inf(1)}
	hi = [2]float64{math.Inf(-1), math.Inf(-1)}
	orbits := m.Count()
	per := max(n/orbits, 1)
	for o := range orbits {
		p := m.Start(o, orbits)
		for i := range per + 200 {
			x, y, z := m.Step(p[0], p[1], p[2])
			if !Bounded(x, y, z) {
				p = m.Start(o, orbits)
				continue
			}
			p = [3]float64{x, y, z}
			if i < 200 {
				continue
			}
			lo[0], lo[1] = math.Min(lo[0], x), math.Min(lo[1], y)
			hi[0], hi[1] = math.Max(hi[0], x), math.Max(hi[1], y)
		}
	}
	if math.IsInf(lo[0], 0) || math.IsInf(lo[1], 0) {
		return [2]float64{-1, -1}, [2]float64{1, 1}
	}
	mx, my := 0.03*(hi[0]-lo[0]), 0.03*(hi[1]-lo[1])
	return [2]float64{lo[0] - mx, lo[1] - my}, [2]float64{hi[0] + mx, hi[1] + my}
}
