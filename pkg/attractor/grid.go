package attractor

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/colorspace"
	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// The Grid's arithmetic and its sweep tables, shared by the page (views_js.go,
// which says what the grid is for and why each rule is as it is) and a native
// rack: how many cells, where each one lies, what a model's sweep dials offer,
// and where a cell sits along each sweep.

// GridMax is how many cells the grid may hold.
const GridMax = 16

// GridCounts are the grid sizes the dial offers, by position.
var GridCounts = []int{1, 2, 4, 9, 16}

// GridCountRing is how the dial says them.
var GridCountRing = []string{"1", "2", "4", "9", "16"}

// GridCount is the cell count at the dial's position f.
func GridCount(f float32) int { return GridCounts[clampSel(f, len(GridCounts)-1)] }

// GridShape is the grid's columns and rows for n cells: two side by side,
// and the squares as near square as they come.
func GridShape(n int) (cols, rows int) {
	switch {
	case n <= 1:
		return 1, 1
	case n == 2:
		return 2, 1
	}
	cols = int(math.Ceil(math.Sqrt(float64(n))))
	rows = (n + cols - 1) / cols
	return cols, rows
}

// GridEdges divides an axis total long into n cells with gap between them,
// as {start, size} pairs, the remainder spent across the cells so the last
// ends on the edge; false when a cell would have no size.
func GridEdges(total, n, gap int) ([][2]int, bool) {
	if n < 1 {
		return nil, false
	}
	span := total + gap // the gaps come out of the boundaries below
	out := make([][2]int, n)
	for i := range n {
		start := i * span / n
		end := (i+1)*span/n - gap
		if end-start < 1 {
			return nil, false
		}
		out[i] = [2]int{start, end - start}
	}
	return out, true
}

// GridFitFactor is how much closer the camera comes for a figure fitted to
// the whole frame to fill one cell of n: what the tighter axis lost. 1
// overlaid, where every cell is the whole frame.
func GridFitFactor(n int, overlay bool) int {
	if overlay {
		return 1
	}
	cols, rows := GridShape(n)
	return min(cols, rows)
}

// GridParam is a parameter a grid may sweep, and the range it covers.
type GridParam struct {
	ID, Label string
	Min, Max  float32
}

// GridSweepParams are the parameters each model's sweep dials offer, in
// order: the numeric ones of the models a numeric sweep is true of (the
// audio embeddings, which redraw a window of sound, and the shapes, which
// rebuild from their knobs) — a flow integrates, so a cell's picture there
// is not a function of its value alone. A rotary of names is not offered:
// its cells would be unrelated pictures, not a range.
var GridSweepParams = map[string][]GridParam{
	"globe": {
		{"globe-lat", "lat", 0, 90},
		{"globe-lon", "lon", 0, 180},
		{"globe-twist", "twist", -8, 8},
	},
	"torus": {
		{"torus-R", "R", 0.1, 5},
		{"torus-r", "r", 0.1, 3},
		{"torus-stacks", "stacks", 3, 100},
		{"torus-slices", "slices", 3, 100},
		{"torus-roll", "roll", -8, 8},
	},
	"polyhedron": {
		{"polyhedron-p", "sides", 3, 6},
		{"polyhedron-q", "meet", 3, 6},
		{"polyhedron-morph", "morph", 0, 2},
		{"polyhedron-kis", "kis", -0.9, 1},
	},
	"takens": {
		{"takens-tau", "τ", 1, 512},
		{"takens-win", "win", 5, 500},
		{"takens-gain", "gain", 0.5, 50},
		{"takens-smooth", "smth", 1, 16},
	},
	"polar": {
		{"polar-drive", "drv", 0.2, 10},
		{"takens-tau", "τ", 1, 512},
		{"polar-win", "win", 5, 500},
		{"polar-gain", "gain", 0.5, 50},
		{"takens-smooth", "smth", 1, 16},
	},
	"stereo": {
		{"stereo-tau", "τ", 1, 512},
		{"stereo-win", "win", 5, 250},
		{"stereo-gain", "gain", 0.5, 50},
		{"stereo-align", "algn", -96, 96},
		{"stereo-width", "wide", 0, 3},
		{"stereo-vg", "vg", 0.1, 8},
		{"stereo-span", "span", 0.25, 8},
		{"stereo-lvl", "lvl", -1, 1},
		{"stereo-hyst", "hyst", 0, 0.5},
		{"stereo-hold", "hold", 0, 500},
		{"stereo-tpos", "tpos", 0, 1},
		{"takens-smooth", "smth", 1, 16},
	},
	"waterfall": {
		{"wfall-lines", "line", 4, 64},
		{"wfall-step", "step", 1, 100},
		{"wfall-top", "top", -60, 20},
		{"wfall-range", "rnge", 10, 80},
		{"wfall-depth", "dpth", 0.2, 3},
	},
}

// The sweeps that are not parameters: the coloring's source and map, which
// every model has, and a flow's start.
const (
	SweepNone  = ""
	SweepSrc   = "#src"
	SweepMap   = "#map"
	SweepStart = "#start"
)

// GridSweepTargets is what model's sweep dials offer, by position, and how
// the dial rings them: none, the model's parameters, the two colorings, and
// a flow's start last, so the positions a link holds for the others stay.
func GridSweepTargets(model string, startable bool) (ids, ring []string) {
	ids, ring = []string{SweepNone}, []string{"—"}
	for _, p := range GridSweepParams[model] {
		ids = append(ids, p.ID)
		ring = append(ring, p.Label)
	}
	ids = append(ids, SweepSrc, SweepMap)
	ring = append(ring, "csrc", "cmap")
	if startable {
		ids = append(ids, SweepStart)
		ring = append(ring, "start")
	}
	return ids, ring
}

// SweepRange is parameter id's range on model, as its knob covers it.
func SweepRange(model, id string) (lo, hi float32, ok bool) {
	for _, p := range GridSweepParams[model] {
		if p.ID == id {
			return p.Min, p.Max, true
		}
	}
	return 0, 0, false
}

// SweepColorSources is the order a source sweep steps through, by the
// gradient source's value: the audio-fed sources — audio, level, dB,
// correlation, side, balance, position, flux, pitch.
var SweepColorSources = []int{4, 6, 11, 7, 8, 9, 13, 10, 12}

// SweepColorMaps is the order a map sweep steps through: every genuine
// mapping, two-color first.
var SweepColorMaps = []int{2, 3, 4, 5, 6, 7, 8, 9, 10}

// SweepPick is the entry of list at t, 0..1 along it.
func SweepPick(list []int, t float32) int {
	return list[min(max(int(t*float32(len(list)-1)+0.5), 0), len(list)-1)]
}

// SweepFrac is where cell i of n sits in a sweep, 0..1; a single cell at
// the start.
func SweepFrac(i, n int) float32 {
	if n <= 1 {
		return 0
	}
	i = min(max(i, 0), n-1)
	return float32(i) / float32(n-1)
}

// SweepAxisFracs is where cell i of n sits on each axis, 0..1: a lone sweep
// runs over every cell in reading order; with a second, across is the
// column and down the row.
func SweepAxisFracs(i, n int, twoAxes bool) (across, down float32) {
	if !twoAxes {
		return SweepFrac(i, n), 0
	}
	cols, rows := GridShape(n)
	i = min(max(i, 0), n-1)
	return SweepFrac(i%cols, cols), SweepFrac(i/cols, rows)
}

// The span of ε a start sweep covers, as powers of ten: FROM and TO at 0 and
// 1 of it, except that 0 itself is ε = 0, the model's own trajectory.
const (
	StartEpsLo = -9
	StartEpsHi = -1
)

// StartEps is the ε a cell at t along the start sweep (FROM to TO already
// applied) starts away from the model's own trajectory.
func StartEps(t float32) float64 {
	if t <= 0 {
		return 0
	}
	return math.Pow(10, StartEpsLo+(StartEpsHi-StartEpsLo)*float64(t))
}

// GridOverlayTints are the colors overlaid cells after the first are drawn
// in: golden-ratio steps of hue from green, the color Twin's copy was.
var GridOverlayTints = func() (t [GridMax][3]float32) {
	for i := range t {
		t[i] = colorspace.FromHSV(0.39+float32(i)*0.618034, 0.85, 1)
	}
	return t
}()

// StartSweepable reports whether model can have a start sweep: whether it
// is a flow the generic stepper integrates. The bifurcation explorer and the
// Poincaré section are not, as the page draws them.
func StartSweepable(model string) bool {
	if model == "bifurcation" || model == "poincare" {
		return false
	}
	_, ok := dynamics.FlowFor4(model)
	return ok
}
