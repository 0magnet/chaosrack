package rackhost

import (
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
)

// The rack's sound: the four signal generators, made here as the page's
// rack signal makes them (audiosrc.FuncGen), and the Mixer's speaker pins
// that put each one on the left or the right. What plays the samples is the
// host's (a Speaker); the rack only makes them.

// Speaker is what plays the rack: told when there is something to hear, it
// asks the rack for samples (Fill) at the rack's SampleRate.
type Speaker interface {
	Play() error
	Pause()
}

// genIDs are the generators' control prefixes, Gen 1 to 4, in the
// oscillators' order.
var genIDs = [audiosrc.OscCount]string{"gen-x", "gen-y", "gen-z", "gen-v"}

// sampleRate is the rate the rack's sound is made at.
const sampleRate = 48000

// levelGlide is how long a level change takes to arrive, in seconds: the
// page's speakers glide to a new gain (setTargetAtTime) rather than jump to
// it, which would click.
const levelGlide = 0.01

// sound is the generators and where they go.
type sound struct {
	mu    sync.Mutex
	fg    *audiosrc.FuncGen
	level [audiosrc.OscCount]float64 // each generator's level, 0..1
	gain  [audiosrc.OscCount]float64 // the level as it glides there
	on    [audiosrc.OscCount]bool    // its wave is not off
	// spk is each generator's pin to the left and right speaker: the
	// Mixer's SPK rows for the gen columns.
	spk     [2][audiosrc.OscCount]float64
	speaker Speaker
	err     error // why the speaker could not play
	vals    [audiosrc.OscCount]float64
}

func newSound() *sound {
	fg := audiosrc.NewFuncGen()
	fg.SetSampleRate(sampleRate)
	for i := range audiosrc.OscCount {
		// The level is applied here, where it can glide.
		fg.SetAmp(i, 1)
	}
	return &sound{fg: fg}
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

// soundSet puts a control's new value into the sound, if it is one of the
// generators'. Called with r.mu held.
func (r *Rack) soundSet(id string) {
	s := r.sound
	i, part := genOf(id)
	if id != "gen-solo" && i < 0 {
		return
	}
	s.mu.Lock()
	defer s.tell()
	defer s.mu.Unlock()
	if id == "gen-solo" {
		solo := -1
		for j, p := range genIDs {
			if p == "gen-"+r.value(id) {
				solo = j
			}
		}
		s.fg.SetSolo(solo)
	} else {
		r.genSet(i, part)
		// A generator switched on and turned while it is pinned nowhere is
		// pinned, as the page's Mixer does it (mixAutoPin): Gen 1 and 3 on
		// the left, 2 and 4 on the right.
		if s.on[i] && s.level[i] > 0 && s.spk[0][i] == 0 && s.spk[1][i] == 0 {
			s.spk[i%2][i] = 1
		}
	}
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

// soundStart puts every generator control's value into the sound. Called
// with r.mu held.
func (r *Rack) soundStart() {
	s := r.sound
	s.mu.Lock()
	for i := range genIDs {
		for _, part := range []string{"freq", "lvl", "wave", "env"} {
			r.genSet(i, part)
		}
		s.gain[i] = s.level[i]
	}
	s.mu.Unlock()
}

// heard reports whether any generator reaches a speaker: switched on, let
// through by the solo, and pinned to one. Called with the sound's lock held.
func (s *sound) heard() bool {
	for i := range genIDs {
		if s.on[i] && s.fg.Audible(i) && (s.spk[0][i] != 0 || s.spk[1][i] != 0) {
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
	k := 1 - math.Exp(-1/(levelGlide*sampleRate))
	for n := range left {
		s.fg.Next(&s.vals)
		var l, rt float64
		for i, v := range s.vals {
			s.gain[i] += (s.level[i] - s.gain[i]) * k
			v *= s.gain[i]
			l += v * s.spk[0][i]
			rt += v * s.spk[1][i]
		}
		left[n], right[n] = float32(l), float32(rt)
	}
}

// freqFromKnob is a generator's frequency knob in hertz: the knob counts
// semitones up from A0 over ten octaves, as the page's does
// (attractor.freqFromKnob).
func freqFromKnob(v float64) float64 {
	const lo, hi, semitones = 27.5, 28160.0, 120
	return lo * math.Pow(hi/lo, v/semitones)
}
