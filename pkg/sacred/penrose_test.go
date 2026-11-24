package sacred

import (
	"math"
	"testing"
)

// Every edge of a Penrose tiling is one length, and the thick rhombs
// outnumber the thin by a ratio that goes to φ.
func TestPenroseRhombs(t *testing.T) {
	ss := penrose(5)
	l0 := dist(ss[0][0], ss[0][1])
	for _, s := range ss {
		if l := dist(s[0], s[1]); math.Abs(l-l0) > 1e-9 {
			t.Fatalf("edges of %v and %v", l0, l)
		}
	}
	thick, thin := PenroseCounts(7)
	if r := float64(thick) / float64(thin); math.Abs(r-Phi) > 0.01 {
		t.Errorf("%d thick to %d thin: %v, want near φ", thick, thin, r)
	}
}

// The rhombic triacontahedron: sixty edges of one length, and every face a
// flat golden rhomb, its diagonals φ to 1.
func TestTriacontahedron(t *testing.T) {
	ss := triacontahedron()
	if len(ss) != 60 {
		t.Fatalf("%d edges, want 60", len(ss))
	}
	l0 := dist3(ss[0][0], ss[0][1])
	for _, s := range ss {
		if l := dist3(s[0], s[1]); math.Abs(l-l0) > 1e-9 {
			t.Fatalf("edges of %v and %v", l0, l)
		}
	}
	// A face: two corners an icosahedron edge apart, two face centers.
	// Corners are where five edges meet, centers where three do.
	count := map[[3]int64]int{}
	key := func(v V) [3]int64 {
		return [3]int64{int64(math.Round(v[0] * 1e6)), int64(math.Round(v[1] * 1e6)), int64(math.Round(v[2] * 1e6))}
	}
	for _, s := range ss {
		count[key(s[0])]++
		count[key(s[1])]++
	}
	fives, threes := 0, 0
	for _, n := range count {
		switch n {
		case 5:
			fives++
		case 3:
			threes++
		}
	}
	if fives != 12 || threes != 20 {
		t.Errorf("%d five-way and %d three-way corners, want 12 and 20", fives, threes)
	}
	// The long diagonal (between three-way corners across a face) is φ times
	// the short: check on one face, found from a corner and two of its
	// centers that share an edge with each other's neighbor corner.
	corner := ss[0][0]
	var centers []V
	for _, s := range ss {
		if key(s[0]) == key(corner) {
			centers = append(centers, s[1])
		}
	}
	// Two centers of one face are the nearest pair round the corner.
	best := math.Inf(1)
	var c0, c1 V
	for i := range centers {
		for j := i + 1; j < len(centers); j++ {
			if d := dist3(centers[i], centers[j]); d < best {
				best, c0, c1 = d, centers[i], centers[j]
			}
		}
	}
	// The face's other corner is the point both centers join besides corner.
	var other V
	for _, s := range ss {
		if key(s[1]) == key(c0) && key(s[0]) != key(corner) {
			for _, u := range ss {
				if key(u[1]) == key(c1) && key(u[0]) == key(s[0]) {
					other = s[0]
				}
			}
		}
	}
	long, short := dist3(c0, c1), dist3(corner, other)
	if short > long {
		long, short = short, long
	}
	if math.Abs(long/short-Phi) > 1e-6 {
		t.Errorf("diagonals %v and %v: ratio %v, want φ", long, short, long/short)
	}
}
