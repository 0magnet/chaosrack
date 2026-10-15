package rackhost

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/rackpic"
)

// shownFigure is the figure the scene drew last.
func shownFigure(t *testing.T, r *Rack, c *clock, secs float64) [][3]float64 {
	t.Helper()
	watch(t, r, c, secs)
	r.scene.mu.Lock()
	defer r.scene.mu.Unlock()
	return append([][3]float64(nil), r.scene.fig.Points...)
}

// The Graphic Artist drifts as it runs, and its harmonics reshape it.
func TestTheGraphicArtistDriftsAndTakesItsKnobs(t *testing.T) {
	r, c := idleRack(t, "graphicartist")
	a := shownFigure(t, r, c, 0.1)
	b := shownFigure(t, r, c, 0.5)
	if len(a) == 0 || a[0] == b[0] {
		t.Fatal("the Graphic Artist did not drift")
	}
	set(t, r, "pause-sw", "1")
	held := shownFigure(t, r, c, 0.1)
	set(t, r, "ga-hc", "5")
	reshaped := shownFigure(t, r, c, 0.1)
	if held[len(held)/3] == reshaped[len(reshaped)/3] {
		t.Error("HmC at 5 drew the same wireframe")
	}
}

// The scope clock draws a face, flat on the screen.
func TestTheScopeClockDrawsAFace(t *testing.T) {
	r, c := idleRack(t, "scopeclock")
	pts := shownFigure(t, r, c, 0.1)
	if len(pts) < 100 {
		t.Fatalf("the clock drew %d points", len(pts))
	}
	for _, p := range pts {
		if p[2] != 0 {
			t.Fatalf("the clock's face is not flat: %v", p)
		}
	}
}

// Fourier Text draws its banner a stroke at a time, never joining one stroke
// to the next, and its harmonics knob changes the glyphs.
func TestFourierTextDrawsItsBanner(t *testing.T) {
	r, c := idleRack(t, "scopetext")
	watch(t, r, c, 0.1)
	r.scene.mu.Lock()
	n, edges := len(r.scene.fig.Points), len(r.scene.fig.Edges)/2
	first := r.scene.fig.Points[n/2]
	r.scene.mu.Unlock()
	if n < 1000 || edges == 0 || edges >= n-1 {
		t.Fatalf("the banner is %d points joined by %d lines: want strokes apart", n, edges)
	}
	set(t, r, "stext-harm", "4")
	watch(t, r, c, 0.1)
	r.scene.mu.Lock()
	again := r.scene.fig.Points[len(r.scene.fig.Points)/2]
	r.scene.mu.Unlock()
	if again == first {
		t.Error("4 harmonics drew the same glyphs as 24")
	}
}

// The bifurcation explorer sweeps the last flow shown: its diagram grows as
// it runs, across the swept parameter's range.
func TestTheBifurcationDiagramGrows(t *testing.T) {
	r, c := idleRack(t, "lorenz")
	watch(t, r, c, 0.1)
	set(t, r, r.selector, "bifurcation")
	early := shownFigure(t, r, c, 0.2)
	later := shownFigure(t, r, c, 2)
	if len(later) <= len(early) || len(later) < 1000 {
		t.Fatalf("the diagram has %d points after 0.2 s and %d after 2 s", len(early), len(later))
	}
	lo, hi := later[0][0], later[0][0]
	for _, p := range later {
		lo, hi = min(lo, p[0]), max(hi, p[0])
	}
	if hi-lo < 10 {
		t.Errorf("the diagram spans %.1f..%.1f across: the sweep did not move", lo, hi)
	}
}

// The Poincaré model is the last flow's crossings of its plane: in place,
// all at the plane's z; face on, flat; as a return map, with its guide.
func TestThePoincareSection(t *testing.T) {
	r, c := idleRack(t, "lorenz")
	watch(t, r, c, 0.1)
	set(t, r, r.selector, "poincare")
	pts := shownFigure(t, r, c, 1)
	if len(pts) < 100 {
		t.Fatalf("the section has %d crossings", len(pts))
	}
	for _, p := range pts {
		if math.Abs(p[2]-pts[0][2]) > 1e-3 {
			t.Fatalf("a crossing at z %.3f, another at %.3f: not one plane", p[2], pts[0][2])
		}
	}
	set(t, r, "sect-view", "1")
	for _, p := range shownFigure(t, r, c, 0.1) {
		if p[2] != 0 {
			t.Fatalf("face on, a crossing is off the plane: %v", p)
		}
	}
	set(t, r, "sect-view", "2")
	if m := shownFigure(t, r, c, 0.1); len(m) < 64 {
		t.Errorf("the return map is %d points", len(m))
	}
}

// LINE S draws the section over a running flow, in gold.
func TestLineSOverlaysTheSection(t *testing.T) {
	r, c := idleRack(t, "lorenz")
	set(t, r, "sect-sw", "1")
	watch(t, r, c, 1)
	img, err := r.Scene(160, 100, 1)
	if err != nil {
		t.Fatal(err)
	}
	gold := 0
	for i := 0; i+2 < len(img.Px); i += 3 {
		if near(img.Px[i:i+3], [3]uint8{255, 204, 38}) {
			gold++
		}
	}
	if gold < 10 {
		t.Errorf("LINE S drew %d gold pixels", gold)
	}
}

// SWEEP is the last flow's parameters: the diagram sweeps the one it is on.
func TestTheBifurcationSweepsWhatSweepPicks(t *testing.T) {
	r, c := idleRack(t, "lorenz")
	watch(t, r, c, 0.1)
	set(t, r, r.selector, "bifurcation")
	watch(t, r, c, 0.1)
	r.mu.Lock()
	sw := r.ctls[r.byID["bif-sweep"]]
	r.mu.Unlock()
	if len(sw.Labels) != 4 || sw.Labels[2] != "ρ" {
		t.Fatalf("SWEEP offers %v on Lorenz", sw.Labels)
	}
	set(t, r, "bif-sweep", "2")
	pts := shownFigure(t, r, c, 1)
	if len(pts) < 100 {
		t.Errorf("sweeping ρ drew %d points", len(pts))
	}
}

// The Turtle walks natively: its figure is drawn with the tinter's colors,
// and CYCLE steps the modulus on, turning the MOD knob.
func TestTheTurtleWalks(t *testing.T) {
	r, c := idleRack(t, "turtle")
	watch(t, r, c, 0.5)
	r.scene.mu.Lock()
	n, ages := len(r.scene.fig.Points), append([]float32(nil), r.scene.fig.Ages...)
	r.scene.mu.Unlock()
	if n < 20 || len(ages) != n {
		t.Fatalf("the Turtle drew %d points with %d tints", n, len(ages))
	}
	tints := map[float32]bool{}
	for _, a := range ages {
		tints[a] = true
	}
	if len(tints) < 2 {
		t.Errorf("the walk is one tint: %v", ages[:3])
	}
	set(t, r, "turtle-cycle", "1")
	watch(t, r, c, 2.2)
	if got := r.value("turtle-mod"); got == "25" {
		t.Error("CYCLE did not step the modulus")
	}
}

// Custom draws its typed system natively: the default template, its
// constants as knobs, and the flavors its switches pick.
func TestCustomDrawsItsSystem(t *testing.T) {
	r, c := idleRack(t, "custom")
	figure := func() attractor.Figure {
		frame(t, r)
		r.scene.mu.Lock()
		defer r.scene.mu.Unlock()
		if r.scene.err != nil {
			t.Fatalf("custom: %v", r.scene.err)
		}
		f := r.scene.fig
		f.Points = append([][3]float64(nil), f.Points...)
		return f
	}
	watch(t, r, c, 0.1)
	a := figure()
	if a.Kind != attractor.FigurePath || len(a.Points) < 100 {
		t.Fatalf("the template drew %d points as kind %d", len(a.Points), a.Kind)
	}
	set(t, r, "custom-rho", "15")
	if b := figure(); b.Points[len(b.Points)/2] == a.Points[len(a.Points)/2] {
		t.Error("ρ at 15 drew the same system")
	}
	set(t, r, "model-sw-0", "1") // iter
	if f := figure(); f.Kind != attractor.FigurePoints {
		t.Errorf("iter drew kind %d, not points", f.Kind)
	}
	set(t, r, "model-sw-1", "1") // surf
	if f := figure(); f.Kind != attractor.FigureLines || len(f.Edges) < 100 || r.value("model-sw-0") != "0" {
		t.Errorf("surf drew kind %d with %d lines, iter %s", f.Kind, len(f.Edges)/2, r.value("model-sw-0"))
	}
}

// The Lattice runs its programs in terminals natively: every lit cell of
// their screens is a point in its own color, STACKS keeps the families it
// names, and the shape knob is what they draw.
func TestTheLatticeDrawsItsTerminals(t *testing.T) {
	r, c := idleRack(t, "lattice")
	figure := func() attractor.Figure {
		watch(t, r, c, 0.3)
		r.scene.mu.Lock()
		defer r.scene.mu.Unlock()
		f := r.scene.fig
		f.Points = append([][3]float64(nil), f.Points...)
		return f
	}
	all := figure()
	if len(all.Points) < 100 || len(all.Colors) != len(all.Points) {
		t.Fatalf("the lattice drew %d points, %d colored", len(all.Points), len(all.Colors))
	}
	set(t, r, "lattice-stacks", "3") // z alone
	if z := figure(); len(z.Points) >= len(all.Points) {
		t.Errorf("one family drew %d points, three %d", len(z.Points), len(all.Points))
	}
	set(t, r, "lattice-stacks", "0", "lattice-shape", "0")
	if s := figure(); len(s.Points) == len(all.Points) && s.Points[0] == all.Points[0] {
		t.Error("another shape drew the same lattice")
	}
}

// ROWS's L puts the model chosen in the Lattice: Lorenz's trail is handed to
// its programs as a figure, and what is drawn is the Lattice's cells.
func TestLPutsTheModelInTheLattice(t *testing.T) {
	r, c := idleRack(t, "lorenz")
	watch(t, r, c, 0.2)
	plain, _ := lit(frame(t, r))
	set(t, r, "lat-on", "1")
	deadline := time.Now().Add(5 * time.Second)
	for {
		watch(t, r, c, 0.1)
		time.Sleep(20 * time.Millisecond)
		r.mu.Lock()
		ext := r.latFig.ext
		r.mu.Unlock()
		n, _ := lit(frame(t, r))
		if ext > 0 && n > 0 && n != plain {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the Lattice never drew Lorenz (scale %.2f, %d lit, %d plain)", ext, n, plain)
		}
	}
}

// DRIVE on audio points a gold cursor at the diagram where the rack's
// loudness puts the swept parameter, and its readout names the value; on
// sweep there is none.
func TestTheBifurcationsAudioCursor(t *testing.T) {
	r, c := idleRack(t, "bifurcation")
	set(t, r, "gen-x-wave", "0", "gen-x-lvl", "80", "mix-rl-g1", "1", "mix-rr-g1", "1", "bif-drive", "1")
	looks(t, r, c, 0.3)
	watch(t, r, c, 1)
	r.mu.Lock()
	h := r.hear("bifurcation", true, 80)
	cur := r.analyzers.bif.cur
	r.mu.Unlock()
	if len(h.overlay.Points) < 96 {
		t.Errorf("the cursor is %d points, want its rule at least", len(h.overlay.Points))
	}
	if f := strings.Fields(cur); len(f) != 2 {
		t.Errorf("the cursor's readout is %q, want a parameter and its value", cur)
	} else if _, err := strconv.ParseFloat(f[1], 64); err != nil {
		t.Errorf("the cursor's readout is %q", cur)
	}
	set(t, r, "bif-drive", "0")
	r.mu.Lock()
	h = r.hear("bifurcation", true, 80)
	cur = r.analyzers.bif.cur
	r.mu.Unlock()
	if len(h.overlay.Points) != 0 || cur != "sweep" {
		t.Errorf("on sweep: a cursor of %d points, reading %q", len(h.overlay.Points), cur)
	}
}

// With the Console's Physics up the Turtle has weight: it sits lower than the
// same walk without, and the room's walls — the picture's edges — hold it
// whenever it fits in the room (a walk that has outgrown it is centered, as
// the page centers it).
func TestTheTurtleHasWeight(t *testing.T) {
	height := func(phys string) float64 {
		r, c := idleRack(t, "turtle")
		set(t, r, "phys-sw", phys)
		sum, n := 0.0, 0
		for i := range 40 {
			pts := shownFigure(t, r, c, 0.1)
			lo, hi, mean := math.Inf(1), math.Inf(-1), 0.0
			for _, p := range pts {
				lo, hi, mean = min(lo, p[1]), max(hi, p[1]), mean+p[1]/float64(len(pts))
			}
			if phys == "1" && hi-lo <= 2*turtleRoom && (lo < -turtleRoom*1.05 || hi > turtleRoom*1.05) {
				t.Fatalf("the figure, %.3f to %.3f, fits the room's ±%.3f but is outside it", lo, hi, turtleRoom)
			}
			if i >= 15 { // once the walk's scale has settled
				sum, n = sum+mean, n+1
			}
		}
		return sum / float64(n)
	}
	if with, without := height("1"), height("0"); with > without-0.08 {
		t.Errorf("with weight the figure sits at %.3f on average, without at %.3f", with, without)
	}
}

// The globe's LAT buttons set its parallels as the page's do: a click on −
// winds one spiral the other way round, on 0 they are rings again.
func TestTheGlobesLatButtonsSetItsParallels(t *testing.T) {
	r, _ := idleRack(t, "globe")
	click := func(mark string) {
		t.Helper()
		r.mu.Lock()
		var it *rackpic.Item
		for _, i := range r.analyzers.ro.btns {
			if r.pic.Items[i].Mark == mark {
				it = r.pic.Items[i]
			}
		}
		r.mu.Unlock()
		if it == nil {
			t.Fatalf("no button marked %q in the globe's bank", mark)
		}
		act(t, r, it.X+it.W/2, it.Y+it.H/2, "click")
	}
	click("set globe-par=1 globe-rev=1")
	r.mu.Lock()
	par, rev := r.value("globe-par"), r.value("globe-rev")
	r.mu.Unlock()
	if par != "1" || rev != "1" {
		t.Errorf("after −: par %q rev %q, want a spiral the other way", par, rev)
	}
	click("set globe-par=0")
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.value("globe-par") != "0" {
		t.Errorf("after 0: par %q, want rings", r.value("globe-par"))
	}
}
