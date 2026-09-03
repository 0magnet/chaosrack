package attractor

import (
	"math"
	"strings"
	"testing"
)

// A pattern row shorter than the bar is the failure this table invites.
//
// The rows are hand-typed strings of dots, and a dot is exactly the character
// the eye cannot count. rhythmHit treats a short row as silence past its end
// rather than panicking, which is the right behavior at runtime and the wrong
// one for finding the mistake: a snare row one dot short simply loses its last
// step, and a bossa missing a beat sounds like a bossa played badly.
func TestPatternRowsFillTheirBar(t *testing.T) {
	if len(rhythmPatterns) == 0 {
		t.Fatal("no patterns at all, so this test proves nothing")
	}
	for _, p := range rhythmPatterns {
		if p.Steps <= 0 {
			t.Errorf("%s: %d steps", p.Name, p.Steps)
			continue
		}
		for v, row := range p.Rows {
			if len(row) != p.Steps {
				t.Errorf("%s voice %d: row is %d characters for a %d-step bar — the missing "+
					"steps play as silence and the pattern is quietly wrong: %q",
					p.Name, v, len(row), p.Steps, row)
			}
			if bad := strings.TrimLeft(row, "x."); bad != "" {
				t.Errorf("%s voice %d: %q contains %q; only 'x' is a hit, so anything else "+
					"is a rest that was meant to be a beat", p.Name, v, row, bad)
			}
		}
	}
}

// Every preset has to actually play something.
//
// An all-rests pattern is a tab that does nothing when pressed, and because the
// section still runs and the beat lamps still sweep, it looks like the audio is
// broken rather than like the pattern is empty.
func TestEveryPresetPlays(t *testing.T) {
	for _, p := range rhythmPatterns {
		hits := 0
		for v := 0; v < rhythmVoiceCount; v++ {
			hits += strings.Count(p.Rows[v], "x")
		}
		if hits == 0 {
			t.Errorf("%s has no hits in any voice — pressing its tab is silence", p.Name)
		}
	}
}

// Preset names are what the tabs print and what a permalink carries, so they
// have to be unique and short enough to read on a tab.
func TestPresetNamesAreUsableOnATab(t *testing.T) {
	const maxTabLabel = 8
	seen := map[string]bool{}
	for _, p := range rhythmPatterns {
		if seen[p.Name] {
			t.Errorf("%q appears twice; the second tab can never be selected, because "+
				"rhythmPatternByName returns the first", p.Name)
		}
		seen[p.Name] = true
		if len([]rune(p.Name)) > maxTabLabel {
			t.Errorf("%q is %d characters; past %d it widens the whole tab bank",
				p.Name, len([]rune(p.Name)), maxTabLabel)
		}
	}
}

// EVERY PRESET IS COUNTED IN THE METER IT IS ACTUALLY IN.
//
// Spelled out per preset rather than derived, because deriving it is the bug
// this replaces. rhythmBeatsPerBar used to infer the meter from the step count
// — "divisible by three and not by four is in three" — and twelve is divisible
// by both, so the waltz came back in four. It drew four lamps and ran its bar a
// third too fast.
//
// The test that was supposed to catch that asked whether step × steps-per-beat
// equalled one beat, computing steps-per-beat from the very function under
// test. It passed on the broken code because it could not do otherwise: both
// sides came from the same wrong answer. So the expectations here are written
// out by hand, which is the only form in which they say anything.
func TestEachPresetIsInItsOwnMeter(t *testing.T) {
	want := map[string]int{
		"waltz": 3, "march": 4, "rock": 4, "shuffle": 4, "swing": 4,
		"bossa": 4, "samba": 4, "tango": 4, "beguine": 4, "chacha": 4,
	}
	if len(want) != len(rhythmPatterns) {
		t.Fatalf("%d presets but %d expectations — a new preset needs its meter stated here",
			len(rhythmPatterns), len(want))
	}
	for _, p := range rhythmPatterns {
		w, ok := want[p.Name]
		if !ok {
			t.Errorf("%s has no stated meter in this test", p.Name)
			continue
		}
		if got := rhythmBeatsPerBar(p); got != w {
			t.Errorf("%s is counted in %d beats, want %d — its bar plays at %.2f× the "+
				"tempo the knob says", p.Name, got, w, float64(got)/float64(w))
		}
	}
}

// The step lengths, worked out by hand at a tempo that divides cleanly.
//
// Independent arithmetic, so it fails if the bar/beat/step relation is wrong in
// any direction — including the three-step-per-beat triplet feel that makes a
// shuffle a shuffle rather than a march with gaps.
func TestStepLengthsAtAKnownTempo(t *testing.T) {
	const bpm = 120 // a beat is exactly half a second
	cases := []struct {
		preset string
		want   float64
		why    string
	}{
		{"march", 0.125, "16 steps over 4 beats = sixteenths"},
		{"waltz", 0.125, "12 steps over 3 beats = sixteenths, three beats to the bar"},
		{"shuffle", 0.5 / 3, "12 steps over 4 beats = triplets"},
	}
	for _, c := range cases {
		p, ok := rhythmPatternByName(c.preset)
		if !ok {
			t.Errorf("%s went missing", c.preset)
			continue
		}
		if got := rhythmStepSeconds(bpm, p); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: a step lasts %.5fs at %d BPM, want %.5fs (%s)",
				c.preset, got, bpm, c.want, c.why)
		}
	}
}

// A bar has to divide evenly into its beats, or the beat lamps step off the
// grid and the pattern's accents land between beats.
func TestBarsDivideIntoWholeSteps(t *testing.T) {
	for _, p := range rhythmPatterns {
		if p.Beats <= 0 {
			t.Errorf("%s does not say how many beats its bar has", p.Name)
			continue
		}
		if p.Steps%p.Beats != 0 {
			t.Errorf("%s: %d steps counted in %d beats is not a whole number of steps "+
				"per beat", p.Name, p.Steps, p.Beats)
		}
	}
}

// rhythmHit is called with a counter that never wraps, so it has to.
func TestHitWrapsTheBar(t *testing.T) {
	p, ok := rhythmPatternByName("march")
	if !ok {
		t.Fatal("march went missing")
	}
	for s := 0; s < p.Steps*3; s++ {
		if rhythmHit(p, voiceBass, s) != rhythmHit(p, voiceBass, s%p.Steps) {
			t.Fatalf("step %d does not match step %d of the bar", s, s%p.Steps)
		}
	}
	if rhythmHit(p, -1, 0) || rhythmHit(p, rhythmVoiceCount, 0) {
		t.Error("an out-of-range voice reported a hit")
	}
}

// The default preset has to exist, or the module opens with no tab down and
// the section plays nothing until something is pressed.
func TestDefaultPresetExists(t *testing.T) {
	if _, ok := rhythmPatternByName(rhythmDefaultPreset); !ok {
		t.Errorf("the default preset %q is not in the table", rhythmDefaultPreset)
	}
}
