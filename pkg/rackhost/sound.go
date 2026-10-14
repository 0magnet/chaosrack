package rackhost

import (
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
)

// The rack's sound: the four signal generators, made here as the page's
// rack signal makes them (audiosrc.FuncGen), Model Out, the model's own
// equations run at audio rate, the Matrix and its drums (matrix.go), and the
// Mixer's speaker pins that put each on the left or the right. What plays
// the samples is the host's (a Speaker); the rack only makes them.

// Speaker is what plays the rack: told when there is something to hear, it
// asks the rack for samples (Fill) at the rack's SampleRate.
type Speaker interface {
	Play() error
	Pause()
}

// genIDs are the generators' control prefixes, Gen 1 to 4, in the
// oscillators' order.
var genIDs = [audiosrc.OscCount]string{"gen-x", "gen-y", "gen-z", "gen-v"}

// The Mixer's columns the rack makes sound for: the four generators, the
// model's x, y and z, and the Matrix's pads and drums.
var mixKeys = [...]string{"g1", "g2", "g3", "g4", "mx", "my", "mz", "tm", "dr"}

const (
	srcModel = audiosrc.OscCount // the first of the model's columns
	srcTM    = srcModel + 3 + busTM
	srcDR    = srcModel + 3 + busDR
	nSources = len(mixKeys)
)

// spkRows are the Mixer's speaker rows, left and right.
var spkRows = [2]string{"sl", "sr"}

// sampleRate is the rate the rack's sound is made at.
const sampleRate = 48000

// levelGlide is how long a level change takes to arrive, in seconds: the
// page's speakers glide to a new gain (setTargetAtTime) rather than jump to
// it, which would click.
const levelGlide = 0.01

// sound is what the rack makes and where it goes. Its lock is taken after
// r.mu and after the scene's, never before either.
type sound struct {
	mu    sync.Mutex
	fg    *audiosrc.FuncGen
	level [audiosrc.OscCount]float64 // each generator's level, 0..1
	gain  [audiosrc.OscCount]float64 // the level as it glides there
	on    [audiosrc.OscCount]bool    // its wave is not off
	// spk is each column's pin to the left and right speaker: the Mixer's
	// SPK rows, as the mix-sl-* and mix-sr-* controls hold them.
	spk     [2][nSources]float64
	model   modelVoice
	speaker Speaker
	err     error // why the speaker could not play
	mat     matrix
	vals    [audiosrc.OscCount]float64
	mo      [3][]float32
	bus     [2][]float32 // the Matrix's pads and drums, a buffer's worth
}

func newSound() *sound {
	fg := audiosrc.NewFuncGen()
	fg.SetSampleRate(sampleRate)
	for i := range audiosrc.OscCount {
		// The level is applied here, where it can glide.
		fg.SetAmp(i, 1)
	}
	return &sound{fg: fg, mat: newMatrix()}
}

// genOf is the generator a control belongs to and the part of it (freq,
// lvl, wave, atk, dcy, env), or -1.
func genOf(id string) (int, string) {
	for i, p := range genIDs {
		if rest, ok := strings.CutPrefix(id, p+"-"); ok {
			return i, rest
		}
	}
	return -1, ""
}

// pinOf is the speaker side and the column of a Mixer pin control the rack
// makes sound for, or ok false.
func pinOf(id string) (side, src int, ok bool) {
	for side, row := range spkRows {
		for src, k := range mixKeys {
			if id == "mix-"+row+"-"+k {
				return side, src, true
			}
		}
	}
	return 0, 0, false
}

// pinID is the control of column src's pin to speaker side.
func pinID(side, src int) string { return "mix-" + spkRows[side] + "-" + mixKeys[src] }

// soundSet puts a control's new value into the sound, if the sound uses it,
// and says which other controls to set as well, to what: a generator switched
// on and turned while it is pinned nowhere is pinned, as the page's Mixer
// does it (mixAutoPin), Gen 1 and 3 on the left and 2 and 4 on the right; a
// rhythm preset moves the Matrix's steps. Called with r.mu held.
func (r *Rack) soundSet(id string) (more [][2]string) {
	s := r.sound
	s.mu.Lock()
	defer s.mu.Unlock()
	switch i, part := genOf(id); {
	case isMatrixControl(id):
		more = r.matrixSet(id)
		if id == "tm-run" && s.mat.run {
			// Run switched on is a hand on the Matrix, which starts it.
			s.mat.armed = true
		}
	case id == "gen-solo":
		solo := -1
		for j, p := range genIDs {
			if p == "gen-"+r.value(id) {
				solo = j
			}
		}
		s.fg.SetSolo(solo)
	case i >= 0:
		r.genSet(i, part)
		if s.on[i] && s.level[i] > 0 && s.spk[0][i] == 0 && s.spk[1][i] == 0 {
			more = append(more, [2]string{pinID(i%2, i), "1"})
		}
	case id == r.selector || id == "mo-spd" || id == "mo-lvl":
		r.modelSet()
	default:
		if side, src, ok := pinOf(id); ok {
			s.spk[side][src] = r.number(id)
		}
	}
	return more
}

// genSet reads part of generator i off its control. Called with r.mu and
// the sound's lock held.
func (r *Rack) genSet(i int, part string) {
	s, id := r.sound, genIDs[i]
	switch part {
	case "freq":
		s.fg.SetFreq(i, freqFromKnob(r.number(id+"-freq")))
	case "lvl":
		s.level[i] = r.number(id+"-lvl") / 100
	case "wave":
		v := r.value(id + "-wave")
		s.on[i] = v != "off"
		s.fg.SetOn(i, s.on[i])
		if w, err := strconv.Atoi(v); err == nil {
			s.fg.SetWave(i, w)
		}
	case "atk", "dcy", "env":
		s.fg.SetEnv(i, r.value(id+"-env") == "rpt", r.number(id+"-atk")/1000, r.number(id+"-dcy")/1000)
	}
}

// modelSet reads Model Out off its knobs and the model selector. Called with
// r.mu and the sound's lock held.
func (r *Rack) modelSet() {
	m := &r.sound.model
	m.want = r.value(r.selector)
	m.speed = math.Pow(2, r.number("mo-spd")/12)
	m.level = r.number("mo-lvl") / 100
}

// soundStart puts every control the sound uses into it. Called with r.mu
// held.
func (r *Rack) soundStart() {
	s := r.sound
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range genIDs {
		for _, part := range []string{"freq", "lvl", "wave", "env"} {
			r.genSet(i, part)
		}
		s.gain[i] = s.level[i]
	}
	for side := range spkRows {
		for src := range mixKeys {
			s.spk[side][src] = r.number(pinID(side, src))
		}
	}
	r.modelSet()
	// The Matrix as the catalog opened it, preset and all; the steps were
	// captured already fitted to the preset.
	for _, id := range []string{"tm-tempo", "tm-steps", "tm-root", "tm-lvl", "tm-wave", "tm-run", "rhythm-lvl", "rhythm-preset"} {
		r.matrixSet(id)
	}
}

// pinned reports whether column src is on either speaker.
func (s *sound) pinned(src int) bool { return s.spk[0][src] != 0 || s.spk[1][src] != 0 }

// heard reports whether anything reaches a speaker: a generator switched
// on, let through by the solo and pinned to one, a model with equations to
// play whose output is pinned, or a Matrix running with pads or drum steps
// lit and pinned. Called with the sound's lock held.
func (s *sound) heard() bool {
	for i := range genIDs {
		if s.on[i] && s.fg.Audible(i) && s.pinned(i) {
			return true
		}
	}
	if s.model.plays() {
		for c := range 3 {
			if s.pinned(srcModel + c) {
				return true
			}
		}
	}
	if m := &s.mat; m.playing() {
		pads, drums := m.lit()
		if pads && m.lvl > 0 && s.pinned(srcTM) || drums && m.dlvl > 0 && s.pinned(srcDR) {
			return true
		}
	}
	return false
}

// tell tells the speaker whether there is anything to hear. It takes the
// sound's lock only to look, and is called without it: a speaker starting
// to play asks for samples (Fill), which need the lock.
func (s *sound) tell() {
	s.mu.Lock()
	sp, heard := s.speaker, s.heard()
	s.mu.Unlock()
	if sp == nil {
		return
	}
	if !heard {
		sp.Pause()
		return
	}
	err := sp.Play()
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

// SetSpeaker gives the rack something to play its sound on.
func (r *Rack) SetSpeaker(sp Speaker) {
	s := r.sound
	s.mu.Lock()
	s.speaker = sp
	s.mu.Unlock()
	s.tell()
}

// SoundErr is why the rack cannot be heard, when it cannot.
func (r *Rack) SoundErr() error {
	s := r.sound
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// SampleRate is the rate Fill makes samples at.
func (r *Rack) SampleRate() int { return sampleRate }

// Fill makes the next len(left) samples of the speakers.
func (r *Rack) Fill(left, right []float32) {
	s := r.sound
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(left)
	modelOn := false
	for c := range 3 {
		modelOn = modelOn || s.pinned(srcModel+c)
	}
	if modelOn {
		s.model.render(&s.mo, n)
	}
	matOn := s.mat.playing() || len(s.mat.voices) > 0
	if matOn {
		for b := range s.bus {
			if cap(s.bus[b]) < n {
				s.bus[b] = make([]float32, n)
			}
			s.bus[b] = s.bus[b][:n]
		}
		s.mat.render(s.bus[busTM], s.bus[busDR], n)
	}
	k := 1 - math.Exp(-1/(levelGlide*sampleRate))
	for j := range n {
		s.fg.Next(&s.vals)
		var l, rt float64
		for i, v := range s.vals {
			s.gain[i] += (s.level[i] - s.gain[i]) * k
			v *= s.gain[i]
			l += v * s.spk[0][i]
			rt += v * s.spk[1][i]
		}
		if modelOn {
			for c := range 3 {
				v := float64(s.mo[c][j])
				l += v * s.spk[0][srcModel+c]
				rt += v * s.spk[1][srcModel+c]
			}
		}
		if matOn {
			for b, src := range [2]int{srcTM, srcDR} {
				v := float64(s.bus[b][j])
				l += v * s.spk[0][src]
				rt += v * s.spk[1][src]
			}
		}
		left[j], right[j] = float32(l), float32(rt)
	}
}

// freqFromKnob is a generator's frequency knob in hertz: the knob counts
// semitones up from A0 over ten octaves, as the page's does
// (attractor.freqFromKnob).
func freqFromKnob(v float64) float64 {
	const lo, hi, semitones = 27.5, 28160.0, 120
	return lo * math.Pow(hi/lo, v/semitones)
}
