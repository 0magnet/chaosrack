package attractor

import (
	"strconv"

	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// The bifurcation explorer's sweep, shared by the page (bifurcation_js.go,
// whose file comment says what the diagram shows) and a native rack: one
// parameter of a flow swept across its knob range, a fresh trajectory
// integrated per column, and the local maxima of z kept against the
// parameter. It computes progressively, a few columns a call, left to
// right, so the diagram grows across the screen live.

// bifPerCol is how many maxima are kept per column. (bifCols, the sweep
// resolution, lives in bifdrive.go with the cursor arithmetic that depends on
// it.)
const bifPerCol = 40

// BifSweep is the sweep and the columns drawn so far.
type BifSweep struct {
	sig      string // source|param sig the data was computed for
	nextCol  int
	colOf    []int32   // column index per stored point
	valOf    []float64 // z-maximum per stored point
	min, max float64
}

// Fill sweeps up to cols more columns of flow's parameter number pidx, which
// points at value and runs lo..hi. A different flow or parameter starts the
// diagram again. It reports whether the flow has a vector field to sweep.
func (b *BifSweep) Fill(flow string, pidx int, value *float32, lo, hi float32, cols int) bool {
	sys, ok := dynamics.FlowFor4(flow)
	if !ok || value == nil {
		return false
	}
	sig := flow + "|" + strconv.Itoa(pidx)
	if b.sig != sig {
		b.sig = sig
		b.nextCol = 0
		b.colOf = b.colOf[:0]
		b.valOf = b.valOf[:0]
		b.min, b.max = 1e30, -1e30
	}
	if sys.Interpreted {
		cols = 1 // an interpreted field costs many times a compiled one
	}
	dt := sys.Dt()
	ic := dynamics.InitCondFor(flow)
	for c := 0; c < cols && b.nextCol < bifCols; c++ {
		j := b.nextCol
		b.nextCol++
		pv := lo + (hi-lo)*float32(j)/float32(bifCols-1)
		saved := *value
		*value = pv
		s := [4]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), sys.W()}
		const transient = 1500
		for range transient {
			flowStep(sys, &s, dt)
			if flowDiverged(s) {
				break
			}
		}
		z2, z1 := s[2], s[2]
		got := 0
		for i := 0; i < 6000 && got < bifPerCol; i++ {
			flowStep(sys, &s, dt)
			if flowDiverged(s) {
				break
			}
			z0 := s[2]
			if z1 > z2 && z1 >= z0 { // local maximum at z1
				b.colOf = append(b.colOf, int32(j)) //nolint:gosec // a column index into the bifurcation image
				b.valOf = append(b.valOf, z1)
				b.min = min(b.min, z1)
				b.max = max(b.max, z1)
				got++
			}
			z2, z1 = z1, z0
		}
		*value = saved
	}
	return true
}

// Started reports whether a quarter of the sweep is done, which is enough to
// frame the diagram by.
func (b *BifSweep) Started() bool { return b.nextCol >= bifCols/4 }

// Points hands put the diagram's points, at most n of them, on the page's
// plane: the parameter across ±20, the maxima up ±12 over the range found so
// far, and the fraction across, which the gradient follows. It reports how
// many it handed over: none until two have been found over some range.
//
// The whole diagram is rendered from the raw (column, value) pairs each time
// — the y normalization tracks the running min and max, so early columns stay
// correctly placed as later ones widen the range.
func (b *BifSweep) Points(n int, put func(i int, x, y, frac float32)) int {
	n = min(n, len(b.colOf))
	if n < 2 || b.max <= b.min {
		return 0
	}
	span := b.max - b.min
	for i := range n {
		fx := float32(b.colOf[i]) / float32(bifCols-1)
		fy := float32((b.valOf[i] - b.min) / span)
		put(i, (fx*2-1)*20, (fy*2-1)*12, fx)
	}
	return n
}

// SweepParam is the parameter of ps a bifurcation sweeps at choice idx: idx
// itself when it is one of them, and otherwise the first that is not the
// timestep.
func SweepParam(ps []dynamics.Param, idx int) int {
	if len(ps) == 0 {
		return -1
	}
	if idx >= 0 && idx < len(ps) {
		return idx
	}
	for i := range ps {
		if ps[i].Label != "dt" {
			return i
		}
	}
	return 0
}

// bifCursorRulePts is how many points the vertical rule is drawn with. Points
// and not a line strip because the diagram is a single POINTS draw and this
// rides the same pipeline; a strip would be a second draw call and a second
// pass through uploadVerticesOnly's in-place center subtraction, which is
// the reasoning the Poincaré return map's y=x diagonal already settled.
const bifCursorRulePts = 96

// bifCursorBand is how many columns either side of the cursor's own are lit
// up with it. One: at 360 columns across the screen a single column is a
// hairline, and the point of the highlight is to show the attractor's slice AT
// this parameter — widening it further would start mixing in structure from
// parameter values the music is not at.
const bifCursorBand = 1

// Cursor hands put the audio cursor at parameter value v of lo..hi, on the
// plane as Points puts the diagram: a rule across it, and then the diagram's
// own points in the cursor's column and bifCursorBand either side — the
// values the attractor settles onto at the parameter the music is at — each
// with its height up the plane.
func (b *BifSweep) Cursor(v, lo, hi float32, put func(x, y, frac float32)) {
	x := (bifFrac(v, lo, hi)*2 - 1) * 20
	for i := range bifCursorRulePts {
		fy := float32(i) / float32(bifCursorRulePts-1)
		put(x, (fy*2-1)*12, fy)
	}
	span := b.max - b.min
	if span <= 0 {
		return
	}
	col := int32(bifColumnFor(v, lo, hi, bifCols)) //nolint:gosec // a column index into the bifurcation image
	for i := range b.colOf {
		if b.colOf[i] < col-bifCursorBand || b.colOf[i] > col+bifCursorBand {
			continue
		}
		cx := float32(b.colOf[i]) / float32(bifCols-1)
		cy := float32((b.valOf[i] - b.min) / span)
		put((cx*2-1)*20, (cy*2-1)*12, cy)
	}
}

// BifAudioValue is where envelope env, 0..1, puts the swept parameter: across
// depth of its range lower..upper, centered on its knob's value center
// (bifdrive.go).
func BifAudioValue(env, depth, center, lower, upper float32) float32 {
	return bifAudioValue(env, depth, center, lower, upper)
}
