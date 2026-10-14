//go:build !js

// Package hostaudio plays a stereo source on the machine's speakers through
// PulseAudio (or PipeWire's PulseAudio server), with no cgo: the native
// rack's sound, which the page plays through Web Audio.
package hostaudio

import (
	"fmt"
	"sync"

	"github.com/jfreymuth/pulse"
)

// latency is how far ahead of the speakers the stream is filled: short
// enough that a knob is heard turning, long enough not to underrun while
// the panel draws.
const latency = 0.05

// Player plays what fill makes, from the first Play to Close. It connects to
// the sound server on the first Play, so a rack that is never heard never
// opens a stream.
//
// A pause closes the stream, and the next Play opens another: this client's
// streams cannot be paused and carried on with. Resumed, a stream sits
// uncorked and silent, since it threw away the server's requests for samples
// while it was paused and the server waits for them still; stopped and
// started again, it is fed twice what it plays, and the rack's clock runs
// ahead of the speakers with the server dropping the rest.
type Player struct {
	fill func(l, r []float32)
	rate int

	mu     sync.Mutex
	client *pulse.Client
	stream *pulse.PlaybackStream
}

// New is a player of fill, which makes len(l) samples a side at rate.
func New(fill func(l, r []float32), rate int) *Player {
	return &Player{fill: fill, rate: rate}
}

// Play starts the sound, or carries on with it.
func (p *Player) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stream != nil && p.stream.Running() {
		return nil
	}
	if p.stream != nil {
		// Closed under it: the server went away, or the reader stopped.
		err := p.stream.Error()
		p.drop()
		if err != nil {
			return fmt.Errorf("the sound stream stopped: %w", err)
		}
	}
	if p.client == nil {
		c, err := pulse.NewClient(pulse.ClientApplicationName("chaosrack"))
		if err != nil {
			return fmt.Errorf("no sound server: %w", err)
		}
		p.client = c
	}
	s, err := p.client.NewPlayback(pulse.Float32Reader(p.reader()),
		pulse.PlaybackStereo, pulse.PlaybackSampleRate(p.rate),
		pulse.PlaybackLatency(latency), pulse.PlaybackMediaName("the rack"))
	if err != nil {
		p.client.Close()
		p.client = nil
		return fmt.Errorf("no sound stream: %w", err)
	}
	p.stream = s
	s.Start()
	return nil
}

// Pause silences the sound at once.
func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.drop()
}

// drop closes the stream, if there is one. Called with p.mu held.
func (p *Player) drop() {
	if p.stream != nil {
		p.stream.Close()
		p.stream = nil
	}
}

// Close stops the sound and lets go of the sound server.
func (p *Player) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.drop()
	if p.client != nil {
		p.client.Close()
		p.client = nil
	}
}

// reader is a stream's reader: it fills the stream's buffer, interleaved
// left and right. It runs on the stream's own goroutine and takes none of
// the player's locks, and has buffers of its own, since a stream being
// closed can still be reading while the next one starts.
func (p *Player) reader() func(buf []float32) (int, error) {
	var l, r []float32
	return func(buf []float32) (int, error) {
		n := len(buf) / 2
		if cap(l) < n {
			l, r = make([]float32, n), make([]float32, n)
		}
		l, r = l[:n], r[:n]
		p.fill(l, r)
		for i := range n {
			buf[2*i], buf[2*i+1] = clip(l[i]), clip(r[i])
		}
		return 2 * n, nil
	}
}

// clip holds a sample to the speakers' range: four generators at full level
// on one side sum to four.
func clip(v float32) float32 {
	return max(-1, min(1, v))
}
