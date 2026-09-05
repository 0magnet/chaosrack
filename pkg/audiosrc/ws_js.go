//go:build js && wasm

package audiosrc

import (
	"encoding/base64"
	"errors"
	"syscall/js"
)

// WSOptions configures a WebSocket audio Source.
type WSOptions struct {
	// URL is the ws:// or wss:// endpoint streaming the audio. Empty
	// means "same origin, path /ws" — resolved from window.location at
	// connect time (ws for http, wss for https). This mirrors
	// audioprism-go's client, which always talks to /ws on its own host.
	URL string

	// SampleRate is the rate (Hz) the server records at. The server side
	// forces this (audioprism-go uses 24000), and the wire format carries
	// no rate, so the consumer has to be told. Default 24000.
	SampleRate int

	// RingSize is the number of samples retained. Must exceed the largest
	// window a consumer snapshots (FFT size) and the largest per-frame
	// drain backlog. Default DefaultRingSize.
	RingSize int
}

// NewWebSocket returns a Source that receives audio from a WebSocket
// server streaming little-endian float32 samples in binary frames — the
// wire format of cmd/audiows and of audioprism-go's /ws handler. Servers
// that have not been upgraded yet, which send the same bytes base64'd in
// a text frame, are still read (see decodeWSMessage). The connection is
// opened immediately and re-established automatically 2s after any drop.
// Samples land in a ring buffer that TimeDomain snapshots and Drain
// consumes, so both the pull-latest (xy scope) and continuous-stream
// (spectrogram STFT) callers are served.
//
// The stream is mono (the server records a single channel); Channels()
// reports 1 and TimeDomainStereo copies the one channel to both outputs.
func NewWebSocket(opts WSOptions) Source {
	if opts.SampleRate == 0 {
		opts.SampleRate = 24000
	}
	if opts.RingSize == 0 {
		opts.RingSize = DefaultRingSize
	}
	w := &wsSource{opts: opts, ring: newRing(opts.RingSize)}
	if js.Global().Get("WebSocket").IsUndefined() {
		w.err = errors.New("WebSocket not supported in this browser")
		return w
	}
	w.url = opts.URL
	if w.url == "" {
		w.url = sameOriginWSURL()
	}
	w.onMsg = js.FuncOf(w.handleMessage)
	w.onOpen = js.FuncOf(func(js.Value, []js.Value) interface{} {
		w.reconnecting = false
		w.err = nil
		return nil
	})
	w.onDown = js.FuncOf(func(js.Value, []js.Value) interface{} {
		w.scheduleReconnect(2000)
		return nil
	})
	w.onErr = js.FuncOf(func(js.Value, []js.Value) interface{} {
		if w.err == nil {
			w.err = errors.New("websocket error connecting to " + w.url)
		}
		return nil
	})
	w.connect()
	return w
}

// sameOriginWSURL builds ws(s)://<host>/ws from window.location, choosing
// wss when the page itself was served over https.
func sameOriginWSURL() string {
	loc := js.Global().Get("location")
	proto := "ws"
	if loc.Get("protocol").String() == "https:" {
		proto = "wss"
	}
	return proto + "://" + loc.Get("host").String() + "/ws"
}

type wsSource struct {
	opts WSOptions
	url  string
	ws   js.Value
	ring *ring

	// The socket's four callbacks, made ONCE and reused for every socket this
	// source opens. connect() used to build three of them per attempt, and
	// connect() runs again two seconds after every close — so a server that is
	// down had this allocating three callbacks every two seconds, for as long
	// as the page stayed open, with nothing releasing any of them. Each one
	// pins a Go closure and an entry in the syscall/js callback table.
	onMsg  js.Func
	onOpen js.Func
	onDown js.Func
	onErr  js.Func

	reconnecting bool
	ready        bool
	closed       bool
	err          error
}

func (w *wsSource) connect() {
	if w.closed {
		return
	}
	ws := openWebSocket(w.url)
	if !ws.Truthy() {
		// Not "unsupported": the constructor was there and refused this
		// address. Say which, because the address came from ?wsurl= and a
		// typo in it is the likeliest way to get here.
		w.err = errors.New("could not open a websocket to " + w.url)
		return
	}
	// Binary frames arrive as a Blob by default, and a Blob is only
	// readable asynchronously (arrayBuffer() returns a promise), which
	// would put a task-queue hop between every audio chunk and the ring
	// buffer. An ArrayBuffer is readable in the message handler itself.
	ws.Set("binaryType", "arraybuffer")
	ws.Call("addEventListener", "open", w.onOpen)
	ws.Call("addEventListener", "message", w.onMsg)
	ws.Call("addEventListener", "close", w.onDown)
	ws.Call("addEventListener", "error", w.onErr)
	w.ws = ws
}

func (w *wsSource) scheduleReconnect(delayMs int) {
	if w.reconnecting || w.closed {
		return
	}
	w.reconnecting = true
	js.Global().Call("setTimeout", js.FuncOf(func(js.Value, []js.Value) interface{} {
		w.reconnecting = false
		w.connect()
		return nil
	}), delayMs)
}

// handleMessage writes one decoded chunk into the ring.
func (w *wsSource) handleMessage(_ js.Value, p []js.Value) interface{} {
	if len(p) == 0 {
		return nil
	}
	samples := decodeWSMessage(p[0].Get("data"))
	if len(samples) == 0 {
		return nil
	}
	w.ring.write(samples)
	w.ready = true
	return nil
}

// decodeWSMessage turns one WebSocket message into samples, accepting
// either encoding of the same bytes: an ArrayBuffer from a binary frame,
// or a string from the older base64 text frame.
//
// Branching on the JS type is the whole compatibility story — no version
// handshake, no query parameter, no flag day. The two encodings are
// distinguishable by construction (a text frame can never arrive as an
// ArrayBuffer), so a new page works against an old server and an old page
// against a new one, and the two repos that speak this format can be
// upgraded in either order.
func decodeWSMessage(data js.Value) []float32 {
	if data.IsUndefined() || data.IsNull() {
		return nil
	}
	if data.Type() == js.TypeString {
		b, err := base64.StdEncoding.DecodeString(data.String())
		if err != nil {
			return nil
		}
		return BytesToFloat32(b)
	}
	if !data.InstanceOf(js.Global().Get("ArrayBuffer")) {
		return nil // a Blob, if some browser ignored binaryType; not readable here
	}
	u8 := js.Global().Get("Uint8Array").New(data)
	b := make([]byte, u8.Length())
	js.CopyBytesToGo(b, u8)
	return BytesToFloat32(b)
}

func (w *wsSource) TimeDomain(dst []float32) []float32 {
	if !w.ready || len(dst) == 0 {
		for i := range dst {
			dst[i] = 0
		}
		return dst
	}
	w.ring.latest(dst)
	return dst
}

func (w *wsSource) TimeDomainStereo(l, r []float32) {
	if len(l) != len(r) {
		panic("audiosrc: TimeDomainStereo requires len(l) == len(r)")
	}
	w.TimeDomain(l)
	copy(r, l) // mono stream: right mirrors left
}

func (w *wsSource) Drain(dst []float32) int {
	if !w.ready {
		return 0
	}
	return w.ring.drain(dst)
}

func (w *wsSource) SampleRate() int { return w.opts.SampleRate }
func (w *wsSource) Channels() int {
	if !w.ready {
		return 0
	}
	return 1
}
func (w *wsSource) Ready() bool { return w.ready && !w.closed }
func (w *wsSource) Err() error  { return w.err }

func (w *wsSource) Close() {
	if w.closed {
		return
	}
	w.closed = true
	if !w.ws.IsUndefined() {
		w.ws.Call("close")
	}
	// Released here and nowhere else: a callback that is freed while the socket
	// can still deliver an event would be called after release, and calling a
	// released js.Func panics — which in wasm takes the page, not the audio.
	// closed is set above, so no reconnect can follow this.
	for _, fn := range []js.Func{w.onMsg, w.onOpen, w.onDown, w.onErr} {
		if !fn.Value.IsUndefined() {
			fn.Release()
		}
	}
}

// openWebSocket constructs a WebSocket without letting it take the page down.
//
// `new WebSocket(url)` THROWS rather than returning an error, and a throw
// arriving back in Go is a panic — which in wasm ends the program: the canvas
// keeps its last frame and nothing responds again. Two ordinary things make it
// throw, and neither is this code being wrong:
//
//   - a URL it cannot parse. The address comes from ?wsurl=, so this is a
//     mistyped link, and it was measured killing the app outright:
//     "?audio=ws&wsurl=ws://[bad" took the whole page with it.
//   - a ws:// address requested from an https page. That is mixed content, and
//     it is every visit to the deployed site rather than an edge case.
//
// The catch is in JavaScript because that is the only place both runtimes
// agree: a deferred recover() is enough for the standard Go build and does
// nothing in the TinyGo one. The recover below is the fallback for a host page
// that serves this package without the shim.
func openWebSocket(url string) (ws js.Value) {
	if sh := js.Global().Get("__crWS"); sh.Truthy() {
		return sh.Call("open", url)
	}
	defer func() {
		if recover() != nil {
			ws = js.Value{}
		}
	}()
	ctor := js.Global().Get("WebSocket")
	if ctor.IsUndefined() {
		return js.Value{}
	}
	return ctor.New(url)
}
