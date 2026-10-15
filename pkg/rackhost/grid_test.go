package rackhost

import (
	"bytes"
	"slices"
	"strconv"
	"testing"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/rackpic"
)

const gridW, gridH = 160, 120

// cells cuts frame m into the n cells of a grid, as gridFrame lays them.
func cells(t *testing.T, m *rackpic.Image, n int) [][]byte {
	t.Helper()
	cols, rows := attractor.GridShape(n)
	xs, _ := attractor.GridEdges(m.W, cols, gridGap(m.W))
	ys, _ := attractor.GridEdges(m.H, rows, gridGap(m.W))
	out := make([][]byte, n)
	for i := range n {
		x, y := xs[i%cols], ys[i/cols]
		for row := y[0]; row < y[0]+y[1]; row++ {
			out[i] = append(out[i], m.Px[3*(row*m.W+x[0]):3*(row*m.W+x[0]+x[1])]...)
		}
	}
	return out
}

func drawn(px []byte) bool {
	for i := 0; i+2 < len(px); i += 3 {
		if !isBack(px[i : i+3]) {
			return true
		}
	}
	return false
}

// sweepTo puts the across dial on target, by its position in the dial.
func sweepTo(t *testing.T, r *Rack, target string) {
	t.Helper()
	r.mu.Lock()
	i := slices.Index(r.grid.ids, target)
	r.mu.Unlock()
	if i < 0 {
		t.Fatalf("the sweep does not offer %q: %v", target, r.grid.ids)
	}
	set(t, r, "sweep-p", strconv.Itoa(i))
}

func gridShot(t *testing.T, r *Rack) *rackpic.Image {
	t.Helper()
	m, err := r.Scene(gridW, gridH, 1)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// A grid of four is the model four times, two by two, with a gutter
// between: every cell drawn, and with nothing swept, every cell the same.
func TestGridDrawsTheModelInEachCell(t *testing.T) {
	r := newRack(t)
	set(t, r, "auto-rotate", "0", "pause-sw", "1", "view-n", "2")
	cs := cells(t, gridShot(t, r), 4)
	for i, c := range cs {
		if !drawn(c) {
			t.Errorf("cell %d is dark", i)
		}
		if len(c) == len(cs[3]) && !bytes.Equal(c, cs[3]) {
			t.Errorf("cell %d differs from cell 3 with nothing swept", i)
		}
	}
	m := gridShot(t, r)
	mid := m.W / 2
	for y := range m.H {
		if p := m.Px[3*(y*m.W+mid) : 3*(y*m.W+mid)+3]; !isBack(p) {
			t.Fatalf("the gutter is drawn on at row %d", y)
		}
	}
}

// The sweep dials offer the model's own: its parameters for a shape, the
// colorings for all, and a flow's start; a model change starts them at
// none.
func TestSweepDialsFollowTheModel(t *testing.T) {
	r := newRack(t)
	set(t, r, r.selector, "torus")
	sweepTo(t, r, "torus-r")
	r.mu.Lock()
	got := r.grid.ids
	c := r.ctls[r.byID["sweep-p"]]
	r.mu.Unlock()
	want, _ := attractor.GridSweepTargets("torus", false)
	if !slices.Equal(got, want) || len(c.Options) != len(want) || c.Labels[0] != "none" {
		t.Errorf("torus offers %v (%d options, %v)", got, len(c.Options), c.Labels)
	}
	set(t, r, r.selector, "lorenz")
	r.mu.Lock()
	got, at := r.grid.ids, r.value("sweep-p")
	r.mu.Unlock()
	if !slices.Equal(got, []string{"", "#src", "#map", "#start"}) || at != "0" {
		t.Errorf("lorenz offers %v, the dial at %s", got, at)
	}
}

// A shape swept across the grid is drawn at each cell's own value.
func TestParameterSweepDrawsEachCellsValue(t *testing.T) {
	r := newRack(t)
	set(t, r, "auto-rotate", "0", "pause-sw", "1", r.selector, "torus", "view-n", "2")
	r.mu.Lock()
	before := r.value("torus-r")
	r.mu.Unlock()
	sweepTo(t, r, "torus-r")
	cs := cells(t, gridShot(t, r), 4)
	for i := 1; i < 4; i++ {
		if bytes.Equal(cs[i], cs[0]) {
			t.Errorf("cell %d is drawn as cell 0", i)
		}
	}
	r.mu.Lock()
	knob := r.value("torus-r")
	r.mu.Unlock()
	if knob != before {
		t.Errorf("the sweep left torus-r at %s, not the %s it was", knob, before)
	}
}

// A map sweep colors each cell its own way; a start sweep runs a trajectory
// a cell, begun apart.
func TestColorAndStartSweeps(t *testing.T) {
	for _, target := range []string{"#map", "#start"} {
		t.Run(target, func(t *testing.T) {
			r := newRack(t)
			set(t, r, "auto-rotate", "0", r.selector, "lorenz", "view-n", "2")
			sweepTo(t, r, target)
			var cs [][]byte
			for range 20 {
				cs = cells(t, gridShot(t, r), 4)
			}
			if bytes.Equal(cs[3], cs[0]) {
				t.Errorf("%s: the last cell is drawn as the first", target)
			}
		})
	}
}

// Overlaid, the cells are drawn one over another on the whole frame, each
// after the first in a hue of its own.
func TestOverlayDrawsTheCellsOverEachOther(t *testing.T) {
	r := newRack(t)
	set(t, r, "auto-rotate", "0", "pause-sw", "1", r.selector, "torus", "view-n", "1")
	sweepTo(t, r, "torus-R")
	set(t, r, "sweep-hi", "0.35") // big enough to see, and small enough to stay in frame
	tiled := gridShot(t, r)
	set(t, r, "grid-ovl", "1")
	m := gridShot(t, r)
	if bytes.Equal(m.Px, tiled.Px) {
		t.Fatal("overlay drew the cells side by side")
	}
	// The second cell's hue is green (GridOverlayTints), which the
	// default red-to-blue gradient never draws.
	found := false
	for i := 0; i+2 < len(m.Px); i += 3 {
		if r, g, b := int(m.Px[i]), int(m.Px[i+1]), int(m.Px[i+2]); g > r+40 && g > b+20 {
			found = true
			break
		}
	}
	if !found {
		t.Error("the second cell is not drawn in its own hue")
	}
}

// Unlinked, each cell keeps a coloring of its own: the panel shows the
// focused cell's, and a cell never focused opens at the page's.
func TestUnlinkedCellsKeepTheirOwnKnobs(t *testing.T) {
	r := newRack(t)
	set(t, r, "view-n", "1", "link-sw", "0", "gradient-colors", "4")
	set(t, r, "focus-n", "1")
	r.mu.Lock()
	b := r.value("gradient-colors")
	r.mu.Unlock()
	if b != "2" {
		t.Errorf("cell B opens at gradient-colors %s, want the page's 2", b)
	}
	set(t, r, "focus-n", "0")
	r.mu.Lock()
	a := r.value("gradient-colors")
	r.mu.Unlock()
	if a != "4" {
		t.Errorf("cell A came back at gradient-colors %s, want the 4 it was left at", a)
	}
}

// Every model that sweeps draws a grid of each thing it sweeps.
func TestEverySweepDraws(t *testing.T) {
	r := newRack(t)
	set(t, r, "view-n", "2")
	for model := range attractor.GridSweepParams {
		set(t, r, r.selector, model)
		r.mu.Lock()
		ids := slices.Clone(r.grid.ids)
		r.mu.Unlock()
		for i := range ids {
			set(t, r, "sweep-p", strconv.Itoa(i), "sweep2-p", strconv.Itoa((i+1)%len(ids)))
			if _, err := r.Scene(gridW, gridH, 1); err != nil {
				t.Errorf("%s sweeping %q: %v", model, ids[i], err)
			}
		}
	}
}
