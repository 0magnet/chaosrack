//go:build !js

package subcmds

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/0magnet/chaosrack/pkg/audiocap"
)

var renderAudioSrc string

// What --audio does here, and how an audio model is given a signal.
const (
	audioUsage = "for an audio model, record this many seconds of what is playing (or, with --signal, generate that much; default 2)"
	audioHint  = "--audio SECONDS records what is playing, --signal NAME uses a test signal"
)

// recordFlags adds what recording takes: which of this machine's sources.
func recordFlags(renderCmd *cobra.Command) {
	renderCmd.Flags().StringVar(&renderAudioSrc, "audio-source", "monitor", "what --audio records: monitor (what is playing), default (the input), or a source ID")
}

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
