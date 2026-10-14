package rackhost

import (
	"sync"
	"time"

	"github.com/0magnet/chaosrack/pkg/attractor"
)

// The rack's signal, RACK L and R: every source the rack makes, and the
// capture, mixed by the Mixer's RACK rows, as the page's bus (audiosrc.Bus)
// mixes them for everything that analyzes the rack. The audio models draw
// it. It is made with the speakers' samples while they play, and on the
// clock while they do not, so a model of the capture moves with nothing to
// hear.

// rackRows are the Mixer's RACK rows, left and right.
var rackRows = [2]string{"rl", "rr"}

// capKeys are the Mixer's capture columns, left and right: on RACK rows only,
// never on the speakers.
var capKeys = [2]string{"cl", "cr"}

// busRing is how much of the rack's signal is kept: more than any audio
// model's window, 500 ms and two delays of 512 samples.
const busRing = 2 * sampleRate

// maxIdleMake is the most of the signal one look makes while the speakers are
// idle: a panel that looked away does not come back to seconds of work.
const maxIdleMake = sampleRate / 2

// rackBus is the rack's signal: its gains and the recent stretch of it.
type rackBus struct {
	gain [2][nSources]float64 // each column's gain on RACK L and R
	cap  [2][2]float64        // the capture's, CAP L and R, on each
	ring [2][]float32
	at   int // where the next sample goes
	n    int // how many samples the ring holds
}

// on reports whether anything is in the mix.
func (b *rackBus) on() bool { return b.capOn() || b.uses() }

// uses reports whether a column the rack makes is in the mix.
func (b *rackBus) uses() bool {
	for side := range b.gain {
		for _, g := range b.gain[side] {
			if g != 0 {
				return true
			}
		}
	}
	return false
}

// capOn reports whether the capture is in the mix.
func (b *rackBus) capOn() bool { return b.cap != [2][2]float64{} }

// push adds a sample of the mix of v and the capture's cl and cr.
func (b *rackBus) push(v *[nSources]float64, cl, cr float32) {
	if b.ring[0] == nil {
		b.ring = [2][]float32{make([]float32, busRing), make([]float32, busRing)}
	}
	for side := range b.ring {
		x := b.cap[side][0]*float64(cl) + b.cap[side][1]*float64(cr)
		for i, g := range b.gain[side] {
			if g != 0 {
				x += g * v[i]
			}
		}
		b.ring[side][b.at] = float32(x)
	}
	b.at = (b.at + 1) % busRing
	b.n = min(b.n+1, busRing)
}

// window is the newest n samples of each side, oldest first, or as many as
// there are.
func (b *rackBus) window(n int) (l, r []float32) {
	n = min(n, b.n)
	l, r = make([]float32, n), make([]float32, n)
	for k := range n {
		i := (b.at - n + k + busRing) % busRing
		l[k], r[k] = b.ring[0][i], b.ring[1][i]
	}
	return l, r
}

// capture is the machine's audio coming in, as the page's server captures it
// (pkg/audiocap): stereo at the rack's rate, kept a second deep, the newest
// kept when the rack falls behind.
type capture struct {
	mu   sync.Mutex
	fifo [2][]float32
	stop func()
	err  error
}

// captureDepth is how much of the capture waits to be mixed.
const captureDepth = sampleRate

// startCapture is how a capture starts (hostCapture). Tests replace it.
var startCapture = hostCapture

// write takes a chunk of the capture, interleaved left and right.
func (c *capture) write(p []float32) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := 0; i+1 < len(p); i += 2 {
		c.fifo[0] = append(c.fifo[0], p[i])
		c.fifo[1] = append(c.fifo[1], p[i+1])
	}
	if over := len(c.fifo[0]) - captureDepth; over > 0 {
		c.fifo[0] = append(c.fifo[0][:0], c.fifo[0][over:]...)
		c.fifo[1] = append(c.fifo[1][:0], c.fifo[1][over:]...)
	}
	return nil
}

// take moves n samples of the capture into l and r: what has come in, the
// rest silence.
func (c *capture) take(l, r []float32, n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := min(n, len(c.fifo[0]))
	copy(l, c.fifo[0][:k])
	copy(r, c.fifo[1][:k])
	clear(l[k:n])
	clear(r[k:n])
	c.fifo[0] = append(c.fifo[0][:0], c.fifo[0][k:]...)
	c.fifo[1] = append(c.fifo[1][:0], c.fifo[1][k:]...)
}

// running reports whether the capture is open.
func (c *capture) running() bool { return c.stop != nil }

// close stops the capture.
func (c *capture) close() {
	if c.stop != nil {
		c.stop()
		c.stop = nil
	}
	c.mu.Lock()
	c.fifo = [2][]float32{}
	c.mu.Unlock()
}

// needCapture opens the capture while an audio model draws a mix it is in,
// and closes it otherwise: the machine's audio is listened to only while
// something shows it. Called with r.mu held.
func (r *Rack) needCapture() {
	s := r.sound
	// Held throughout: opening or closing a record waits on nothing that
	// takes it, since the capture's writer takes only its own lock.
	s.mu.Lock()
	defer s.mu.Unlock()
	want := s.rack.capOn() && attractor.IsAudioModel(r.value(r.selector))
	c := &s.capt
	switch {
	case want && !c.running() && c.err == nil:
		stop, err := startCapture(r.audioSource, c.write)
		if err != nil {
			// Reported once on the status line, and not retried every look.
			c.err = err
			return
		}
		c.stop = stop
	case !want && c.running():
		c.close()
	case !want:
		c.err = nil
	}
}

// busLook makes the rack's signal up to now when the speakers are idle, as
// the page's bus is drained each frame whatever is heard. Called without the
// sound's lock.
func (s *sound) busLook() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !s.rack.on() || now.Sub(s.filled) < 100*time.Millisecond {
		s.made = now
		return
	}
	n := min(int(now.Sub(s.made).Seconds()*sampleRate), maxIdleMake)
	s.made = now
	if n <= 0 {
		return
	}
	if cap(s.idle[0]) < n {
		s.idle = [2][]float32{make([]float32, n), make([]float32, n)}
	}
	s.render(s.idle[0][:n], s.idle[1][:n])
}

// audioFigure is audio model key drawn from the newest of the rack's signal,
// as the page draws its newest window, and the frame it is drawn in: the
// full-scale cube for an embedding, and the scope's screen, at its gain, for
// the XY. Called with r.mu held.
func (r *Rack) audioFigure(key string) (attractor.Figure, [][3]float64) {
	o := attractor.AudioOptions{SampleRate: sampleRate}
	switch key {
	case "takens", "polar":
		o.Tau = float32(r.number("takens-tau"))
	case "stereo":
		o.Tau = float32(r.number("stereo-tau"))
	}
	o.WindowMS = float32(r.number(key + "-win"))
	need := attractor.AudioWindow(key, o)
	s := r.sound
	s.mu.Lock()
	l, rt := s.rack.window(need)
	s.mu.Unlock()
	fig, ok := attractor.AudioFigure(key, l, rt, len(l), o)
	if !ok {
		fig = attractor.Figure{Kind: attractor.FigurePath}
	}
	e := 1.0
	if key == "xy" {
		if g := r.number("xy-gain"); g > 0 {
			e = 1 / g
		}
		return fig, [][3]float64{{-e, -e, 0}, {e, e, 0}}
	}
	return fig, [][3]float64{{-e, -e, -e}, {e, e, e}}
}

// rackPinSet reads a pin of the Mixer's RACK rows into the rack's signal.
// Called with r.mu and the sound's lock held.
func (r *Rack) rackPinSet(id string) {
	b := &r.sound.rack
	for side, row := range rackRows {
		for src, k := range mixKeys {
			if id == "mix-"+row+"-"+k {
				b.gain[side][src] = r.number(id)
			}
		}
		for c, k := range capKeys {
			if id == "mix-"+row+"-"+k {
				b.cap[side][c] = r.number(id)
			}
		}
	}
}

// SetAudioSource sets what the capture records: "monitor" (what is playing,
// the default), "default" (the input), or a PulseAudio source's ID. A
// capture already open keeps its source until it is next opened.
func (r *Rack) SetAudioSource(source string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if source == "" {
		source = "monitor"
	}
	r.audioSource = source
	r.sound.mu.Lock()
	r.sound.capt.err = nil
	r.sound.mu.Unlock()
}
