package rackhost

import (
	"math"
	"testing"
	"time"
)

// fakeCapture stands in for the machine's audio: it records what it was asked
// for, and each feed hands the rack n samples of a tone on the left and its
// inverse on the right.
type fakeCapture struct {
	source  string
	write   func([]float32) error
	stopped bool
	ph      float64
}

func (f *fakeCapture) start(source string, write func([]float32) error) (func(), error) {
	f.source, f.write, f.stopped = source, write, false
	return func() { f.stopped = true }, nil
}

func (f *fakeCapture) feed(n int, hz float64) {
	p := make([]float32, 2*n)
	for i := range n {
		v := float32(0.5 * math.Sin(2*math.Pi*f.ph))
		p[2*i], p[2*i+1] = v, -v
		f.ph += hz / sampleRate
	}
	_ = f.write(p) //nolint:errcheck // the fake capture's writer never fails
}

// The tests never listen to the machine: a capture opened by a test with no
// fakeCapture of its own is silent.
func init() {
	startCapture = func(string, func([]float32) error) (func(), error) { return func() {}, nil }
}

// withCapture puts f in place of the machine's capture for one test.
func withCapture(t *testing.T, f *fakeCapture) {
	t.Helper()
	old := startCapture
	startCapture = f.start
	t.Cleanup(func() { startCapture = old })
}

// idleRack is a rack on a hand-moved clock whose speakers are never asked
// for samples, choosing model.
func idleRack(t *testing.T, model string) (*Rack, *clock) {
	t.Helper()
	r := newRack(t)
	c := &clock{t: time.Unix(1000, 0)}
	r.sound.now = c.now
	r.sound.games.now = c.now
	set(t, r, r.selector, model)
	return r, c
}

// looks moves the clock on secs, the panel looking every 50 ms.
func looks(t *testing.T, r *Rack, c *clock, secs float64) {
	t.Helper()
	for range int(secs / 0.05) {
		c.t = c.t.Add(50 * time.Millisecond)
		look(t, r)
	}
}

// By default the rack's signal is the capture, and an audio model listens to
// it only while it is chosen: chosen, the capture opens on what is playing;
// another model, and it closes.
func TestAnAudioModelListensOnlyWhileChosen(t *testing.T) {
	f := &fakeCapture{}
	withCapture(t, f)
	r, c := idleRack(t, "globe")
	look(t, r)
	if f.write != nil {
		t.Fatal("the capture opened with no audio model chosen")
	}
	set(t, r, r.selector, "xy")
	look(t, r)
	if f.write == nil || f.source != "monitor" {
		t.Fatalf("choosing XY opened the capture on %q, want monitor", f.source)
	}
	f.feed(sampleRate/2, 300)
	looks(t, r, c, 0.5)
	l, rt := r.sound.rack.window(sampleRate / 10)
	if _, p := pitch(l); math.Abs(p-0.5) > 0.01 {
		t.Errorf("RACK L peaks at %.3f; CAP L is on it at 0.5", p)
	}
	for i := range l {
		if l[i] != -rt[i] {
			t.Fatalf("RACK R is not CAP R at sample %d: %v against %v", i, rt[i], l[i])
		}
	}
	set(t, r, r.selector, "globe")
	look(t, r)
	if !f.stopped {
		t.Error("the capture stayed open after leaving XY")
	}
}

// The XY draws the rack's signal: two generators on RACK L and R a fifth
// apart make a Lissajous figure inside the scope's screen.
func TestTheXYDrawsTheRacksSignal(t *testing.T) {
	r, c := idleRack(t, "xy")
	set(t, r, "mix-rl-cl", "0", "mix-rr-cr", "0", "mix-rl-g1", "1", "mix-rr-g2", "1",
		"gen-x-lvl", "60", "gen-y-lvl", "60")
	looks(t, r, c, 0.3)
	fig, fit := r.audioFigure("xy")
	if len(fig.Points) < 100 {
		t.Fatalf("the XY drew %d points", len(fig.Points))
	}
	var lo, hi [2]float64
	for _, p := range fig.Points {
		for a := range 2 {
			lo[a], hi[a] = math.Min(lo[a], p[a]), math.Max(hi[a], p[a])
		}
	}
	for a := range 2 {
		if hi[a]-lo[a] < 0.8 || hi[a] > fit[1][a] || lo[a] < fit[0][a] {
			t.Errorf("axis %d spans %.2f..%.2f; two generators at 0.6 should fill most of ±%.1f", a, lo[a], hi[a], fit[1][a])
		}
	}
	img, err := r.Scene(120, 80, 1)
	if err != nil {
		t.Fatalf("the XY scene: %v", err)
	}
	lit := 0
	for i := 0; i+2 < len(img.Px); i += 3 {
		if int(img.Px[i])+int(img.Px[i+1])+int(img.Px[i+2]) > 120 {
			lit++
		}
	}
	if lit < 40 {
		t.Errorf("the XY scene has %d lit pixels", lit)
	}
}

// While the speakers play, they make the rack's signal, and a look makes
// none of it again.
func TestTheSpeakersMakeTheSignalWhileTheyPlay(t *testing.T) {
	r, c := idleRack(t, "takens")
	set(t, r, "mix-rl-g1", "1")
	l, rt := make([]float32, 4800), make([]float32, 4800)
	r.Fill(l, rt)
	n := r.sound.rack.n
	c.t = c.t.Add(50 * time.Millisecond)
	look(t, r)
	if r.sound.rack.n != n {
		t.Errorf("a look made %d more samples while the speakers were playing", r.sound.rack.n-n)
	}
	c.t = c.t.Add(time.Second)
	look(t, r)
	if got := r.sound.rack.n - n; got != maxIdleMake {
		t.Errorf("a second after the speakers stopped, a look made %d samples, want at most %d", got, maxIdleMake)
	}
}
