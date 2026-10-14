package rackhost

import (
	"math"
	"strconv"

	"github.com/0magnet/chaosrack/pkg/rhythm"
)

// The Matrix: the pentatonic step sequencer and, under its grid, the drum
// lanes, played by one clock as the page's are (tonematrix_js.go). The page
// schedules a column a moment ahead on the audio clock; here the clock IS the
// samples, so a column sounds on the sample it falls on.
//
// Like the page's, it is silent until a hand has been on it: a pad pressed,
// a rhythm tab pressed, or Run switched on. The page cannot start its audio
// before that, and a rack that boots into a rock beat is not the page's.

// matrix is the Matrix's pattern and clock. It is the sound's: Fill plays it,
// under the sound's lock.
type matrix struct {
	pat  [rhythm.GridSteps][rhythm.GridRows]bool
	drum [rhythm.GridSteps][rhythm.VoiceCount]bool
	// perBeat is the lanes' steps to the beat and beats the bar's, both the
	// preset's.
	perBeat, beats int

	armed, run bool
	bpm        float64
	steps      int     // where the loop ends
	root       int     // midi note of the bottom row
	wave       int     // the pads' wave
	lvl, dlvl  float64 // the pads' and the drums' levels, 0..1

	step   int     // the next grid column
	until  float64 // samples until it sounds
	col    int     // the column sounding, -1 for none
	beatAt float64 // samples since the beat sounding began

	voices []voice
	edits  int // counts changes to the pads, for the picture
}

func newMatrix() matrix {
	return matrix{perBeat: rhythm.GridPerBeat, beats: 4, bpm: 120, steps: 16, col: -1}
}

// maxVoices bounds the hits sounding at once: a full column at the fastest
// tempo rings over a few dozen columns.
const maxVoices = 512

// stepSeconds is a grid column: a sixteenth at the tempo, whatever the drums
// count in.
func (m *matrix) stepSeconds() float64 { return 60 / m.bpm / rhythm.GridPerBeat }

// lit reports whether any pad or lane step is on.
func (m *matrix) lit() (pads, drums bool) {
	for c := range m.pat {
		for _, on := range m.pat[c] {
			pads = pads || on
		}
		for _, on := range m.drum[c] {
			drums = drums || on
		}
	}
	return pads, drums
}

// playing reports whether the clock runs.
func (m *matrix) playing() bool { return m.armed && m.run }

// loadDrums puts a rhythm preset into the lanes, its bar repeated across all
// of them, and starts the loop from the top. It says what the steps knob
// should be: a whole number of the preset's bars.
func (m *matrix) loadDrums(p rhythm.Pattern) (steps int) {
	m.beats = rhythm.BeatsPerBar(p)
	if pb := rhythm.LanePerBeat(p); rhythm.GridBeats*pb <= rhythm.GridSteps {
		m.perBeat = pb
	}
	m.drum = [rhythm.GridSteps][rhythm.VoiceCount]bool{}
	for d := range rhythm.GridBeats * m.perBeat {
		for v := range rhythm.VoiceCount {
			m.drum[d][v] = rhythm.Hit(p, v, d)
		}
	}
	m.edits++
	m.step, m.until = 0, 0
	return rhythm.LoopFor(m.steps, m.beats)
}

// press toggles pad c of row r, where a row past the pitches is a drum lane
// and c that lane's step.
func (m *matrix) press(c, r int) {
	if c < 0 || c >= rhythm.GridSteps || r < 0 {
		return
	}
	switch {
	case r < rhythm.GridRows:
		m.pat[c][r] = !m.pat[c][r]
	case r < rhythm.GridRows+rhythm.VoiceCount && c < rhythm.GridBeats*m.perBeat:
		m.drum[c][r-rhythm.GridRows] = !m.drum[c][r-rhythm.GridRows]
	default:
		return
	}
	m.edits++
}

// clear puts every pad out.
func (m *matrix) clear() {
	m.pat = [rhythm.GridSteps][rhythm.GridRows]bool{}
	m.drum = [rhythm.GridSteps][rhythm.VoiceCount]bool{}
	m.edits++
}

// render adds n samples of the pads to tm and of the drums to dr.
func (m *matrix) render(tm, dr []float32, n int) {
	clear(tm[:n])
	clear(dr[:n])
	stepLen := m.stepSeconds() * sampleRate
	for j := range n {
		if m.playing() {
			if m.until <= 0 {
				m.fire(m.step, stepLen)
				m.step = (m.step + 1) % max(1, m.steps)
				m.until += stepLen
			}
			m.until--
			m.beatAt++
		}
		var out [2]float64
		live := m.voices[:0]
		for i := range m.voices {
			v := &m.voices[i]
			s, on := v.next()
			out[v.bus()] += s
			if on {
				live = append(live, *v)
			}
		}
		m.voices = live
		tm[j] = float32(out[busTM] * m.lvl * 0.3) // headroom for full columns
		// Headroom: a samba puts four drums on some steps, and a bass drum
		// alone peaks at 0.9.
		dr[j] = float32(out[busDR] * m.dlvl * 0.35)
	}
}

// fire sounds column c: a ping for each lit pad, and on the column that
// starts a beat, that beat's lane steps, spread across it at the lanes' own
// subdivision, so a triplet lands between the grid's columns.
func (m *matrix) fire(c int, stepLen float64) {
	m.col = c
	ring := rhythm.PingSeconds(m.stepSeconds())
	for r, on := range m.pat[c] {
		if on {
			hz := 440 * math.Pow(2, float64(rhythm.NoteOf(m.root, r)-69)/12)
			m.add(ping(m.wave, hz, ring, 0))
		}
	}
	if c%rhythm.GridPerBeat != 0 {
		return
	}
	m.beatAt = 0
	beat := stepLen * rhythm.GridPerBeat
	for k := range m.perBeat {
		d := c/rhythm.GridPerBeat*m.perBeat + k
		for v, on := range m.drum[d] {
			if on {
				m.add(drum(v, int(math.Round(beat*float64(k)/float64(m.perBeat)))))
			}
		}
	}
}

func (m *matrix) add(v voice) {
	if len(m.voices) < maxVoices {
		m.voices = append(m.voices, v)
	}
}

// matrixView is what the Matrix's picture shows of it now.
type matrixView struct {
	col, lane, beat int // the grid column, lane step and beat sounding, -1 for none
	perBeat, beats  int
	steps           int
	edits           int
}

// view is the Matrix as its picture shows it.
func (m *matrix) view() matrixView {
	v := matrixView{col: -1, lane: -1, beat: -1, perBeat: m.perBeat, beats: m.beats, steps: m.steps, edits: m.edits}
	if !m.playing() || m.col < 0 {
		return v
	}
	v.col = m.col
	b := m.col / rhythm.GridPerBeat
	v.beat = b % max(1, m.beats)
	sub := m.stepSeconds() * sampleRate * rhythm.GridPerBeat / float64(m.perBeat)
	v.lane = b*m.perBeat + min(m.perBeat-1, int(m.beatAt/sub))
	return v
}

// The Matrix's controls, the drums' level and their preset among them: its
// module is "matrix" in the catalog.
func isMatrixControl(id string) bool {
	switch id {
	case "tm-tempo", "tm-steps", "tm-root", "tm-lvl", "tm-wave", "tm-run", "rhythm-lvl", "rhythm-preset":
		return true
	}
	return false
}

// matrixSet reads one of the Matrix's controls into it, and says what other
// control follows: a preset moves the steps knob to a whole number of its
// bars. Called with r.mu and the sound's lock held.
func (r *Rack) matrixSet(id string) (more [][2]string) {
	m := &r.sound.mat
	switch id {
	case "tm-tempo":
		m.bpm = r.number(id)
		if m.bpm < 40 {
			m.bpm = 120
		}
	case "tm-steps":
		m.steps = int(r.number(id))
		if m.steps < 1 || m.steps > rhythm.GridSteps {
			m.steps = 16
		}
		if m.step >= m.steps {
			m.step = 0
		}
	case "tm-root":
		m.root = rhythm.RootMidi(int(r.number(id)))
	case "tm-lvl":
		m.lvl = r.number(id) / 100
	case "tm-wave":
		m.wave = int(r.number(id))
	case "rhythm-lvl":
		m.dlvl = r.number(id) / 100
	case "tm-run":
		m.run = r.value(id) == "1"
		m.until = 0 // start cleanly rather than racing to catch up
		if !m.run {
			m.col = -1
		}
	case "rhythm-preset":
		if p, ok := rhythm.ByName(r.value(id)); ok {
			if n := m.loadDrums(p); strconv.Itoa(n) != r.value("tm-steps") {
				more = append(more, [2]string{"tm-steps", strconv.Itoa(n)})
			}
		}
	}
	return more
}
