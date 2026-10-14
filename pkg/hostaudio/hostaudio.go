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
type Player struct {
	fill func(l, r []float32)
	rate int

	mu     sync.Mutex
	client *pulse.Client
	stream *pulse.PlaybackStream
	l, r   []float32
}

// New is a player of fill, which makes len(l) samples a side at rate.
func New(fill func(l, r []float32), rate int) *Player {
	return &Player{fill: fill, rate: rate}
}

// Play starts the sound, or carries on with it.
func (p *Player) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stream == nil {
		c, err := pulse.NewClient(pulse.ClientApplicationName("chaosrack"))
		if err != nil {
			return fmt.Errorf("no sound server: %w", err)
		}
		s, err := c.NewPlayback(pulse.Float32Reader(p.read),
			pulse.PlaybackStereo, pulse.PlaybackSampleRate(p.rate),
			pulse.PlaybackLatency(latency), pulse.PlaybackMediaName("the rack"))
		if err != nil {
			c.Close()
			return fmt.Errorf("no sound stream: %w", err)
		}
		p.client, p.stream = c, s
	}
	if p.stream.Running() {
		return nil
	}
	if p.stream.Closed() {
		return fmt.Errorf("the sound server went away: %w", p.stream.Error())
	}
	// Start the first time, Resume after a Pause; each does nothing in the
	// other's state.
	p.stream.Resume()
	p.stream.Start()
	return nil
}

// Pause silences the sound at once, keeping the stream for the next Play.
func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stream != nil {
		p.stream.Pause()
	}
}

// Close stops the sound and lets go of the sound server.
func (p *Player) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stream != nil {
		p.stream.Close()
		p.client.Close()
		p.stream, p.client = nil, nil
	}
}

// read fills the stream's buffer, interleaved left and right. It runs on the
// stream's own goroutine and takes none of the player's locks.
func (p *Player) read(buf []float32) (int, error) {
	n := len(buf) / 2
	if cap(p.l) < n {
		p.l, p.r = make([]float32, n), make([]float32, n)
	}
	l, r := p.l[:n], p.r[:n]
	p.fill(l, r)
	for i := range n {
		buf[2*i], buf[2*i+1] = clip(l[i]), clip(r[i])
	}
	return 2 * n, nil
}

// clip holds a sample to the speakers' range: four generators at full level
// on one side sum to four.
func clip(v float32) float32 {
	return max(-1, min(1, v))
}
