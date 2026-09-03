//go:build !js

// Package audiocap captures the machine's audio and hands it to a browser over
// a WebSocket, in the wire format pkg/audiosrc decodes.
//
// It exists as a package rather than as part of cmd/audiows because BOTH
// servers want it and neither should have to be the other. Running one process
// for the page and a second for the audio is not a fact about audio capture, it
// is what happens when the capture lives inside a main package: the only way to
// reuse it is to run it.
//
// The root server keeps this OPT-IN (its --audio flag) rather than always on,
// which is the part of the old arrangement that was right. The capture pulls in
// a PulseAudio client and only means anything on a machine that has one, so a
// server that nobody asked to capture anything should not open a sound device
// on start-up.
package audiocap

import (
	"fmt"
	"log"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
	"github.com/jfreymuth/pulse"
	"golang.org/x/net/websocket"
)

// Options configure a capture. The zero value is usable: 24 kHz, the default
// source, and the latency pulse picks.
type Options struct {
	// SampleRate is what the stream is recorded at. The wire format carries no
	// rate, so whatever is set here has to match what the page is told (the
	// ?wsrate= parameter, default 24000).
	SampleRate int

	// Latency is the requested buffer latency in seconds. Smaller is more
	// responsive and more likely to glitch.
	Latency float64

	// Source is "" or "default" for PulseAudio's default source, "monitor" for
	// the monitor of the default sink — which is what "capture what is playing
	// on this machine" means — or a source ID.
	Source string
}

func (o Options) withDefaults() Options {
	if o.SampleRate == 0 {
		o.SampleRate = 24000
	}
	return o
}

// Start opens the capture and calls write for every chunk. The returned func
// stops the stream and closes the client.
func (o Options) Start(write func([]float32) error) (func(), error) {
	o = o.withDefaults()
	client, err := pulse.NewClient()
	if err != nil {
		return nil, fmt.Errorf("pulse.NewClient: %w (is PulseAudio/PipeWire running?)", err)
	}

	opts := []pulse.RecordOption{pulse.RecordSampleRate(o.SampleRate)}
	if o.Latency > 0 {
		opts = append(opts, pulse.RecordLatency(o.Latency))
	}
	if opt, desc, err := o.sourceOption(client); err != nil {
		log.Printf("audiocap: source %q: %v — falling back to default source", o.Source, err)
	} else if opt != nil {
		opts = append(opts, opt)
		log.Printf("audiocap: capturing %s", desc)
	}

	stream, err := client.NewRecord(pulse.Float32Writer(func(p []float32) (int, error) {
		if len(p) == 0 {
			return 0, nil
		}
		if err := write(p); err != nil {
			return 0, err // closed connection — unwinds the record stream
		}
		return len(p), nil
	}), opts...)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("NewRecord: %w", err)
	}
	stream.Start()
	return func() {
		stream.Stop()
		client.Close()
	}, nil
}

func (o Options) sourceOption(client *pulse.Client) (pulse.RecordOption, string, error) {
	switch o.Source {
	case "", "default":
		return nil, "", nil
	case "monitor":
		sink, err := client.DefaultSink()
		if err != nil {
			return nil, "", err
		}
		return pulse.RecordMonitor(sink), "monitor of default sink " + sink.ID(), nil
	default:
		src, err := client.SourceByID(o.Source)
		if err != nil {
			return nil, "", err
		}
		return pulse.RecordSource(src), "source " + src.ID(), nil
	}
}

// Serve is the WebSocket handler: it forwards every captured chunk as a binary
// frame for the lifetime of the connection.
//
// One capture per connection, which is one per browser tab. Errors end this
// connection and are logged — never the whole server, because a page that
// cannot get audio is a page without audio and not a reason to stop serving
// everything else.
func (o Options) Serve(ws *websocket.Conn) {
	defer func() { _ = ws.Close() }() //nolint:errcheck // teardown on the way out; nothing is left to report a failure to

	stop, err := o.Start(func(p []float32) error {
		// A []byte makes this a binary frame; a string would make it a text
		// one. That single type choice is the whole encoding switch.
		return websocket.Message.Send(ws, audiosrc.Float32ToBytes(p))
	})
	if err != nil {
		log.Printf("audiocap: %v", err)
		return
	}
	defer stop()

	// Block until the client goes away. The browser never sends data, so a
	// receive error means the socket closed and we can tear down.
	for {
		var msg string
		if err := websocket.Message.Receive(ws, &msg); err != nil {
			return
		}
	}
}
