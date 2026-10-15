package rackhost

import "testing"

// inkCount is how many of the picture's items are inked in color c.
func inkCount(r *Rack, c int) int {
	n := 0
	for _, it := range r.pic.Items {
		if it != nil && it.Ink != nil && *it.Ink == c {
			n++
		}
	}
	return n
}

// The Style module's LED color redraws every readout, and a bank put up
// after is drawn in it too.
func TestTheLEDColorRedrawsTheReadouts(t *testing.T) {
	r := newRack(t)
	red, green := hexColor("#ff3b30"), hexColor("#35e06a")
	r.mu.Lock()
	before := inkCount(r, red)
	r.mu.Unlock()
	if before < 20 {
		t.Fatalf("only %d readouts are red to begin with", before)
	}
	set(t, r, "led-color", "green")
	set(t, r, r.selector, "rossler")
	r.mu.Lock()
	defer r.mu.Unlock()
	if n := inkCount(r, red); n != 0 {
		t.Errorf("%d readouts are still red", n)
	}
	if n := inkCount(r, green); n < before/2 {
		t.Errorf("%d readouts are green, of %d that were red", n, before)
	}
}

// With a phosphor chosen the model is traced on its tube, in its color, and
// the scopes' beams are that phosphor; with none, neither is.
func TestAPhosphorTracesTheModelOnItsTube(t *testing.T) {
	r, c := idleRack(t, "lorenz")
	set(t, r, "phosphor", "4") // P3 amber
	watch(t, r, c, 0.2)
	m, err := r.Scene(120, 80, 2)
	if err != nil {
		t.Fatal(err)
	}
	amber, other := 0, 0
	for i := 0; i+2 < len(m.Px); i += 3 {
		p := m.Px[i : i+3]
		if max(p[0], p[1], p[2]) < 60 {
			continue // the glass
		}
		if p[0] >= p[1] && p[1] >= p[2] {
			amber++
		} else {
			other++
		}
	}
	if amber < 50 || other > amber/20 {
		t.Errorf("on P3 amber: %d amber pixels, %d of other colors", amber, other)
	}
	set(t, r, "gen-x-wave", "0", "mix-rl-g1", "1")
	looks(t, r, c, 0.3)
	i := canvasOf(t, r, panelCanvas{kind: canvasScope})
	s := screen(t, r, i, 200, 140)
	lit := 0
	for y := range s.H {
		for x := range s.W {
			if p := s.At(x, y); p.R > 100 && p.R > p.G && p.G > 2*p.B { // amber, at the trace's alpha
				lit++
			}
		}
	}
	if lit < 50 {
		t.Errorf("the scope's beam is not amber: %d pixels", lit)
	}
}
