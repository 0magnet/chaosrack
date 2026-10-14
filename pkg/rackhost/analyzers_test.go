package rackhost

import (
	"testing"

	"github.com/0magnet/chaosrack/pkg/acoustics"
)

// watch moves the clock on secs, the panel drawing the scene every 50 ms as
// it looks.
func watch(t *testing.T, r *Rack, c *clock, secs float64) {
	t.Helper()
	for range int(secs / 0.05) {
		looks(t, r, c, 0.05)
		if _, err := r.Scene(80, 40, 1); err != nil {
			t.Fatal(err)
		}
	}
}

// rtaBars is the RTA's bars now: each band's top, left to right.
func rtaBars(t *testing.T, r *Rack) []float32 {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	passes := r.analyzerPlot("rta", 400)
	if len(passes) != 2 {
		t.Fatalf("the RTA drew %d passes", len(passes))
	}
	var tops []float32
	for i := 1; i < len(passes[0].v); i += 2 {
		tops = append(tops, passes[0].v[i].Y)
	}
	return tops
}

// The RTA measures the rack's signal: a tone's band stands tallest, and TOP
// lifts every bar.
func TestTheRTAMeasuresTheRacksSignal(t *testing.T) {
	r, c := toneRack(t, "rta")
	watch(t, r, c, 1)
	tops := rtaBars(t, r)
	bands := acoustics.RTABands(3)
	if len(tops) != len(bands) {
		t.Fatalf("%d bars for %d third-octave bands", len(tops), len(bands))
	}
	hi := 0
	for i := range tops {
		if tops[i] > tops[hi] {
			hi = i
		}
	}
	l, _ := r.sound.window(sampleRate / 2)
	hz, _ := pitch(l)
	if b := bands[hi]; hz < b.Lo || hz > b.Hi {
		t.Errorf("the tallest bar is %.0f–%.0f Hz; the tone is %.0f Hz", b.Lo, b.Hi, hz)
	}
	set(t, r, "rta-top", "-30")
	watch(t, r, c, 1)
	if lifted := rtaBars(t, r); lifted[hi] <= tops[hi] {
		t.Errorf("TOP at -30 dBFS left the tone's bar at %.2f, from %.2f", lifted[hi], tops[hi])
	}
	img, err := r.Scene(200, 100, 1)
	if err != nil {
		t.Fatal(err)
	}
	green := 0
	for i := 0; i+2 < len(img.Px); i += 3 {
		if near(img.Px[i:i+3], [3]uint8{102, 255, 115}) {
			green++
		}
	}
	if green < 20 {
		t.Errorf("the RTA's scene shows %d pixels of its green", green)
	}
}

// An analyzer listens to the capture as an audio model does.
func TestAnAnalyzerOpensTheCapture(t *testing.T) {
	f := &fakeCapture{}
	withCapture(t, f)
	r, _ := idleRack(t, "rta")
	look(t, r)
	if f.write == nil {
		t.Error("choosing the RTA did not open the capture")
	}
}
