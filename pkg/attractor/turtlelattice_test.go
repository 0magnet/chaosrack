package attractor

import (
	"bytes"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/0magnet/lattice"
)

// TestTheTurtleStandsOnTheLatticesVoxels walks Turtles too big for the
// Lattice and small enough to enlarge, flat and in three dimensions, and
// checks the line the Lattice is handed: every point a voxel's center inside
// the volume, and every step along one axis only, so the Lattice writes it as
// the straight run it is.
func TestTheTurtleStandsOnTheLatticesVoxels(t *testing.T) {
	for _, c := range []struct {
		name            string
		mod, dim, trail float32
	}{
		// A drifting walk on the whole trail grows until the budget stops it,
		// and the scale with it; on TRAIL long it reaches a length and keeps it.
		{"fib mod 25, 3-D, drifting", 25, 3, 1},
		{"fib mod 30, 3-D, closed", 30, 3, 0},
		{"fib mod 12, flat, closed", 12, 2, 0},
	} {
		for _, rows := range []int{12, 24} {
			k := TurtleDefaults
			k[0], k[4], k[6] = c.mod, c.dim, c.trail
			var r TurtleRun
			r.Step(k, 20000, 600, 1, 1, true, nil, func(x, y, z, tint float32) {})
			line := r.latticeLine(20000, rows)
			if len(line) < 2 {
				t.Fatalf("%s at %d rows: %d points", c.name, rows, len(line))
			}
			for i, p := range line {
				for a := range 3 {
					v := (p[a] + 1) / 2 * float64(rows) // voxels from the low edge
					if v < 0 || v >= float64(rows) || math.Abs(v-math.Floor(v)-0.5) > 1e-9 {
						t.Fatalf("%s at %d rows: point %d %v is not a voxel's center", c.name, rows, i, p)
					}
				}
				if i == 0 {
					continue
				}
				moved := 0
				for a := range 3 {
					if math.Abs(p[a]-line[i-1][a]) > 1e-9 {
						moved++
					}
				}
				if moved != 1 {
					t.Fatalf("%s at %d rows: step %d from %v to %v is not along one axis", c.name, rows, i, line[i-1], p)
				}
			}
			// Walked on, the scale holds: the trail grows and is trimmed, and
			// the figure is not to flick between two sizes as it does.
			q, changes := r.walk.latQ, 0
			for range 50 {
				r.Step(k, 20000, 4, 1, 1, true, nil, func(x, y, z, tint float32) {})
				r.latticeLine(20000, rows)
				if r.walk.latQ != q {
					q = r.walk.latQ
					changes++
				}
			}
			if changes > 1 {
				t.Errorf("%s at %d rows: the scale changed %d times in 50 updates", c.name, rows, changes)
			}
			t.Logf("%s at %d rows: %d points, scale %d", c.name, rows, len(line), r.walk.latQ)
		}
	}
}

// TestTheTurtlesCornersAreNotDiagonals draws a closed Turtle walk with the
// lattice program itself and reads what it writes: every step a straight ─ or
// │ (or · where the step runs through the sheet), and not one ╱ or ╲. A
// corner on a voxel's center was written as the sum of its two steps, a
// diagonal, at every turn (latticeSteps).
func TestTheTurtlesCornersAreNotDiagonals(t *testing.T) {
	k := TurtleDefaults
	k[0], k[4] = 12, 2 // fib mod 12, flat: a closed walk of square turns
	var r TurtleRun
	r.Step(k, 20000, 600, 1, 1, true, nil, func(x, y, z, tint float32) {})
	for _, rows := range []int{12, 24} {
		var f lattice.Figure
		f.Set(r.LatticeSteps(20000, rows))
		straight, diagonal := 0, 0
		for _, ax := range []string{"x", "y", "z"} {
			for s := range rows {
				p, err := lattice.NewProgram([]string{"-n", strconv.Itoa(rows), "-axis", ax, "-slice", strconv.Itoa(s),
					"-style", "lines", "-spin", "0", "-pitch", "0", "-no-color"}, nil)
				if err != nil {
					t.Fatal(err)
				}
				p.Show(&f)
				var b bytes.Buffer
				if err := p.Print(&b, time.Unix(0, 0)); err != nil {
					t.Fatal(err)
				}
				straight += strings.Count(b.String(), "─") + strings.Count(b.String(), "│")
				diagonal += strings.Count(b.String(), "╱") + strings.Count(b.String(), "╲")
			}
		}
		if straight == 0 || diagonal > 0 {
			t.Errorf("at %d rows: %d straight glyphs, %d diagonal", rows, straight, diagonal)
		}
	}
}
