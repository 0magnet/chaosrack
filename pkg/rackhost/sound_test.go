package rackhost

import (
	"math"
	"testing"
)

// fakeSpeaker plays the way PulseAudio does: starting asks the rack for its
// first samples before it returns.
type fakeSpeaker struct {
	r       *Rack
	playing bool
	plays   int
}

func (f *fakeSpeaker) Play() error {
	if !f.playing {
		l, r := make([]float32, 64), make([]float32, 64)
		f.r.Fill(l, r)
	}
	f.playing = true
	f.plays++
	return nil
}

func (f *fakeSpeaker) Pause() { f.playing = false }

// listen is a second of the speakers, after a tenth to let levels settle.
func listen(r *Rack) (l, rt []float32) {
	skip := make([]float32, sampleRate/10)
	r.Fill(skip, make([]float32, len(skip)))
	l, rt = make([]float32, sampleRate), make([]float32, sampleRate)
	r.Fill(l, rt)
	return l, rt
}

// pitch is a signal's frequency by its upward zero crossings, and its peak.
func pitch(s []float32) (hz, peak float64) {
	n := 0
	for i := 1; i < len(s); i++ {
		if s[i-1] < 0 && s[i] >= 0 {
			n++
		}
		peak = max(peak, math.Abs(float64(s[i])))
	}
	return float64(n) * sampleRate / float64(len(s)), peak
}

func set(t *testing.T, r *Rack, kv ...string) {
	t.Helper()
	for i := 0; i+1 < len(kv); i += 2 {
		if err := r.Set(kv[i], kv[i+1]); err != nil {
			t.Fatal(err)
		}
	}
}

// A rack nobody has touched is silent, as the page's is: no generator is
// pinned to a speaker.
func TestAnUntouchedRackIsSilent(t *testing.T) {
	r := newRack(t)
	sp := &fakeSpeaker{r: r}
	r.SetSpeaker(sp)
	if sp.playing {
		t.Error("the speaker was started for an untouched rack")
	}
	l, rt := listen(r)
	if _, p := pitch(l); p != 0 {
		t.Errorf("left peaks at %v", p)
	}
	if _, p := pitch(rt); p != 0 {
		t.Errorf("right peaks at %v", p)
	}
}

// A generator switched on and sent OUT to one side plays there, Gen 1 on the
// left and Gen 2 on the right, each its knob's note at its level.
func TestTurningAGeneratorPlaysIt(t *testing.T) {
	r := newRack(t)
	sp := &fakeSpeaker{r: r}
	r.SetSpeaker(sp)
	set(t, r, "gen-x-wave", "0", "gen-x-out", "1")
	set(t, r, "gen-x-lvl", "50")
	if !sp.playing {
		t.Fatal("turning Gen 1 did not start the speaker")
	}
	l, rt := listen(r)
	if hz, p := pitch(l); math.Abs(hz-196) > 2 || math.Abs(p-0.5) > 0.02 {
		t.Errorf("left plays %.1f Hz at %.2f, want G3 (196 Hz) at 0.50", hz, p)
	}
	if _, p := pitch(rt); p != 0 {
		t.Errorf("Gen 1 is on the right too, at %v", p)
	}
	set(t, r, "gen-y-wave", "0", "gen-y-out", "3", "gen-y-freq", "48") // A4
	_, rt = listen(r)
	if hz, _ := pitch(rt); math.Abs(hz-440) > 2 {
		t.Errorf("right plays %.1f Hz, want A4 (440 Hz)", hz)
	}
	// Its LED says the hertz, as the page's does, not the knob's semitones.
	if ro, ok := r.reads["gen-y-freq"]; !ok {
		t.Error("Gen 2's frequency has no readout")
	} else if got := r.pic.Items[ro.item].Text; got != "00440.0" {
		t.Errorf("Gen 2's LED reads %q, want 00440.0", got)
	}
	// Solo Gen 2: the left falls silent.
	set(t, r, "gen-solo", "y")
	l, _ = listen(r)
	if _, p := pitch(l); p > 0.001 {
		t.Errorf("with Gen 2 soloed the left still peaks at %v", p)
	}
	// Both off: the speaker is paused.
	set(t, r, "gen-solo", "", "gen-x-wave", "off", "gen-y-wave", "off")
	if sp.playing {
		t.Error("the speaker plays with every generator off")
	}
}

// The envelope, repeating, shapes the level: the loudest stretch and the
// quietest differ.
func TestTheEnvelopeShapesTheLevel(t *testing.T) {
	r := newRack(t)
	set(t, r, "gen-x-wave", "0", "gen-x-out", "1", "gen-x-lvl", "100", "gen-x-atk", "100", "gen-x-dcy", "100", "gen-x-env", "rpt")
	l, _ := listen(r)
	lo, hi := 1.0, 0.0
	for k := 0; k+480 <= len(l); k += 480 {
		_, p := pitch(l[k : k+480])
		lo, hi = min(lo, p), max(hi, p)
	}
	if hi-lo < 0.5 {
		t.Errorf("with a 100 ms envelope the level ranges %.2f to %.2f", lo, hi)
	}
}

// A generator's OUT pins it on the Mixer, the speakers and the rack alike,
// as controls, and its pin's lamp lights; a pin moved on the Mixer turns OUT
// to MIX.
func TestOutPinsTheGenerator(t *testing.T) {
	r := newRack(t)
	set(t, r, "gen-y-wave", "0", "gen-y-out", "3")
	for _, id := range []string{"mix-sr-g2", "mix-rr-g2"} {
		if got := r.value(id); got != "1" {
			t.Fatalf("OUT R left %s at %q", id, got)
		}
	}
	for _, id := range []string{"mix-sl-g2", "mix-rl-g2"} {
		if got := r.value(id); got != "0" {
			t.Fatalf("OUT R left %s at %q", id, got)
		}
	}
	l, ok := r.lamps["mix-sr-g2"]
	if !ok {
		t.Fatal("the pin has no lamp")
	}
	if f := r.pic.Items[l].Fill; f == nil || *f != pinLit {
		t.Errorf("the pin's lamp is %v, want lit %06x", f, pinLit)
	}
	// Clicking it unpins it, and Gen 2 falls silent.
	b := r.pic.Ctls["mix-sr-g2"]
	if err := r.Act(0, b[0]+b[2]/2, b[1]+b[3]/2, "click", 0); err != nil {
		t.Fatal(err)
	}
	if got := r.value("mix-sr-g2"); got != "0" {
		t.Errorf("clicking the pin left it at %q", got)
	}
	if got := r.value("gen-y-out"); got != "4" {
		t.Errorf("with only the rack pinned, OUT reads %q, want MIX (4)", got)
	}
	if f := r.pic.Items[l].Fill; f == nil || *f != pinDark {
		t.Errorf("the unpinned lamp is %v, want dark %06x", f, pinDark)
	}
	_, rt := listen(r)
	if _, p := pitch(rt); p > 0.001 {
		t.Errorf("unpinned, Gen 2 still peaks at %v", p)
	}
}

// Model Out pinned to a speaker plays the model's equations at its level,
// and a model with none plays nothing.
func TestModelOutPlaysTheModel(t *testing.T) {
	r := newRack(t)
	if r.selector == "" {
		t.Fatal("no model selector in the catalog")
	}
	sp := &fakeSpeaker{r: r}
	r.SetSpeaker(sp)
	set(t, r, r.selector, "lorenz", "mix-sl-mx", "1")
	if !sp.playing {
		t.Fatal("pinning the model's x did not start the speaker")
	}
	l, rt := listen(r)
	if _, p := pitch(l); p < 0.4 || p > 0.61 {
		t.Errorf("lorenz's x peaks at %.2f on the left, want about LVL's 0.60", p)
	}
	if _, p := pitch(rt); p != 0 {
		t.Errorf("the right peaks at %v with nothing pinned there", p)
	}
	set(t, r, "mo-lvl", "100")
	l, _ = listen(r)
	if _, p := pitch(l); p < 0.8 {
		t.Errorf("at LVL 100 lorenz's x peaks at %.2f", p)
	}
	set(t, r, r.selector, "globe")
	if sp.playing {
		t.Error("the globe has no equations, and the speaker still plays")
	}
}
