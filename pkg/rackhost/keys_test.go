package rackhost

import (
	"math"
	"testing"

	"github.com/0magnet/chaosrack/pkg/keysynth"
)

// keyAt is the middle of key midi's lower part, below where the black keys
// reach, or of the black key itself.
func keyAt(t *testing.T, r *Rack, midi int) (x, y float64) {
	t.Helper()
	i, ok := r.keys.item[midi]
	if !ok {
		t.Fatalf("the picture has no key %d", midi)
	}
	it := r.pic.Items[i]
	if isBlack(midi) {
		return it.X + it.W/2, it.Y + it.H/2
	}
	return it.X + it.W/2, it.Y + it.H*0.85
}

// look is the panel looking at the rack, as it does every refresh.
func look(t *testing.T, r *Rack) {
	t.Helper()
	if _, err := r.PictureChanges(0); err != nil {
		t.Fatal(err)
	}
}

func act(t *testing.T, r *Rack, x, y float64, kind string) {
	t.Helper()
	if err := r.Act(0, x, y, kind, 0); err != nil {
		t.Fatal(err)
	}
}

// fill is n seconds of the left speaker.
func fill(r *Rack, secs float64) []float32 {
	n := int(secs * sampleRate)
	l, rt := make([]float32, n), make([]float32, n)
	r.Fill(l, rt)
	return l
}

// A key held down sounds its note until it is let go, lit in the picture as
// the page lights it, and rings out after.
func TestAKeyPlaysItsNote(t *testing.T) {
	r := newRack(t)
	sp := &fakeSpeaker{r: r}
	r.SetSpeaker(sp)
	set(t, r, "syn-preset", "sine")
	x, y := keyAt(t, r, 69)
	act(t, r, x, y, "click")
	if !sp.playing {
		t.Fatal("pressing a key did not start the speaker")
	}
	look(t, r)
	if f := r.pic.Items[r.keys.item[69]].Fill; f == nil || *f != heldWhite {
		t.Errorf("the held A4 is drawn %v, want the page's held green", f)
	}
	s := fill(r, 1)
	if hz := crossings(s[sampleRate/2:]); math.Abs(hz-440) > 2 {
		t.Errorf("A4 sounds at %.1f Hz, want 440", hz)
	}
	// The sine preset: 0.8 of the level's 0.8, under the keys' headroom.
	if _, p := pitch(s[sampleRate/2:]); math.Abs(p-0.8*0.8*keysHeadroom) > 0.01 {
		t.Errorf("A4 peaks at %.3f, want %.3f", p, 0.8*0.8*keysHeadroom)
	}
	act(t, r, 0, 0, "release")
	look(t, r)
	if f := r.pic.Items[r.keys.item[69]].Fill; f != nil && *f == heldWhite {
		t.Error("the key let go is still lit")
	}
	// It rings out over the release (0.12 s), and then stops by itself.
	tail := fill(r, 0.2)
	if _, p := pitch(tail[:sampleRate/50]); p < 0.05 {
		t.Errorf("the note stopped dead at the release, peak %.3f", p)
	}
	if !sp.playing {
		t.Fatal("the speaker stopped before the note rang out")
	}
	look(t, r)
	if len(r.sound.keys.voices) != 0 || sp.playing {
		t.Errorf("%d notes sound and the speaker plays %v after the release; want none", len(r.sound.keys.voices), sp.playing)
	}
}

// The piano is the default: its strings fall away while the key is held, the
// bass slower than the treble.
func TestThePianoFallsAway(t *testing.T) {
	r := newRack(t)
	r.SetSpeaker(&fakeSpeaker{r: r})
	x, y := keyAt(t, r, 48)
	act(t, r, x, y, "click")
	s := fill(r, 2)
	_, early := pitch(s[sampleRate/10 : sampleRate/5])
	_, late := pitch(s[sampleRate*9/5:])
	if early < 0.02 || late > early*0.8 {
		t.Errorf("C3 peaks %.3f early and %.3f at 2 s; it should fall away", early, late)
	}
	p := keysynth.Params{Dec: 3.2, Ktrk: 1}
	if p.Tau(36) <= p.Tau(84) {
		t.Error("the bass falls as fast as the treble")
	}
}

// A preset turns every knob: its own values, and the piano's for the rest.
func TestAPresetSetsEveryKnob(t *testing.T) {
	r := newRack(t)
	set(t, r, "syn-dec", "7")
	set(t, r, "syn-preset", "organ")
	for id, want := range map[string]string{
		"syn-str": "0", "syn-wave": "1", "syn-sus": "1", "syn-shape": "organ", "syn-dec": "3.2",
	} {
		if got := r.value(id); got != want {
			t.Errorf("after organ, %s is %s, want %s", id, got, want)
		}
	}
	k := &r.sound.keys
	if k.p.Str != 0 || k.p.Wave != 1 || k.p.Shape != "organ" {
		t.Errorf("the organ's sound is str %v wave %v shape %q", k.p.Str, k.p.Wave, k.p.Shape)
	}
	set(t, r, "syn-preset", "piano")
	if r.value("syn-str") != "1" || r.value("syn-wave") != "0" || r.value("syn-shape") != "sine" {
		t.Error("the piano did not put the organ's knobs back")
	}
}

// The organ holds while the key is down, as a held plain wave does.
func TestTheOrganHolds(t *testing.T) {
	r := newRack(t)
	r.SetSpeaker(&fakeSpeaker{r: r})
	set(t, r, "syn-preset", "organ")
	x, y := keyAt(t, r, 60)
	act(t, r, x, y, "click")
	s := fill(r, 2)
	_, early := pitch(s[sampleRate/5 : sampleRate/2])
	_, late := pitch(s[sampleRate*3/2:])
	if early < 0.05 || math.Abs(late-early) > early*0.05 {
		t.Errorf("the organ peaks %.3f early and %.3f later; it should hold", early, late)
	}
}

// The shape knob retypes a held plain wave at once, as the page's does.
func TestTheShapeKnobRetypesAHeldNote(t *testing.T) {
	r := newRack(t)
	r.SetSpeaker(&fakeSpeaker{r: r})
	set(t, r, "syn-preset", "sine")
	x, y := keyAt(t, r, 57)
	act(t, r, x, y, "click")
	fill(r, 0.1)
	set(t, r, "syn-shape", "square")
	s := fill(r, 0.2)
	// A square's samples sit at its level, a sine's spread below it. The
	// level, not the peak: the filter rings over each edge.
	at, lvl := 0, 0.8*0.8*keysHeadroom
	for _, v := range s {
		if math.Abs(math.Abs(float64(v))-lvl) < lvl*0.05 {
			at++
		}
	}
	if float64(at)/float64(len(s)) < 0.8 {
		t.Errorf("%.0f%% of the held note is at its level; a square's is nearly all", 100*float64(at)/float64(len(s)))
	}
}

// A drag sounds every key it crosses, in order, and holds the one it lands
// on; the keys it passed let go by themselves.
func TestADragPlaysTheKeysItCrosses(t *testing.T) {
	r := newRack(t)
	r.SetSpeaker(&fakeSpeaker{r: r})
	x0, y := keyAt(t, r, 60)
	x1, _ := keyAt(t, r, 67)
	act(t, r, x0, y, "click")
	act(t, r, x1, y, "drag")
	k := &r.sound.keys
	var got []int
	for _, v := range k.voices {
		got = append(got, v.midi)
	}
	want := []int{60, 62, 64, 65, 67} // the white keys, low on the bed
	if len(got) != len(want) {
		t.Fatalf("the drag sounds %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the drag sounds %v, want %v", got, want)
		}
	}
	if _, held := k.held[67]; !held || len(k.held) != 1 {
		t.Errorf("held %v after the drag, want only G4", k.held)
	}
	fill(r, 1)
	if len(k.lit()) != 1 {
		t.Errorf("lit %v a second on, want only the key landed on", k.lit())
	}
}

// A drag or a release away from the keys is nothing to any control.
func TestADragOffTheKeysTurnsNothing(t *testing.T) {
	r := newRack(t)
	vals := map[string]string{}
	for _, c := range r.ctls {
		vals[c.ID] = c.Value
	}
	for i, it := range r.pic.Items {
		if it == nil || i >= r.outside {
			continue
		}
		act(t, r, it.X+it.W/2, it.Y+it.H/2, "drag")
		act(t, r, it.X+it.W/2, it.Y+it.H/2, "release")
	}
	for _, c := range r.ctls {
		if vals[c.ID] != c.Value {
			t.Errorf("a drag or release moved %s from %s to %s", c.ID, vals[c.ID], c.Value)
		}
	}
}
