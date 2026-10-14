package rackhost

import (
	"math"
	"math/rand/v2"
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

// The transfer function measures CAP R against CAP L: noise at half the
// level and a millisecond late is a flat -6 dB and a 1 ms delay.
func TestTheTransferFunctionMeasuresTheCapture(t *testing.T) {
	f := &fakeCapture{}
	withCapture(t, f)
	r, c := idleRack(t, "xfer")
	set(t, r, "xf-show", "1") // magnitude only
	look(t, r)
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // a test's noise, the same every run
	const delay = 48
	hist := make([]float32, delay)
	for range 60 {
		const n = sampleRate / 20
		p := make([]float32, 2*n)
		for i := range n {
			x := float32(rng.Float64() - 0.5)
			p[2*i], p[2*i+1] = x, 0.5*hist[0]
			hist = append(hist[1:], x)
		}
		_ = f.write(p) //nolint:errcheck // the fake capture's writer never fails
		watch(t, r, c, 0.05)
	}
	r.mu.Lock()
	lines := r.analyzerPlot("xfer", 400)[0].v
	ms, ok := r.analyzers.xfer.DelayMS()
	r.mu.Unlock()
	if len(lines) < 20 {
		t.Fatalf("the magnitude curve is %d segments", len(lines)/2)
	}
	// -6 dB on a ±40 dB lane from -0.85 to 0.85. Over the lower half of the
	// bands: above it a band is wide enough that the delay turns its phase
	// within it, and its average droops, as it does on the page.
	want := -0.85 + 1.7*(40-6.02)/80
	for _, v := range lines {
		if v.X > 0 {
			continue
		}
		if math.Abs(float64(v.Y)-want) > 0.02 {
			t.Fatalf("the magnitude reaches %.3f; -6 dB is %.3f", v.Y, want)
		}
	}
	if !ok || math.Abs(ms-1) > 0.05 {
		t.Errorf("the delay reads %.3f ms (%v), want 1", ms, ok)
	}
}
