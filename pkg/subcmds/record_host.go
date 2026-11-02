//go:build !js

package subcmds

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/0magnet/chaosrack/pkg/audiocap"
)

var renderAudioSrc string

// What --audio does here, and how an audio model is given a signal.
const (
	audioUsage = "for an audio model, record this many seconds of what is playing (or, with --signal, generate that much; default 2); a video draws it as it plays, and --audio 0 until Ctrl+C"
	audioHint  = "--audio-file FILE draws a recording, --audio SECONDS records what is playing, --signal NAME uses a test signal"
)

// recordFlags adds what recording takes: which of this machine's sources.
func recordFlags(renderCmd *cobra.Command) {
	renderCmd.Flags().StringVar(&renderAudioSrc, "audio-source", "monitor", "what --audio records: monitor (what is playing), default (the input), or a source ID")
}

// recLatency is the buffering a recording asks the sound server for.
const recLatency = 0.02

// record captures secs seconds of stereo audio through PulseAudio/PipeWire.
func record(secs float64) (l, r []float32, err error) {
	want := int(secs * renderSampleRate)
	l, r = make([]float32, 0, want), make([]float32, 0, want)
	done := make(chan struct{})
	full := false
	errf("chaosrack: recording %gs from %s\n", secs, renderAudioSrc)
	stop, err := audiocap.Options{
		SampleRate: renderSampleRate,
		Channels:   2,
		Source:     renderAudioSrc,
		// As the rack asks (capture_host.go): left to its default, the server
		// buffers seconds and hands them over late and in lumps.
		Latency: recLatency,
	}.Start(func(p []float32) error {
		if full {
			return nil
		}
		for i := 0; i+1 < len(p) && len(l) < want; i += 2 {
			l = append(l, p[i])
			r = append(r, p[i+1])
		}
		if len(l) >= want {
			full = true
			close(done)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	select {
	case <-done:
	case <-time.After(time.Duration(secs*float64(time.Second)) + 5*time.Second):
		stop()
		return nil, nil, fmt.Errorf("recording stalled after %d of %d samples", len(l), want)
	}
	stop()
	return l, r, nil
}

// liveCapture is this machine's audio as it comes in, for a video drawn
// while it records: the frames take it a frame's worth at a time, and what
// they have not taken yet waits here, so a frame that draws slower than real
// time falls behind rather than losing any of it.
type liveCapture struct {
	mu     sync.Mutex
	cond   *sync.Cond
	l, r   []float32
	got    int  // samples taken in, all told
	want   int  // samples to take in, 0 for until stopped
	closed bool // no more is coming
	stop   func()
	unsig  func() // gives Ctrl+C back
	once   sync.Once
}

// startLive starts recording: secs seconds of it, or until stopped for 0.
func startLive(secs float64) (*liveCapture, error) {
	c := &liveCapture{want: int(secs * renderSampleRate)}
	c.cond = sync.NewCond(&c.mu)
	stop, err := audiocap.Options{
		SampleRate: renderSampleRate,
		Channels:   2,
		Source:     renderAudioSrc,
		// As the rack asks (capture_host.go): left to its default, the server
		// buffers seconds and hands them over late and in lumps.
		Latency: recLatency,
	}.Start(func(p []float32) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		for i := 0; i+1 < len(p) && !c.closed; i += 2 {
			c.l = append(c.l, p[i])
			c.r = append(c.r, p[i+1])
			c.got++
			if c.want > 0 && c.got >= c.want {
				c.closed = true
			}
		}
		c.cond.Broadcast()
		return nil
	})
	if err != nil {
		return nil, err
	}
	c.stop = stop
	return c, nil
}

// take is the next n samples, waiting for them; fewer, then none, once the
// recording has ended.
func (c *liveCapture) take(n int) (l, r []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.l) < n && !c.closed {
		c.cond.Wait()
	}
	k := min(n, len(c.l))
	l, r = append([]float32(nil), c.l[:k]...), append([]float32(nil), c.r[:k]...)
	c.l, c.r = c.l[k:], c.r[k:]
	return l, r
}

// behind is how far, in seconds, the frames are behind the recording.
func (c *liveCapture) behind() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return float64(len(c.l)) / renderSampleRate
}

// end stops recording; what is already in is still taken.
func (c *liveCapture) end() {
	c.once.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.cond.Broadcast()
		c.mu.Unlock()
		c.stop()
		if c.unsig != nil {
			c.unsig()
		}
	})
}

// startRecording starts recording for a video of frames frames, or, for 0,
// until Ctrl+C, which ends the recording rather than the program: the frames
// still to draw are drawn and the video is finished with its sound. A second
// Ctrl+C stops at once.
func startRecording(frames int) (*liveCapture, error) {
	secs := 0.0
	if frames > 0 {
		secs = float64(frames) / float64(renderFPS)
	}
	c, err := startLive(secs)
	if err != nil {
		return nil, err
	}
	ctx, unsig := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	c.unsig = unsig
	go func() {
		<-ctx.Done()
		c.end()
	}()
	if secs > 0 {
		errf("chaosrack: drawing %gs of %s as it plays (Ctrl+C ends it sooner)\n", secs, renderAudioSrc)
	} else {
		errf("chaosrack: drawing %s as it plays; Ctrl+C ends the video\n", renderAudioSrc)
	}
	return c, nil
}

// progress says, once a second of video, how much has been drawn and how far
// the drawing is behind the recording.
func (c *liveCapture) progress(frame int) {
	if frame%renderFPS != 0 {
		return
	}
	errf("\rchaosrack: %ds drawn, %.1fs behind   ", frame/renderFPS, c.behind())
}
