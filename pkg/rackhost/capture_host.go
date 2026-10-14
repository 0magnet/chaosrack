//go:build !js

package rackhost

import "github.com/0magnet/chaosrack/pkg/audiocap"

// hostCapture captures the machine's audio through PulseAudio, from source
// ("monitor" for what is playing, "default" for the input, or a source's ID),
// as the page's server does.
func hostCapture(source string, write func([]float32) error) (func(), error) {
	return audiocap.Options{SampleRate: sampleRate, Channels: 2, Latency: 0.02, Source: source}.Start(write)
}
