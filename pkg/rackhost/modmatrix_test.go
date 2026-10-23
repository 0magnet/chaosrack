package rackhost

import (
	"bytes"
	"math"
	"testing"
	"time"

	"github.com/0magnet/chaosrack/pkg/attractor"
)

// clickMark clicks the middle of the part marked mark, and reports where it
// was.
func clickMark(t *testing.T, r *Rack, mark string, yFrac float64) {
	t.Helper()
	p, err := r.Picture()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range p.Items {
		if it != nil && it.Mark == mark {
			if err := r.Act(0, it.X+it.W/2, it.Y+it.H*yFrac, "click", 0); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("no part marked %q", mark)
}

// The matrix's rows are the model's parameters, then the view's; DEST lists
// them by name.
func TestModRowsFollowTheModel(t *testing.T) {
	r := newRack(t)
	set(t, r, r.selector, "torus")
	r.mu.Lock()
	defer r.mu.Unlock()
	mv := &r.mod
	if mv.nParams == 0 || len(mv.rows) != mv.nParams+len(attractor.ModViewRows) {
		t.Fatalf("%d rows, %d of them the model's", len(mv.rows), mv.nParams)
	}
	want := []string{"torus-R", "torus-r", "torus-stacks", "torus-slices", "torus-roll"}
	for i, id := range want {
		if mv.rows[i] != id {
			t.Errorf("row %d is %q, want %q", i, mv.rows[i], id)
		}
	}
	if mv.rows[len(want)] != "" || mv.rows[mv.nParams] != "view-spinx" {
		t.Errorf("rows %d and %d are %q and %q", len(want), mv.nParams, mv.rows[len(want)], mv.rows[mv.nParams])
	}
	if got := r.pic.Items[mv.legend[1]].Text; got != "r" {
		t.Errorf("row 1 is named %q", got)
	}
	if c := r.ctls[r.byID["modmx-dest"]]; len(c.Options) != len(want)+len(attractor.ModViewRows) {
		t.Errorf("DEST lists %v", c.Labels)
	}
}

// SHAPE sets the selected route's shape, and reads each route's own as the
// selection moves.
func TestModShapeIsTheSelectedRoutes(t *testing.T) {
	r := newRack(t)
	set(t, r, r.selector, "torus")
	clickMark(t, r, "btn mxpin-1-0", 0.5) // torus-r, selected
	set(t, r, "modmx-shape", attractor.ModShapeSwing)
	clickMark(t, r, "btn mxpin-0-0", 0.5) // torus-R, selected, still a push
	r.mu.Lock()
	sr, sR, shown := r.mod.routes["torus-r"].shape, r.mod.routes["torus-R"].shape, r.value("modmx-shape")
	r.mu.Unlock()
	if sr != attractor.ModShapeSwing || sR != attractor.ModShapePush || shown != attractor.ModShapePush {
		t.Errorf("torus-r %q, torus-R %q, SHAPE shows %q", sr, sR, shown)
	}
}

// A pin routes its column to its row, lit, selected, at the default depth;
// pressed again it takes the route away.
func TestModPinRoutes(t *testing.T) {
	r := newRack(t)
	set(t, r, r.selector, "torus")
	clickMark(t, r, "btn mxpin-1-0", 0.5) // torus-r from the rack's mono
	r.mu.Lock()
	m, sel, dp := r.mod.routes["torus-r"], r.mod.sel, r.value("modmx-depth")
	pin := r.pic.Items[r.mod.pins[1][0]-1]
	r.mu.Unlock()
	if m.channel != "mono" || m.level != modDefDepth || sel != "torus-r" || dp != "0.4" {
		t.Errorf("routed %+v, selected %q, depth %s", m, sel, dp)
	}
	if *pin.Fill == pinDark {
		t.Error("the routed pin is dark")
	}
	clickMark(t, r, "btn mxpin-1-0", 0.5)
	r.mu.Lock()
	m = r.mod.routes["torus-r"]
	pin = r.pic.Items[r.mod.pins[1][0]-1]
	r.mu.Unlock()
	if m.on() || *pin.Fill != pinDark {
		t.Errorf("pressed again, the route is %+v and the pin %06x", m, *pin.Fill)
	}
}

// A route moves its knob by its source: the figure is drawn at the routed
// value, and the knob keeps the value it was set to.
func TestModRouteMovesTheModel(t *testing.T) {
	r := newRack(t)
	set(t, r, "auto-rotate", "0", "pause-sw", "1", r.selector, "torus")
	shot := func() []byte {
		m, err := r.Scene(120, 90, 1)
		if err != nil {
			t.Fatal(err)
		}
		return m.Px
	}
	shot()
	still := shot()
	clickMark(t, r, "btn mxpin-0-0", 0.5) // torus-R from mono
	set(t, r, "modmx-depth", "1")
	r.mu.Lock()
	loud := make([]float32, attractor.ModBandCount)
	for i := range loud {
		loud[i] = 1
	}
	r.mod.bands.Store("mono", loud, 1)
	r.mu.Unlock()
	if bytes.Equal(shot(), still) {
		t.Error("the routed torus drew as it was")
	}
	r.mu.Lock()
	knob, routed := r.value("torus-R"), r.number("torus-R")
	r.mu.Unlock()
	if knob != "1.5" || routed <= 1.5 {
		t.Errorf("the knob reads %s and the model %v", knob, routed)
	}
}

// An EQ bar sets the selected route's band to where it is pressed, and the
// bar fills to it.
func TestModEQ(t *testing.T) {
	r := newRack(t)
	set(t, r, r.selector, "torus")
	clickMark(t, r, "btn mxpin-0-0", 0.5)
	clickMark(t, r, "btn mxeq-2", 0.25) // three quarters up
	r.mu.Lock()
	b := r.mod.routes["torus-R"].bands[2]
	fills := len(r.modOverlays())
	r.mu.Unlock()
	if b < 0.7 || b > 0.8 {
		t.Errorf("band 2 is %v, want 0.75", b)
	}
	if fills != 2 {
		t.Errorf("%d overlays, want the selection's rim and one band", fills)
	}
}

// DEST selects a row; DPTH sets its depth, and its pin follows.
func TestModDestAndDepth(t *testing.T) {
	r := newRack(t)
	set(t, r, r.selector, "torus")
	clickMark(t, r, "btn mxpin-2-0", 0.5) // stacks
	set(t, r, "modmx-dest", "torus-R")
	r.mu.Lock()
	sel := r.mod.sel
	r.mu.Unlock()
	if sel != "torus-R" {
		t.Fatalf("DEST selected %q", sel)
	}
	set(t, r, "modmx-dest", "torus-stacks", "modmx-depth", "-1")
	r.mu.Lock()
	m := r.mod.routes["torus-stacks"]
	pin := r.pic.Items[r.mod.pins[2][0]-1]
	r.mu.Unlock()
	if m.level != -1 || *pin.Fill != mixRGB(pinDark, pinNeg, 1) {
		t.Errorf("depth %v, pin %06x", m.level, *pin.Fill)
	}
}

// DEST, SRC and DPTH route as the pins do, for a hand on the keyboard.
func TestModRoutesFromTheKeyboard(t *testing.T) {
	r := newRack(t)
	set(t, r, r.selector, "torus", "modmx-dest", "torus-r", "modmx-src", "L")
	r.mu.Lock()
	m := r.mod.routes["torus-r"]
	pin := r.pic.Items[r.mod.pins[1][1]-1]
	r.mu.Unlock()
	if m.channel != "L" || m.level != modDefDepth || *pin.Fill == pinDark {
		t.Errorf("SRC routed %+v, pin %06x", m, *pin.Fill)
	}
	set(t, r, "modmx-dest", "torus-R")
	if v := r.value("modmx-src"); v != "" {
		t.Errorf("an unrouted row shows SRC %q", v)
	}
	set(t, r, "modmx-dest", "torus-r", "modmx-src", "")
	r.mu.Lock()
	m = r.mod.routes["torus-r"]
	r.mu.Unlock()
	if m.on() {
		t.Errorf("SRC off left %+v", m)
	}
}

// A generator turned below A0, on the rack's left, routed to pan X with the
// wave shape, swings the model either side of the knob as the X-Y scope's
// beam swings; the same route as a push only ever moves it one way.
func TestAWaveRouteFollowsASlowGeneratorBothWays(t *testing.T) {
	r := newRack(t)
	r.SetSpeaker(&fakeSpeaker{r: r})
	set(t, r, r.selector, "globe")
	set(t, r, "gen-x-wave", "0", "gen-x-out", "1", "gen-x-freq", "-60") // 0.86 Hz, left
	r.mu.Lock()
	f := r.value("gen-x-freq")
	r.mu.Unlock()
	if f != "-60" {
		t.Fatalf("Gen 1's FREQ is %s, want -60, eight octaves under A0 being -96", f)
	}
	var path []float64
	sweep := func(shape string) (lo, hi float64) {
		r.mu.Lock()
		r.mod.routes["view-panx"] = modRoute{channel: "L", level: 0.25, shape: shape}
		r.mu.Unlock()
		lo, hi = math.Inf(1), math.Inf(-1)
		frame := make([]float32, sampleRate/60)
		for range 120 { // two seconds, a frame at a time
			r.Fill(frame, make([]float32, len(frame)))
			r.mu.Lock()
			r.modulate("globe")
			v := r.mod.over["pan-x"]
			r.mu.Unlock()
			lo, hi = min(lo, v), max(hi, v)
			if shape == attractor.ModShapeWave {
				path = append(path, v)
			}
		}
		return lo, hi
	}
	if lo, hi := sweep(attractor.ModShapeWave); lo > -1 || hi < 1 {
		t.Errorf("the wave route moved pan X only over %.2f..%.2f, want both ways past ±1", lo, hi)
	}
	// Traced, not jumping: at 0.86 Hz a frame moves it a little, where a
	// tone would land anywhere in its swing frame to frame.
	// From the frame the rack's window is full: before it there is no
	// newest sample, and the route reads silence.
	for i := 4; i < len(path); i++ {
		if d := math.Abs(path[i] - path[i-1]); d > 0.5 {
			t.Fatalf("pan X jumped %.2f between frames %d and %d", d, i-1, i)
		}
	}
	if lo, _ := sweep(attractor.ModShapePush); lo < 0 {
		t.Errorf("the push route moved pan X below its setting, to %.2f", lo)
	}
}

// TestAModelSourceAdvancesOnceAFrame: each route reading a model source
// advanced its smoothing, so two routes on head x followed x twice as fast as
// one.
func TestAModelSourceAdvancesOnceAFrame(t *testing.T) {
	r := newRack(t)
	set(t, r, r.selector, "globe")
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sound.now = func() time.Time { return time.Unix(1000, 0) }
	r.mod.head, r.mod.extent = [3]float64{1, 0, 0}, 1
	r.mod.routes["view-panx"] = modRoute{channel: "mx", level: 0.25}
	r.mod.routes["view-pany"] = modRoute{channel: "mx", level: 0.25}
	r.modulate("globe")
	want := attractor.ModelModSmooth(0, attractor.ModelModNorm(1, 1), 1)
	if got := r.mod.model["mx"]; got != want {
		t.Errorf("head x after a frame read by two routes is %v, want one frame's %v", got, want)
	}
}
