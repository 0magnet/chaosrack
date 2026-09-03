//go:build !js

package server

import (
	"log"

	"github.com/0magnet/chaosrack/pkg/audiocap"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/websocket"
)

// The audio feed, served by the same process that serves the page.
//
// The capture used to live only in cmd/audiows, so hearing anything meant
// running two servers and telling the page where the second one was
// (?wsurl=ws://…). That is a workaround for where the code lived, not a
// property of audio capture: one listener can serve the page, the host agent
// and the samples.
//
// OPT-IN, because the reason for keeping it out of the root server was partly
// right. The capture opens a PulseAudio client, and a server nobody asked to
// record anything should not touch a sound device on start-up. --audio is the
// asking; without it this registers nothing at all and the route 404s exactly
// as it did before.
var (
	audioOn     bool
	audioRate   int
	audioSource string
	audioLat    float64
)

func init() {
	runCmd.Flags().BoolVar(&audioOn, "audio", false, "capture this machine's audio and serve it at /ws (needs PulseAudio/PipeWire)")
	runCmd.Flags().IntVar(&audioRate, "audio-rate", 24000, "sample rate for --audio; must match the page's ?wsrate=")
	runCmd.Flags().StringVar(&audioSource, "audio-source", "monitor", "what --audio records: monitor (what is playing), default (the input), or a source ID")
	runCmd.Flags().Float64Var(&audioLat, "audio-latency", 0.05, "requested capture latency in seconds for --audio")
}

// mountAudio serves the capture at /ws when --audio was passed.
//
// "monitor" is the default source because it is what people mean: capture what
// this machine is PLAYING, not what its microphone hears. The mic is already
// reachable without any server at all — it is what the page uses when no
// ?audio= is given — so a flag that duplicated it would be the less useful of
// the two defaults.
func mountAudio(r *gin.Engine) {
	if !audioOn {
		return
	}
	opts := audiocap.Options{SampleRate: audioRate, Latency: audioLat, Source: audioSource}
	r.GET("/ws", gin.WrapH(websocket.Handler(opts.Serve)))
	log.Printf("chaosrack: --audio on; open with ?audio=ws")
}

// audioFeed is what the page is told about audio: "ws" when this server is
// capturing, empty when it is not. Empty means the page never dials a socket
// that nothing is listening on.
func audioFeed() string {
	if audioOn {
		return "ws"
	}
	return ""
}
