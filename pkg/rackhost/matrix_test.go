package rackhost

import (
	"math"
	"testing"

	"github.com/0magnet/chaosrack/pkg/rackpic"
	"github.com/0magnet/chaosrack/pkg/rhythm"
)

// press clicks the middle of the part the catalog marked mark.
func press(t *testing.T, r *Rack, mark string) {
	t.Helper()
	for _, i := range r.mx.marked {
		if it := r.pic.Items[i]; it != nil && it.Mark == mark {
			if err := r.Act(0, it.X+it.W/2, it.Y+it.H/2, "click", 0); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("the picture has no %q", mark)
}

// onsets is when s starts a hit, in seconds: a 5 ms window louder than the
// one before by a margin, after a quiet one.
func onsets(s []float32) []float64 {
	const win = sampleRate / 200
	var out []float64
	prev := 0.0
	for k := 0; k+win <= len(s); k += win {
		_, p := pitch(s[k : k+win])
		if p > 0.02 && p > 2*prev {
			out = append(out, float64(k)/sampleRate)
		}
		prev = p
	}
	return out
}

// crossings is how many times a second s crosses zero going up.
func crossings(s []float32) float64 { hz, _ := pitch(s); return hz }

// The Matrix opens with the rock beat in its lanes and Run on, and still
// waits for a hand, as the page's does.
func TestTheMatrixWaitsForAHand(t *testing.T) {
	r := newRack(t)
	sp := &fakeSpeaker{r: r}
	r.SetSpeaker(sp)
	if _, drums := r.sound.mat.lit(); !drums {
		t.Fatal("the lanes are empty; the catalog's preset is rock")
	}
	if sp.playing {
		t.Error("the speaker started before anything was touched")
	}
}

// Pressing a rhythm tab plays its beat on both speakers: rock's hi-hat
// counts eighths, an onset every quarter of a second at 120.
func TestARhythmTabPlaysItsBeat(t *testing.T) {
	r := newRack(t)
	sp := &fakeSpeaker{r: r}
	r.SetSpeaker(sp)
	press(t, r, "tab rock")
	if !sp.playing {
		t.Fatal("pressing the rock tab did not start the speaker")
	}
	l, rt := make([]float32, 2*sampleRate), make([]float32, 2*sampleRate)
	r.Fill(l, rt)
	on := onsets(l)
	if len(on) < 7 || len(on) > 9 {
		t.Fatalf("a bar of rock has %d onsets at %v, want 8", len(on), on)
	}
	for i := 1; i < len(on); i++ {
		if d := on[i] - on[i-1]; math.Abs(d-0.25) > 0.011 {
			t.Errorf("onsets %d and %d are %.3f s apart, want 0.25", i-1, i, d)
		}
	}
	if _, p := pitch(rt); p < 0.05 {
		t.Errorf("the right peaks at %v; the drums are pinned to both", p)
	}
	// The playhead and the beat lamp follow.
	if _, err := r.Picture(); err != nil {
		t.Fatal(err)
	}
	if v := r.mx.shown; v.col < 0 || v.beat < 0 {
		t.Errorf("after a bar the picture shows column %d and beat %d", v.col, v.beat)
	}
	set(t, r, "tm-run", "0")
	if sp.playing {
		t.Error("Run off, and the speaker plays")
	}
}

// A pad plays its row's note: the bottom row is the root, C3 by default.
func TestAPadPlaysItsNote(t *testing.T) {
	r := newRack(t)
	press(t, r, "clear")
	press(t, r, "pad 0 15")
	if !r.mx.pat[0][15] && !r.sound.mat.pat[0][15] {
		t.Fatal("the pad did not light")
	}
	l, rt := make([]float32, sampleRate/5), make([]float32, sampleRate/5)
	r.Fill(l, rt)
	want := 440 * math.Pow(2, float64(rhythm.RootMidi(3)-69)/12)
	if hz := crossings(l[sampleRate/100:]); math.Abs(hz-want) > 6 {
		t.Errorf("the bottom pad plays %.1f Hz, want C3 (%.1f)", hz, want)
	}
	// A fifth of a second in, at 120 the playhead is on the second column:
	// the pad is lit, and the one beside it is under the playhead.
	p, err := r.Picture()
	if err != nil {
		t.Fatal(err)
	}
	if it := p.Items[r.mx.pad[0][15]-1]; fillOf(it) != padOn {
		t.Errorf("the pad is %06x, want lit %06x", fillOf(it), padOn)
	}
	if it := p.Items[r.mx.pad[1][15]-1]; fillOf(it) != phOff {
		t.Errorf("the pad beside it is %06x, want the playhead's %06x", fillOf(it), phOff)
	}
	// Back on the first column the lit pad is the playhead's bright green.
	skip := make([]float32, 2*sampleRate-sampleRate/5+100) // the loop is two seconds
	r.Fill(skip, make([]float32, len(skip)))
	if p, err = r.Picture(); err != nil {
		t.Fatal(err)
	}
	if it := p.Items[r.mx.pad[0][15]-1]; fillOf(it) != phOn {
		t.Errorf("the pad sounding is %06x, want %06x", fillOf(it), phOn)
	}
}

// A preset in three, or in triplets, redraws the lanes and the lamps, and
// moves the loop to a whole number of its bars.
func TestAPresetSetsTheLanesAndTheLoop(t *testing.T) {
	r := newRack(t)
	set(t, r, "rhythm-preset", "waltz")
	if got := r.value("tm-steps"); got != "12" {
		t.Errorf("a waltz left the loop at %s steps, want 12", got)
	}
	p, err := r.Picture()
	if err != nil {
		t.Fatal(err)
	}
	shown := 0
	for _, i := range r.mx.beats {
		if p.Items[i] != nil {
			shown++
		}
	}
	if shown != 3 {
		t.Errorf("a waltz shows %d beat lamps, want 3", shown)
	}
	set(t, r, "rhythm-preset", "shuffle")
	p, err = r.Picture()
	if err != nil {
		t.Fatal(err)
	}
	row := rhythm.GridRows + rhythm.Bass
	a, b := p.Items[r.mx.pad[0][row]-1], p.Items[r.mx.pad[2][row]-1]
	if a == nil || b == nil || p.Items[r.mx.pad[3][row]-1] != nil {
		t.Fatal("a triplet beat should draw three lane steps of each beat's four")
	}
	four := r.mx.base[r.mx.pad[0][row]-1].W
	if a.W <= four {
		t.Errorf("a triplet step is %.1f wide, no wider than a sixteenth's %.1f", a.W, four)
	}
	// The third triplet ends where the fourth sixteenth did.
	end := r.mx.base[r.mx.pad[3][row]-1]
	if math.Abs(b.X+b.W-(end.X+end.W)) > 0.01 {
		t.Errorf("the beat's triplets end at %.1f, its sixteenths at %.1f", b.X+b.W, end.X+end.W)
	}
	// A press on a triplet lands on its lane step.
	press(t, r, "pad 2 16")
	if got := r.sound.mat.drum[2][rhythm.Bass]; got == rhythm.Hit(mustPreset(t, "shuffle"), rhythm.Bass, 2) {
		t.Error("pressing the third triplet of the first beat did not change it")
	}
}

func mustPreset(t *testing.T, name string) rhythm.Pattern {
	t.Helper()
	p, ok := rhythm.ByName(name)
	if !ok {
		t.Fatalf("no preset %q", name)
	}
	return p
}

// The drums are what the page builds of nodes: a bass drum sweeps down to
// 45 Hz, a hat is noise with the lows taken out, and each falls silent.
func TestTheDrumVoices(t *testing.T) {
	play := func(v int, secs float64) []float32 {
		vc := drum(v, 0)
		out := make([]float32, int(secs*sampleRate))
		for i := range out {
			s, on := vc.next()
			out[i] = float32(s)
			if !on {
				return out[:i]
			}
		}
		return out
	}
	bass := play(rhythm.Bass, 1)
	if n := float64(len(bass)) / sampleRate; math.Abs(n-0.3) > 0.001 {
		t.Errorf("the bass drum stops after %.3f s, want 0.3", n)
	}
	if hz := crossings(bass[sampleRate/10 : sampleRate/4]); math.Abs(hz-45) > 7 {
		t.Errorf("the bass drum settles at %.1f Hz, want 45", hz)
	}
	if _, p := pitch(bass[:sampleRate/100]); p < 0.5 || p > 0.9 {
		t.Errorf("the bass drum starts at %.2f, want up to 0.9", p)
	}
	hat := play(rhythm.Hat, 1)
	if n := float64(len(hat)) / sampleRate; math.Abs(n-0.07) > 0.001 {
		t.Errorf("the hat stops after %.3f s, want 0.07", n)
	}
	if hz := crossings(hat[:sampleRate/50]); hz < 5000 {
		t.Errorf("the hat crosses zero %.0f times a second; it should be bright", hz)
	}
	cym := play(rhythm.Cymbal, 1)
	if n := float64(len(cym)) / sampleRate; math.Abs(n-0.47) > 0.001 {
		t.Errorf("the cymbal stops after %.3f s, want 0.47", n)
	}
	if sn := play(rhythm.Snare, 1); len(sn) != sampleRate/5 {
		t.Errorf("the snare lasts %d samples, want %d", len(sn), sampleRate/5)
	}
}

// A drum lane hit waits its turn within the beat: the triplets of a shuffle
// fall a third of a beat apart.
func TestTripletsFallBetweenTheColumns(t *testing.T) {
	m := newMatrix()
	m.armed, m.run, m.dlvl, m.steps = true, true, 1, 16
	m.loadDrums(rhythm.Pattern{Name: "t", Steps: 12, Beats: 4, Rows: [rhythm.VoiceCount]string{"", "", "xxx.........", ""}})
	tm, dr := make([]float32, sampleRate/2), make([]float32, sampleRate/2)
	m.render(tm, dr, len(tm))
	on := onsets(dr)
	if len(on) != 3 {
		t.Fatalf("three triplet hats sound at %v", on)
	}
	third := 60.0 / 120 / 3
	for i := 1; i < 3; i++ {
		if d := on[i] - on[i-1]; math.Abs(d-third) > 0.006 {
			t.Errorf("triplets %d and %d are %.3f s apart, want %.3f", i-1, i, d, third)
		}
	}
}

// fillOf is an item's fill, -1 for none.
func fillOf(it *rackpic.Item) int {
	if it == nil || it.Fill == nil {
		return -1
	}
	return *it.Fill
}
