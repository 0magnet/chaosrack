package attractor

// The rhythm section, as the home organs had one.
//
// A Lowrey or a Yamaha Electone of the period carried a row of interlocking
// tabs — WALTZ, MARCH, BOSSA NOVA, SWING — and a tempo knob beside them. You
// pressed one tab and it popped the last one out, because the box played one
// rhythm at a time and the mechanism said so. That is what this is, and it is
// why the presets are SWITCHES here rather than the selector the backdrop got:
// there, a row of switches misrepresented a one-of-N choice with nothing to
// show it; here the interlocking bank IS the instrument being reproduced, and
// the exclusivity is the point rather than an accident.
//
// Untagged so the patterns can be read and checked on the host: whether they
// are the right length, whether a bar of a 3/4 rhythm really is three beats,
// whether anything plays on every step of every voice. None of that needs a
// browser, and a pattern table is exactly the sort of thing that rots quietly.

// rhythmVoices are the drums, in the order the pattern rows are written.
const (
	voiceBass   = iota // bass drum: a pitched thump
	voiceSnare         // snare: noise plus a tone
	voiceHat           // hi-hat: a short bright tick
	voiceCymbal        // cymbal / claves accent
	rhythmVoiceCount
)

// rhythmPattern is one preset: a bar of steps per voice.
//
// Written as strings because a drum pattern is a picture of itself — "x..x..x."
// is legible in a way [][]bool never is, and a wrong beat shows up by eye. An
// 'x' is a hit, anything else is a rest.
type rhythmPattern struct {
	Name  string
	Steps int // steps in one bar
	Beats int // beats in that bar — DECLARED, never inferred; see below
	Rows  [rhythmVoiceCount]string
}

// rhythmPatterns is the tab row, in the order the tabs appear.
//
// THE METER IS WRITTEN DOWN because it cannot be worked out from the step
// count, which is what the first version tried: "divisible by three and not by
// four means it is in three". Twelve is divisible by both, so every twelve-step
// bar came back as four beats. The waltz drew four lamps and ran its bar a
// third too fast, and the tell was four lamps under a rhythm that has three.
//
// A twelve-step bar is genuinely ambiguous and the ambiguity is musical, not
// arithmetical: the waltz is THREE beats of four sixteenths, while the shuffle
// and swing are FOUR beats of three — triplets. Same twelve boxes, different
// count, and only the person writing the pattern knows which.
var rhythmPatterns = []rhythmPattern{
	// Oom-pah-pah: bass on one, the other two beats on the snare, hat in
	// eighths so the count is audible. Three beats of four sixteenths.
	{"waltz", 12, 3, [rhythmVoiceCount]string{
		"x...........",
		"....x...x...",
		"x.x.x.x.x.x.",
		"............",
	}},
	{"march", 16, 4, [rhythmVoiceCount]string{
		"x...x...x...x...",
		"....x.......x...",
		"x.x.x.x.x.x.x.x.",
		"................",
	}},
	{"rock", 16, 4, [rhythmVoiceCount]string{
		"x.......x.......",
		"....x.......x...",
		"x.x.x.x.x.x.x.x.",
		"................",
	}},
	// Four beats of triplets. The hat takes the FIRST AND THIRD of each triplet
	// and skips the middle — that long-short limp is the whole of what makes a
	// shuffle a shuffle. On the beat instead (which is how this was first
	// written) it is a slow march wearing the name.
	{"shuffle", 12, 4, [rhythmVoiceCount]string{
		"x.....x.....",
		"...x.....x..",
		"x.xx.xx.xx.x",
		"............",
	}},
	{"swing", 12, 4, [rhythmVoiceCount]string{
		"x.....x.....",
		"...x.....x..",
		"x..x.xx..x.x",
		"............",
	}},
	{"bossa", 16, 4, [rhythmVoiceCount]string{
		"x..x..x...x..x..",
		"................",
		"x.x.x.x.x.x.x.x.",
		"..x...x..x...x..",
	}},
	{"samba", 16, 4, [rhythmVoiceCount]string{
		"x..x..x.x..x..x.",
		"....x.......x...",
		"xxxxxxxxxxxxxxxx",
		"..x..x....x..x..",
	}},
	{"tango", 16, 4, [rhythmVoiceCount]string{
		"x...x...x...x...",
		"......x.......x.",
		"x.x.x.x.x.x.x.x.",
		"x.....x.x.....x.",
	}},
	{"beguine", 16, 4, [rhythmVoiceCount]string{
		"x.....x...x.....",
		"....x.......x...",
		"x.x.x.x.x.x.x.x.",
		"..x...x...x...x.",
	}},
	{"chacha", 16, 4, [rhythmVoiceCount]string{
		"x...x...x...x...",
		"............x.x.",
		"x.x.x.x.x.x.x.x.",
		"........x...x.x.",
	}},
}

// rhythmDefaultPreset is the tab that is down when the module opens. Here
// rather than beside the live state so the host can check it names a real
// pattern — a default that does not exist is a module that opens silent.
const rhythmDefaultPreset = "rock"

// rhythmPatternByName finds a preset, and reports whether there was one.
func rhythmPatternByName(name string) (rhythmPattern, bool) {
	for _, p := range rhythmPatterns {
		if p.Name == name {
			return p, true
		}
	}
	return rhythmPattern{}, false
}

// rhythmHit reports whether voice v plays on step s of p.
//
// The step wraps, so a caller counting freely upward does not have to know how
// long the bar is — which is what lets the tempo clock stay a simple counter.
func rhythmHit(p rhythmPattern, v, s int) bool {
	if v < 0 || v >= rhythmVoiceCount || p.Steps <= 0 {
		return false
	}
	row := p.Rows[v]
	if len(row) == 0 {
		return false
	}
	i := s % p.Steps
	if i < 0 {
		i += p.Steps
	}
	if i >= len(row) {
		return false
	}
	return row[i] == 'x'
}

// rhythmBeatsPerBar is how many beats one bar of p is counted in.
//
// A thin reader over the declared field, kept as a function because both
// callers — the beat lamps asking how many to draw and the tempo asking how
// long a beat is — must get the same answer, and because it is the one place
// to catch a pattern that forgot to say.
func rhythmBeatsPerBar(p rhythmPattern) int {
	if p.Beats > 0 {
		return p.Beats
	}
	return 4
}

// rhythmStepSeconds is how long one step lasts at a given tempo.
//
// TEMPO IS IN BEATS, NOT STEPS, because that is what the knob on the organ
// meant and what a metronome marking means. A four-four bar of sixteen steps is
// four beats, so a step is a sixteenth; a bar of twelve in three is three
// beats, so a step is a triplet of the beat. Dividing by the step count instead
// would run the waltz at a third of the tempo it claimed — the kind of wrong
// that sounds merely "slow" rather than broken, and so goes unnoticed.
func rhythmStepSeconds(bpm float64, p rhythmPattern) float64 {
	if bpm <= 0 || p.Steps <= 0 {
		return 0
	}
	barSeconds := 60 / bpm * float64(rhythmBeatsPerBar(p))
	return barSeconds / float64(p.Steps)
}
