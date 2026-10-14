package rackhost

import (
	"testing"
)

// The Palette's color knobs dial the gradient's swatches: the start knob at
// hue 120 is green, and its Level at 0 is black.
func TestTheColorKnobsDialTheSwatches(t *testing.T) {
	r := newRack(t)
	set(t, r, "ck-color-base-hue", "120")
	r.mu.Lock()
	g := r.look().gradient
	r.mu.Unlock()
	if g.Base != [3]float32{0, 1, 0} || g.Mid != [3]float32{0, 1, 0} || g.Top != [3]float32{0, 0, 1} {
		t.Errorf("start at hue 120 makes the swatches %v %v %v", g.Base, g.Mid, g.Top)
	}
	set(t, r, "ck-color-base-lvl", "0")
	r.mu.Lock()
	g = r.look().gradient
	r.mu.Unlock()
	if g.Base != [3]float32{} {
		t.Errorf("Level 0 makes the start %v, not black", g.Base)
	}
}

// The bg knob tints what shows only the background, and leaves the model.
func TestTheBackgroundKnobTintsTheEmptyScene(t *testing.T) {
	r, c := idleRack(t, "globe")
	watch(t, r, c, 0.05)
	plain, err := r.Scene(60, 40, 1)
	if err != nil {
		t.Fatal(err)
	}
	set(t, r, "ck-color-bg-hue", "240", "ck-color-bg-lvl", "20")
	tinted, err := r.Scene(60, 40, 1)
	if err != nil {
		t.Fatal(err)
	}
	if p := tinted.Px[0:3]; !(p[2] > plain.Px[2]+50) || p[0] != plain.Px[0] {
		t.Errorf("the corner is %v tinted, %v plain: want only more blue", p, plain.Px[0:3])
	}
}

// pressMark clicks the middle of the part marked m.
func pressMark(t *testing.T, r *Rack, m string) {
	t.Helper()
	r.mu.Lock()
	var x, y float64
	found := false
	for _, i := range r.analyzers.ro.btns {
		if it := r.pic.Items[i]; it.Mark == m {
			x, y, found = it.X+it.W/2, it.Y+it.H/2, true
		}
	}
	r.mu.Unlock()
	if !found {
		t.Fatalf("no %q on the bank", m)
	}
	act(t, r, x, y, "click")
}

// Takens' ↻ measures τ again, over a τ somebody set, as the page's does.
func TestTakensRemeasureButton(t *testing.T) {
	r, c := toneRack(t, "takens")
	watch(t, r, c, 0.5)
	measured := r.value("takens-tau")
	set(t, r, "takens-tau", "5")
	watch(t, r, c, 0.2)
	if r.value("takens-tau") != "5" {
		t.Fatal("the one-shot measured over a chosen τ")
	}
	pressMark(t, r, "btn takens-remeasure")
	if got := r.value("takens-tau"); got != measured {
		t.Errorf("↻ turned τ to %s; the tone measures %s", got, measured)
	}
}

// The Lyapunov ↻ measures the model again.
func TestTheLyapunovRemeasureButton(t *testing.T) {
	r, c := idleRack(t, "lorenz")
	watch(t, r, c, 0.3)
	val := shows(t, r, "lyap-led")
	pressMark(t, r, "btn lyap-remeasure")
	r.mu.Lock()
	again := r.analyzers.ro.lyap.val
	r.mu.Unlock()
	if again != "measuring…" {
		t.Errorf("↻ left the readout at %q", again)
	}
	watch(t, r, c, 0.1)
	if got := shows(t, r, "lyap-led"); got != val {
		t.Errorf("measured again, Lorenz reads %q, before %q", got, val)
	}
}
