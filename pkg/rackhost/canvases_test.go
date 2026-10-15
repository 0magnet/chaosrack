package rackhost

import (
	"image"
	"testing"

	"github.com/0magnet/chaosrack/pkg/rackpic"
)

// canvasOf is the index of the panel's canvas of kind k (and scope n).
func canvasOf(t *testing.T, r *Rack, k panelCanvas) int {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, c := range r.canvasesOf() {
		if c == k {
			return i
		}
	}
	t.Fatalf("no canvas %+v", k)
	return -1
}

// screen is canvas i drawn w by h.
func screen(t *testing.T, r *Rack, i, w, h int) *rackpic.Image {
	t.Helper()
	got, err := r.Canvases(0, []rackpic.CanvasWant{{Index: i, W: w, H: h}})
	if err != nil || got[i] == nil {
		t.Fatalf("canvas %d: %v %v", i, got, err)
	}
	return got[i]
}

// beamLit is how many of m's pixels are the beam's green, and how far they
// spread across and down.
func beamLit(m *rackpic.Image) (n int, span image.Rectangle) {
	span = image.Rect(m.W, m.H, 0, 0)
	for y := range m.H {
		for x := range m.W {
			c := m.At(x, y)
			if c.G > 150 && c.R < 120 && c.B < 140 {
				n++
				span = span.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return n, span
}

// The panel's six screens are told apart: the model's monitor, the Record
// module's and the four scopes'.
func TestThePanelsScreensAreToldApart(t *testing.T) {
	r := newRack(t)
	r.mu.Lock()
	kinds := r.canvasesOf()
	r.mu.Unlock()
	count := map[panelCanvas]int{}
	for _, k := range kinds {
		count[k]++
	}
	want := map[panelCanvas]int{{kind: canvasMonitor}: 1, {kind: canvasRecord}: 1}
	for n := range scopeCount {
		want[panelCanvas{kind: canvasScope, n: n}] = 1
	}
	for k, n := range want {
		if count[k] != n {
			t.Errorf("%+v: %d screens, want %d (all: %v)", k, count[k], n, kinds)
		}
	}
}

// A scope draws its input: a generator in the rack's signal sweeps a trace
// across the tube; on X-Y against itself it is a diagonal; and with SCALE
// ILLUM at OFF the tube is dark.
func TestAScopeDrawsTheRacksSignal(t *testing.T) {
	r, c := idleRack(t, "lorenz")
	set(t, r, "gen-x-wave", "0", "gen-x-lvl", "80", "mix-rl-g1", "1", "mix-rr-g1", "1")
	looks(t, r, c, 0.3)
	i := canvasOf(t, r, panelCanvas{kind: canvasScope})
	n, span := beamLit(screen(t, r, i, 200, 140))
	if n < 100 || span.Dx() < 150 {
		t.Fatalf("a sine on RACK L lit %d pixels across %d of 200", n, span.Dx())
	}
	set(t, r, "scope-chan", "3", "scope-in2", "0")
	for range 8 { // the sweep's afterglow fades out under the figure
		screen(t, r, i, 200, 140)
	}
	_, diag := beamLit(screen(t, r, i, 200, 140))
	if diag.Dx() < 20 || diag.Dy() < 20 {
		t.Errorf("RACK L against itself on X-Y spans %v, want a diagonal", diag)
	}
	set(t, r, "scope-illum", "-0.2")
	if n, _ := beamLit(screen(t, r, i, 200, 140)); n != 0 {
		t.Errorf("a scope switched off lit %d pixels", n)
	}
}

// The model's monitor shows the model as last drawn, and stands by with its
// switch down.
func TestTheMonitorShowsTheModel(t *testing.T) {
	r, c := idleRack(t, "lorenz")
	watch(t, r, c, 0.2)
	i := canvasOf(t, r, panelCanvas{kind: canvasMonitor})
	lit := func() int {
		m := screen(t, r, i, 160, 120)
		n := 0
		for y := range m.H - 16 { // above the caption
			for x := range m.W {
				if p := m.At(x, y); max(p.R, p.G, p.B) > 120 { // brighter than the STANDBY lettering
					n++
				}
			}
		}
		return n
	}
	if n := lit(); n < 50 {
		t.Errorf("the monitor shows %d lit pixels of Lorenz", n)
	}
	set(t, r, "bay-visual-0-mon-on", "0")
	if n := lit(); n > 40 {
		t.Errorf("standing by, the monitor shows %d lit pixels", n)
	}
}

// The monitor's caption is the model's name, without the selector's padding.
func TestTheMonitorCaptionIsTheModelsName(t *testing.T) {
	r, c := idleRack(t, "lorenz")
	watch(t, r, c, 0.1)
	r.mu.Lock()
	m := r.monitorFrame(300, 200, true, "")
	r.mu.Unlock()
	// The caption's band is as wide as its text: "Lorenz" and a margin.
	band := 0
	for x := range 300 {
		if p := m.RGBAAt(x, 199); p.R == 0 && p.G == 0 && p.B == 0 || p == (scopeDark) {
			continue
		}
		band = x
	}
	if w := 6*7 + 10; band > w+2 {
		t.Errorf("the caption runs to x=%d, wider than \"Lorenz\" (%d)", band, w)
	}
}
