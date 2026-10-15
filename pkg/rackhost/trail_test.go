package rackhost

import (
	"math"
	"sync"
	"testing"
	"time"
)

// still is a rack drawing lorenz with nothing turning it, so that what
// changes between frames is the flow.
func still(t *testing.T, kv ...[2]string) *Rack {
	t.Helper()
	r := newRack(t)
	if r.selector == "" {
		t.Skip("no model selector in the catalog")
	}
	set := append([][2]string{{r.selector, "lorenz"}, {"auto-rotate", "0"},
		{"rotation-controls-x", "0"}, {"rotation-controls-y", "0"}, {"rotation-controls-z", "0"}}, kv...)
	for _, c := range set {
		if err := r.Set(c[0], c[1]); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

// frameW is how wide a test frame is.
const frameW = 120

func frame(t *testing.T, r *Rack) []byte {
	t.Helper()
	m, err := r.Scene(frameW, 60, 2)
	if err != nil {
		t.Fatal(err)
	}
	return m.Px
}

// lit is how many pixels of a frame are drawn, and the mean x of them.
func lit(px []byte) (n int, meanX float64) {
	for i := 0; i+2 < len(px); i += 3 {
		if px[i] != sceneBack.R || px[i+1] != sceneBack.G || px[i+2] != sceneBack.B {
			n++
			meanX += float64((i / 3) % frameW)
		}
	}
	if n > 0 {
		meanX /= float64(n)
	}
	return n, meanX
}

// A flow runs on from frame to frame, as the page's does, and Pause holds it.
func TestTheFlowRunsAndPauseHoldsIt(t *testing.T) {
	r := still(t)
	a, b := frame(t, r), frame(t, r)
	if string(a) == string(b) {
		t.Error("two frames of lorenz are the same: the flow is not running")
	}
	if err := r.Set("pause-sw", "1"); err != nil {
		t.Fatal(err)
	}
	c, d := frame(t, r), frame(t, r)
	if string(c) != string(d) {
		t.Error("paused, the frames still change")
	}
}

// Trail is how long a run each frame draws.
func TestTrailIsTheRunsLength(t *testing.T) {
	r := still(t, [2]string{"trail-slider", "5000"})
	frame(t, r)
	if got := len(r.scene.trail.pts); got != 5000 {
		t.Errorf("a Trail of 5000 drew %d points", got)
	}
	if err := r.Set("trail-slider", "500000"); err != nil {
		t.Fatal(err)
	}
	frame(t, r)
	if got := len(r.scene.trail.pts); got != trailBudget {
		t.Errorf("a Trail past the budget drew %d points, want the budget's %d", got, trailBudget)
	}
}

// View's X slides the model right, and Display's Line draws it thicker.
func TestPanAndLineMoveAndThickenTheModel(t *testing.T) {
	r := still(t, [2]string{"pause-sw", "1"})
	n0, x0 := lit(frame(t, r))
	if err := r.Set("pan-x", "8"); err != nil {
		t.Fatal(err)
	}
	_, x1 := lit(frame(t, r))
	if x1 <= x0+4 {
		t.Errorf("pan X 8 left the model at x %.1f, from %.1f", x1, x0)
	}
	if err := r.Set("line-width", "10"); err != nil {
		t.Fatal(err)
	}
	if n1, _ := lit(frame(t, r)); n1 <= n0 {
		t.Errorf("Line 10 lit %d pixels, no more than Line 1's %d", n1, n0)
	}
}

// Persist keeps the old trails: a rotating model leaves its earlier poses.
func TestPersistKeepsTheOldTrails(t *testing.T) {
	r := still(t, [2]string{"trail-slider", "2000"})
	n0, _ := lit(frame(t, r))
	if err := r.Set("persist-trail", "1"); err != nil {
		t.Fatal(err)
	}
	var n int
	for range 10 {
		n, _ = lit(frame(t, r))
	}
	if n <= n0 {
		t.Errorf("after ten persisted frames %d pixels are lit, no more than one frame's %d", n, n0)
	}
}

// Turning a flow's constant while the model draws is safe: the integrator
// reads the constant the setter writes.
func TestAConstantSetWhileDrawingDoesNotRace(t *testing.T) {
	r := still(t, [2]string{"trail-slider", "2000"})
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 20 {
			frame(t, r)
		}
	})
	for i := range 20 {
		if err := r.Set("lorenz-r", []string{"28", "30"}[i%2]); err != nil {
			t.Error(err)
		}
	}
	wg.Wait()
}

// A frame of lorenz at a large terminal's size, at the page's Trail.
func BenchmarkScene(b *testing.B) {
	r, err := New()
	if err != nil {
		b.Fatal(err)
	}
	if err := r.Set(r.selector, "lorenz"); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if _, err := r.Scene(240, 120, 2); err != nil {
			b.Fatal(err)
		}
	}
}

// With Timing's TBC on, the pose turns by the time a frame took; off, by a
// sixtieth of a second a frame, however long it took.
func TestTBCTurnsByTimeOrByFrame(t *testing.T) {
	r := newRack(t)
	set(t, r, "auto-rotate", "1")
	turn := func(frames int, gap time.Duration) float64 {
		r.scene.mu.Lock()
		a := r.scene.angle[1]
		r.scene.mu.Unlock()
		for range frames {
			time.Sleep(gap)
			if _, err := r.Scene(40, 20, 2); err != nil {
				t.Fatal(err)
			}
		}
		r.scene.mu.Lock()
		defer r.scene.mu.Unlock()
		return r.scene.angle[1] - a
	}
	turn(1, 0)
	on := turn(5, 40*time.Millisecond) // about 0.2 s
	set(t, r, "tbc-sw", "0")
	turn(1, 0)
	off := turn(5, 40*time.Millisecond) // five frames: a twelfth of a second's turn
	if math.Abs(off-5*autoSpin/60) > 1e-9 {
		t.Errorf("TBC off: five frames turned %.4f, want %.4f", off, 5*autoSpin/60)
	}
	if on < 1.5*off {
		t.Errorf("TBC on: 0.2 s turned %.4f, no more than five bare frames' %.4f", on, off)
	}
}
