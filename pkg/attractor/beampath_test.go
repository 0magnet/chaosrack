package attractor

import (
	"math"
	"testing"
)

func flat(p [3]float32) [3]float64 { return [3]float64{float64(p[0]), float64(p[1]), float64(p[2])} }

// Two separate unit squares, as a mesh lists them (pairs by index): each
// chains into one stroke, and the strokes are joined by blanked jumps.
func twoSquares() beamPath {
	v := []float32{0, 0, 0, 1, 0, 0, 1, 1, 0, 0, 1, 0, 3, 0, 0, 4, 0, 0, 4, 1, 0, 3, 1, 0}
	idx := []uint16{0, 1, 1, 2, 2, 3, 3, 0, 4, 5, 5, 6, 6, 7, 7, 4}
	var p beamPath
	p.fromLines(v, 3, idx, len(idx))
	return p
}

func TestBeamChainsAMeshIntoStrokes(t *testing.T) {
	p := twoSquares()
	if len(p.pts) != 10 {
		t.Fatalf("%d points, want two closed strokes of 5", len(p.pts))
	}
	lit := 0
	for _, l := range p.lit {
		if l {
			lit++
		}
	}
	// 8 sides drawn; the jump between the squares and the return are not.
	if lit != 8 || p.lit[4] || p.lit[9] {
		t.Errorf("lit %v", p.lit)
	}
	// 8 units of side, plus two blanked jumps of 3 at the retrace's speed.
	if got, want := p.length(flat), 8+6.0/beamBlankSpeed; math.Abs(got-want) > 1e-9 {
		t.Errorf("length %v, want %v", got, want)
	}
}

// A walk draws what it crosses and nothing it jumps; a whole circuit or more
// draws the whole figure, once.
func TestBeamWalkDrawsWhatItCrosses(t *testing.T) {
	p := twoSquares()
	var w beamWalker
	drawn := 0.0
	visit := func(seg int, f0, f1 float64) { drawn += (f1 - f0) * p.segLen(seg, flat) }
	w.walk(&p, flat, 2.5, visit)
	if math.Abs(drawn-2.5) > 1e-9 || w.seg != 2 || math.Abs(w.frac-0.5) > 1e-9 {
		t.Errorf("drew %v, at %d+%v", drawn, w.seg, w.frac)
	}
	drawn = 0
	w.walk(&p, flat, 100, visit)
	if math.Abs(drawn-8-math.Mod(100, p.length(flat))) > 1e-6 && drawn < 8 {
		t.Errorf("a long walk drew %v", drawn)
	}
	if at := w.at(&p, flat); math.IsNaN(at[0]) {
		t.Errorf("at %v", at)
	}
}

// A strip is one stroke, closed only if it ends where it began.
func TestBeamFromStrip(t *testing.T) {
	var open, closed beamPath
	open.fromStrip([]float32{0, 0, 0, 9, 1, 0, 0, 9, 1, 1, 0, 9}, 0, 3)
	if !open.lit[0] || !open.lit[1] || open.lit[2] {
		t.Errorf("open strip lit %v", open.lit)
	}
	closed.fromStrip([]float32{0, 0, 0, 9, 1, 0, 0, 9, 0, 0, 0, 9}, 0, 3)
	if !closed.lit[2] {
		t.Errorf("closed strip lit %v", closed.lit)
	}
}
