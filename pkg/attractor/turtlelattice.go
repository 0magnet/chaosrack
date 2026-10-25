package attractor

import (
	"math"

	"github.com/0magnet/lattice"
)

// The Turtle in the Lattice, on its grid.
//
// The Turtle walks the integer lattice: every step one unit along an axis,
// every turn square. The Lattice is a grid of voxels on the same three axes. So
// the walk can stand on the voxels exactly, every step a run of them straight
// along an axis, and the lattice writes it as the straight lines ─ │ it is.
// Handed the trail as the screen draws it — scaled to fit by its extent, the
// camera's drift canceled, corners perhaps rounded — its steps fell between
// voxels, and each came out as a ragged stair of whichever cells it grazed.
//
// THE SCALE IS A WHOLE NUMBER. Small enough a walk is given q voxels a step,
// which enlarges it to fill the volume; too big for one voxel a step, it is
// given one voxel for every −q steps, counted from the walk's own origin, so a
// step along an axis is still a move along it, of one voxel or none. Counted
// from the origin rather than from the trail's corner, a walk that drifts
// moves through the grid by whole voxels instead of being cut afresh into
// different voxels every time the trail is trimmed.
//
// It changes as rarely as it can: coarser at once when the walk no longer
// fits, finer only when it would fit with an eighth to spare, so a trail that
// grows and is trimmed back does not flick between two sizes.

// latticeRes is how many voxels one step is at scale q.
func latticeRes(q int) float64 {
	if q > 0 {
		return float64(q)
	}
	return 1 / float64(-q)
}

// latticeScaleFor is the finest scale a walk span steps across fits rows
// voxels at.
func latticeScaleFor(span float64, rows int) int {
	room := float64(rows - 1)
	if span <= room {
		return max(1, int(room/max(span, 1)))
	}
	return -int(math.Ceil(span / room))
}

// floorDiv is a/b rounded down, for b > 0: the voxel a lattice coordinate is
// in when b steps make one.
func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && a < 0 {
		q--
	}
	return q
}

// onLattice is the trail's n points from base on the voxel centers of a
// lattice rows to a side, the repeated voxels dropped, with each point's tint.
func (t *turtleWalk) onLattice(base, n, rows int, out []lattice.Vec3, tints []float32) ([]lattice.Vec3, []float32) {
	out, tints = out[:0], tints[:0]
	if n < 1 || rows < 2 {
		return out, tints
	}
	pts := t.pts[base : base+n]
	lo, hi := pts[0], pts[0]
	for _, p := range pts {
		lo.X, lo.Y, lo.Z = min(lo.X, p.X), min(lo.Y, p.Y), min(lo.Z, p.Z)
		hi.X, hi.Y, hi.Z = max(hi.X, p.X), max(hi.Y, p.Y), max(hi.Z, p.Z)
	}
	span := float64(max(hi.X-lo.X, hi.Y-lo.Y, hi.Z-lo.Z))
	fit, roomy := latticeScaleFor(span, rows), latticeScaleFor(span*9/8, rows)
	switch {
	case t.latQ == 0 || latticeRes(t.latQ) > latticeRes(fit):
		t.latQ = fit
	case latticeRes(t.latQ) < latticeRes(roomy):
		t.latQ = roomy
	}
	q := t.latQ
	// pisano's Y grows downward, matching a terminal; here up is up.
	vox := func(p int) int {
		if q > 0 {
			return p * q
		}
		return floorDiv(p, -q)
	}
	at := func(p [3]int) [3]int { return [3]int{vox(p[0]), vox(-p[1]), vox(p[2])} }
	// Centered: the voxels the walk reaches, put in the middle of the rows.
	vlo := at([3]int{lo.X, hi.Y, lo.Z})
	vhi := at([3]int{hi.X, lo.Y, hi.Z})
	var off [3]int
	for a := range 3 {
		off[a] = (rows-1-(vhi[a]-vlo[a]))/2 - vlo[a]
	}
	center := func(i int) float64 { return (float64(i)+0.5)/float64(rows)*2 - 1 }
	last := [3]int{math.MinInt, 0, 0}
	for i, p := range pts {
		v := at([3]int{p.X, p.Y, p.Z})
		if v == last {
			continue
		}
		last = v
		out = append(out, lattice.Vec3{center(v[0] + off[0]), center(v[1] + off[1]), center(v[2] + off[2])})
		tints = append(tints, t.tint[base+i])
	}
	return out, tints
}

// latticeSteps is line, a walk on voxel centers, as the Lattice is handed it:
// each step a segment of its own, begun just past the voxel it leaves.
//
// THE CORNERS ARE WHY. A voxel is written in the direction of everything that
// passes through it, summed, and a corner on a voxel's center is passed through
// by the step in and the step out: one along x and one along y, which sum to a
// diagonal. Every turn of the walk came out as ╱ or ╲. Begun past the voxel it
// leaves, a step writes only the voxels it enters, so a corner reads as the end
// of the run that reached it and the next run starts straight from there.
func latticeSteps(line []lattice.Vec3, rows int, flat []lattice.Vec3, segs [][]lattice.Vec3) ([]lattice.Vec3, [][]lattice.Vec3) {
	flat, segs = flat[:0], segs[:0]
	past := 0.51 * 2 / float64(rows) // a little over half a voxel
	for i := 1; i < len(line); i++ {
		a, b := line[i-1], line[i]
		d := b.Add(a.Scale(-1))
		l := d.Len()
		if l == 0 {
			continue
		}
		flat = append(flat, a.Add(d.Scale(past/l)), b)
	}
	for k := 0; k+1 < len(flat); k += 2 {
		segs = append(segs, flat[k:k+2:k+2])
	}
	return flat, segs
}

// latticeLine is the trail on screen, as Step last left it at budget, on the
// voxel centers of a Lattice rows to a side (onLattice). Nil before there is
// a trail.
func (r *TurtleRun) latticeLine(budget, rows int) []lattice.Vec3 {
	t := r.walk
	if t == nil || len(t.pts) < 2 {
		return nil
	}
	n := min(len(t.pts), min(budget, t.trailLen()))
	line, _ := t.onLattice(len(t.pts)-n, n, rows, nil, nil)
	return line
}

// LatticeSteps is what the Lattice draws in place of the trail as the screen
// shows it: the walk on its voxels, a step at a time (latticeSteps). Nil
// before there is a trail.
func (r *TurtleRun) LatticeSteps(budget, rows int) [][]lattice.Vec3 {
	_, segs := latticeSteps(r.latticeLine(budget, rows), rows, nil, nil)
	return segs
}

// Rows is how many voxels the Lattice has to a side: its terminals' rows.
func (l *LatticeStack) Rows() int {
	n, _ := l.args(false)
	return n
}
