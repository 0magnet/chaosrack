package rackhost

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/audiosrc"
)

// The rack's sound: the four signal generators, made here as the page's
// rack signal makes them (audiosrc.FuncGen), Model Out, the model's own
// equations run at audio rate, the Matrix and its drums (matrix.go), the Keys
// (keys.go), and the Mixer's speaker pins that put each on the left or the right. What plays
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
// model's x, y and z, the Matrix's pads and drums, the Keys, and the games' blips (SOUNDS).
var mixKeys = [...]string{"g1", "g2", "g3", "g4", "mx", "my", "mz", "tm", "dr", "ky", "fx"}

const (
	srcModel = audiosrc.OscCount // the first of the model's columns
	srcTM    = srcModel + 3 + busTM
	srcDR    = srcModel + 3 + busDR
	srcKY    = srcDR + 1
	srcFX    = srcKY + 1
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
	// wob is the FVF wobbulator, and listen whether its Listen switch plays
	// the capture through it into the FX column (modelsw.go).
	wob    *attractor.Wobbulator
	listen struct {
		on, shown bool
		proc      *attractor.FVFProcessor
	}
	vals  [audiosrc.OscCount]float64
	mo    [3][]float32
	bus   [2][]float32 // the Matrix's pads and drums, a buffer's worth
	keys  keyBank
	ky    []float32 // the Keys, a buffer's worth
	games games
	fx    []float32 // the games' blips, a buffer's worth
	// touched is a hand having been on the rack, which lets the ball's
	// blips be heard, as a gesture lets the page's audio start.
	touched bool
	// rack is the rack's signal (bus.go), capt the capture it mixes, and
	// capL, capR a buffer's worth of it.
	rack       rackBus
	capt       capture
	capL, capR []float32
	// now is the clock; filled is when the speaker last asked for samples,
	// made when the rack's signal was last made up to; idle is where a look
	// makes it while the speakers are idle.
	now          func() time.Time
	filled, made time.Time
	idle         [2][]float32
	// told is whether the speaker was last told there is something to hear.
	told bool
}

func newSound() *sound {
	fg := audiosrc.NewFuncGen()
	fg.SetSampleRate(sampleRate)
	for i := range audiosrc.OscCount {
		// The level is applied here, where it can glide.
		fg.SetAmp(i, 1)
	}
	s := &sound{fg: fg, mat: newMatrix(), keys: newKeyBank(), games: newGames(), wob: attractor.NewWobbulator(), now: time.Now}
	s.wireGames()
	return s
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
	case isKeysControl(id):
		more = r.keysSet(id)
	case isGameControl(id):
		r.gameSet(id)
	case strings.HasPrefix(id, "model-sw-"):
		more = r.modelSwitchSet(id)
	case strings.HasPrefix(id, "fvf-"):
		s.wob.Set(id, float32(r.number(id)))
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
	case id == r.selector || isFamily(id) || id == "mo-spd" || id == "mo-lvl":
		r.modelSet()
		// Listen plays only while FVF, whose switch it is, is on show.
		s.listen.shown = r.model() == "fvf"
	default:
		if side, src, ok := pinOf(id); ok {
			s.spk[side][src] = r.number(id)
		} else {
			r.rackPinSet(id)
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
	m.want = r.model()
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
	for _, row := range rackRows {
		for _, k := range append(mixKeys[:], capKeys[:]...) {
			r.rackPinSet("mix-" + row + "-" + k)
		}
	}
	r.modelSet()
	// The Matrix as the catalog opened it, preset and all; the steps were
	// captured already fitted to the preset.
	for _, id := range []string{"tm-tempo", "tm-steps", "tm-root", "tm-lvl", "tm-wave", "tm-run", "rhythm-lvl", "rhythm-preset"} {
		r.matrixSet(id)
	}
	// The Synth bay as the catalog opened it: the preset's knobs are set
	// already, so the preset itself is not applied again.
	for _, c := range r.ctls {
		if isKeysControl(c.ID) && c.ID != "syn-preset" {
			r.keysSet(c.ID)
		}
		if isGameControl(c.ID) && c.ID != "model-sw-0" && !strings.HasPrefix(c.ID, "pong-pad-") {
			r.gameSet(c.ID)
		}
	}
}

// pinned reports whether column src is on either speaker.
func (s *sound) pinned(src int) bool { return s.spk[0][src] != 0 || s.spk[1][src] != 0 }

// heard reports whether anything reaches a speaker: a generator switched
// on, let through by the solo and pinned to one, a model with equations to
// play whose output is pinned, a Matrix running with pads or drum steps
// lit and pinned, or a key sounding, while the Keys are pinned. Called with the sound's lock held.
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
	if s.games.heard(s.touched) && s.pinned(srcFX) {
		return true
	}
	if s.listening() && s.pinned(srcFX) {
		return true
	}
	return len(s.keys.voices) > 0 && s.keys.lvl > 0 && s.pinned(srcKY)
}

// tell tells the speaker whether there is anything to hear. It takes the
// sound's lock only to look, and is called without it: a speaker starting
// to play asks for samples (Fill), which need the lock.
func (s *sound) tell() {
	s.mu.Lock()
	sp, heard := s.speaker, s.heard()
	s.told = heard
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

// settle tells the speaker again if what is heard has changed with no hand
// on the rack: a key let go rings on after it, and stops by itself. Called
// without the sound's lock, each time the panel looks.
func (s *sound) settle() {
	s.mu.Lock()
	changed := s.speaker != nil && s.heard() != s.told
	s.mu.Unlock()
	if changed {
		s.tell()
	}
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
	if s.err == nil && s.capt.err != nil {
		return fmt.Errorf("the capture: %w", s.capt.err)
	}
	return s.err
}

// SampleRate is the rate Fill makes samples at.
func (r *Rack) SampleRate() int { return sampleRate }

// Fill makes the next len(left) samples of the speakers.
func (r *Rack) Fill(left, right []float32) {
	s := r.sound
	s.mu.Lock()
	defer s.mu.Unlock()
	s.render(left, right)
	s.filled = s.now()
	s.made = s.filled
}

// uses reports whether column src is heard or in the rack's signal.
func (s *sound) uses(src int) bool {
	return s.pinned(src) || s.rack.gain[0][src] != 0 || s.rack.gain[1][src] != 0
}

// grown is b at n samples, made bigger if it must be.
func grown(b []float32, n int) []float32 {
	if cap(b) < n {
		return make([]float32, n)
	}
	return b[:n]
}

// render makes the next len(left) samples of every source: the speakers'
// mix into left and right, and the rack's signal into its ring. Called with
// the sound's lock held.
func (s *sound) render(left, right []float32) {
	n := len(left)
	modelOn := false
	for c := range 3 {
		modelOn = modelOn || s.uses(srcModel+c)
	}
	if modelOn {
		s.model.render(&s.mo, n)
	}
	matOn := s.mat.playing() || len(s.mat.voices) > 0
	if matOn {
		for b := range s.bus {
			s.bus[b] = grown(s.bus[b], n)
		}
		s.mat.render(s.bus[busTM], s.bus[busDR], n)
	}
	keysOn := len(s.keys.voices) > 0
	if keysOn {
		s.ky = grown(s.ky, n)
		s.keys.render(s.ky, n)
	}
	busOn := s.rack.on()
	listening := s.listening()
	s.capL, s.capR = grown(s.capL, n), grown(s.capR, n)
	if busOn && s.rack.capOn() || listening {
		s.capt.take(s.capL, s.capR, n)
	} else {
		clear(s.capL)
		clear(s.capR)
	}
	fxOn := len(s.games.blips) > 0 || listening
	if fxOn {
		s.fx = grown(s.fx, n)
		s.games.render(s.fx, n)
		if listening {
			s.listenInto(s.fx, n)
		}
	}
	k := 1 - math.Exp(-1/(levelGlide*sampleRate))
	for j := range n {
		var v [nSources]float64
		s.fg.Next(&s.vals)
		for i, x := range s.vals {
			s.gain[i] += (s.level[i] - s.gain[i]) * k
			v[i] = x * s.gain[i]
		}
		if modelOn {
			for c := range 3 {
				v[srcModel+c] = float64(s.mo[c][j])
			}
		}
		if matOn {
			v[srcTM], v[srcDR] = float64(s.bus[busTM][j]), float64(s.bus[busDR][j])
		}
		if keysOn {
			v[srcKY] = float64(s.ky[j])
		}
		if fxOn {
			v[srcFX] = float64(s.fx[j])
		}
		var l, rt float64
		for i, x := range v {
			l += x * s.spk[0][i]
			rt += x * s.spk[1][i]
		}
		left[j], right[j] = float32(l), float32(rt)
		if busOn {
			s.rack.push(&v, s.capL[j], s.capR[j])
		}
	}
}

// freqFromKnob is a generator's frequency knob in hertz: the knob counts
// semitones up from A0 over ten octaves, as the page's does
// (attractor.freqFromKnob).
func freqFromKnob(v float64) float64 {
	const lo, hi, semitones = 27.5, 28160.0, 120
	return lo * math.Pow(hi/lo, v/semitones)
}
