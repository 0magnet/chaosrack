package rhythm

// The Matrix's arithmetic: the grid the patterns are played on, where its
// loop can end, and what note each row is. Here rather than beside the page's
// grid so the rack on a host plays the same notes on the same clock.

// The grid: pitch rows, the columns of the row, and how many columns make a
// beat (sixteenths).
const (
	GridRows    = 16
	GridSteps   = 64
	GridPerBeat = 4
	GridBeats   = GridSteps / GridPerBeat
)

// LoopSteps are where the steps knob can end the loop, in its order: whole
// beats, three to sixteen.
var LoopSteps = []int{12, 16, 24, 32, 48, 64}

// penta is the major pentatonic, the tonematrix scale: five notes an octave
// and no semitone between any two, so every pattern is consonant.
var penta = [5]int{0, 2, 4, 7, 9}

// RootMidi is the root knob's octave as a midi note: C of it (1 is C1, 24).
func RootMidi(octave int) int { return 12 * (octave + 1) }

// NoteOf is grid row's midi note over root: pentatonic degrees stacked up
// from the root, the highest note on the top row (0).
func NoteOf(root, row int) int {
	b := GridRows - 1 - row
	return root + 12*(b/5) + penta[b%5]
}

// PingSeconds is how long a pad's note rings, for grid columns stepSeconds
// long: two of them, held between a fifth and a half of a second.
func PingSeconds(stepSeconds float64) float64 {
	return min(max(2*stepSeconds, 0.2), 0.5)
}

// LanePerBeat is how many drum-lane steps p counts to the beat: four for
// sixteenths, three for a triplet beat.
func LanePerBeat(p Pattern) int { return max(1, p.Steps/BeatsPerBar(p)) }

// LoopFor is the loop length a preset of this many beats to the bar wants:
// steps if it holds whole bars, else the nearest of LoopSteps that does.
func LoopFor(steps, beats int) int {
	if beats < 1 || (steps/GridPerBeat)%beats == 0 {
		return steps
	}
	best := steps
	for _, n := range LoopSteps {
		if (n/GridPerBeat)%beats != 0 {
			continue
		}
		if best == steps || abs(n-steps) < abs(best-steps) {
			best = n
		}
	}
	return best
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
