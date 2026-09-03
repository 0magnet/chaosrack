//go:build js && wasm

package audiosrc

import (
	"encoding/base64"
	"errors"
	"strings"
	"syscall/js"
)

// The browser end of the optional WebTransport audio path (?audio=wt).
//
// Kept as thin as it can be, on purpose: nothing in this file can be unit
// tested — there is no WebTransport in Node, so `make test-wasm` cannot
// reach a line of it. Every decision it makes is therefore made by code
// that CAN be tested natively: SelectTransport in transport.go decides
// whether to fall back and what to say about it, Reassembler in
// datagram.go puts the datagrams back together, and BytesToFloat32 in
// wire.go decodes the samples — the same function the WebSocket path
// uses, because both transports carry identical bytes.
//
// What is left here is plumbing: fetch the certificate hash, open the
// session, pump the datagram reader, and hand the pieces to the code
// above. See datagram.go for why datagrams rather than a stream.

// WTOptions configures a WebTransport audio Source.
type WTOptions struct {
	// InfoURL is where the server publishes the WebTransport endpoint
	// and its certificate fingerprint. Empty means same origin,
	// /wt-info. The fingerprint cannot be compiled in: the certificate
	// is generated per run because the browser API that pins one by hash
	// refuses anything valid for more than 14 days.
	InfoURL string

	// URL and CertHash override what /wt-info reports. Both set means
	// /wt-info is never fetched, which is the only way to reach a server
	// that publishes the pair some other way.
	URL      string
	CertHash string

	// SampleRate is the rate the server records at, agreed out of band
	// exactly as for the WebSocket — the wire format carries no rate.
	// Default 24000.
	SampleRate int

	// RingSize is the number of samples retained. Default 16384.
	RingSize int

	// WS configures the WebSocket source used when WebTransport cannot
	// be had. The fallback is not optional: Safari has no WebTransport
	// at all, and a corporate network that blocks UDP has no QUIC.
	WS WSOptions
}

// NewWebTransport returns a Source that receives audio over WebTransport
// datagrams, falling back to a WebSocket Source whenever it cannot.
//
// The fallback happens automatically at any of four points — no
// WebTransport in this browser, no /wt-info from the server, a rejected
// handshake, or a session that dies after working — and the reason is
// reported through Notice(), which the status overlay shows. A silent
// fallback would be indistinguishable from WebTransport working, which is
// the worst possible outcome for a feature whose entire point is what
// happens on a bad link.
func NewWebTransport(opts WTOptions) Source {
	if opts.SampleRate == 0 {
		opts.SampleRate = 24000
	}
	if opts.RingSize == 0 {
		opts.RingSize = 16384
	}
	if opts.InfoURL == "" {
		opts.InfoURL = "/wt-info"
	}
	if opts.WS.SampleRate == 0 {
		opts.WS.SampleRate = opts.SampleRate
	}
	if opts.WS.RingSize == 0 {
		opts.WS.RingSize = opts.RingSize
	}

	w := &wtSource{opts: opts, ring: newRing(opts.RingSize)}
	supported := !js.Global().Get("WebTransport").IsUndefined() && !js.Global().Get("WebTransport").IsNull()
	if kind, reason := SelectTransport("wt", WTProbe{Supported: supported}); kind != TransportWebTransport {
		w.fallBack(reason)
		return w
	}
	if opts.URL != "" && opts.CertHash != "" {
		w.dial(opts.URL, opts.CertHash)
		return w
	}
	w.fetchInfo()
	return w
}

type wtSource struct {
	opts WTOptions
	ring *ring
	ra   Reassembler

	wt     js.Value
	reader js.Value
	buf    []byte // scratch for one datagram; Reassembler.Push copies

	// fallback is the WebSocket Source that takes over once we give up.
	// Non-nil means every method below delegates and this source is
	// nothing but a wrapper carrying a notice.
	fallback Source
	notice   string

	ready  bool
	closed bool
}

// fetchInfo asks the page's own origin where the WebTransport endpoint is
// and what certificate to trust. A failure here is the ordinary case of
// a server started without -wt, so it falls back quietly rather than
// erroring.
func (w *wtSource) fetchInfo() {
	onErr := js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		w.fallBackProbe(WTProbe{Supported: true, InfoErr: jsError(args, "cannot reach "+w.opts.InfoURL)})
		return nil
	})
	onJSON := js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		if len(args) == 0 {
			w.fallBackProbe(WTProbe{Supported: true, InfoErr: errors.New("empty " + w.opts.InfoURL)})
			return nil
		}
		info := args[0]
		url, hash := info.Get("url"), info.Get("certHash")
		if !url.Truthy() || !hash.Truthy() {
			w.fallBackProbe(WTProbe{Supported: true, InfoErr: errors.New(w.opts.InfoURL + " has no url/certHash")})
			return nil
		}
		w.dial(url.String(), hash.String())
		return nil
	})
	onResp := js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		if len(args) == 0 || !args[0].Get("ok").Bool() {
			w.fallBackProbe(WTProbe{Supported: true, InfoErr: errors.New(w.opts.InfoURL + " is not being served")})
			return nil
		}
		args[0].Call("json").Call("then", onJSON).Call("catch", onErr)
		return nil
	})
	js.Global().Call("fetch", w.opts.InfoURL).Call("then", onResp).Call("catch", onErr)
}

// dial opens the session, pinning the certificate by fingerprint.
//
// serverCertificateHashes is what makes a generated certificate usable
// from a browser at all: no CA has signed it, and without this the only
// alternatives are installing it in the machine's trust store or running
// the browser with --ignore-certificate-errors. Its cost is the 14-day
// validity cap, which is why the fingerprint is fetched at runtime rather
// than built in.
func (w *wtSource) dial(url, certHash string) {
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(certHash, "="))
	if err != nil {
		w.fallBackProbe(WTProbe{Supported: true, DialErr: errors.New("unreadable certificate hash")})
		return
	}
	hashBytes := js.Global().Get("Uint8Array").New(len(raw))
	js.CopyBytesToJS(hashBytes, raw)

	entry := js.Global().Get("Object").New()
	entry.Set("algorithm", "sha-256")
	entry.Set("value", hashBytes)
	hashes := js.Global().Get("Array").New()
	hashes.Call("push", entry)
	init := js.Global().Get("Object").New()
	init.Set("serverCertificateHashes", hashes)

	// The constructor throws synchronously on a malformed URL, which in
	// Go/wasm arrives as a panic rather than an error.
	//
	// The recover below catches that on the standard runtime and does NOTHING
	// on the TinyGo one, where the same panic still ends the program with
	// "unreachable" — so the page shim is what actually makes both builds safe,
	// and the recover is the fallback for a host serving this without it.
	defer func() {
		if r := recover(); r != nil {
			w.fallBackProbe(WTProbe{Supported: true, DialErr: errors.New("cannot open " + url)})
		}
	}()
	wt := openWebTransport(url, init)
	if !wt.Truthy() {
		w.fallBackProbe(WTProbe{Supported: true, DialErr: errors.New("cannot open " + url)})
		return
	}
	w.wt = wt

	onDialErr := js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		w.fallBackProbe(WTProbe{Supported: true, DialErr: jsError(args, "handshake to "+url+" failed")})
		return nil
	})
	onReady := js.FuncOf(func(js.Value, []js.Value) interface{} {
		w.startReading()
		return nil
	})
	wt.Get("ready").Call("then", onReady).Call("catch", onDialErr)
	// A session that dies after working falls back too. Reconnecting the
	// WebTransport instead would mean re-fetching the fingerprint and
	// re-dialing on a link that has just proved unreliable, while the
	// WebSocket source already reconnects itself every 2 s — so the
	// simpler path is also the one that recovers.
	wt.Get("closed").Call("then", js.FuncOf(func(js.Value, []js.Value) interface{} {
		w.fallBackProbe(WTProbe{Supported: true, DialErr: errors.New("session closed")})
		return nil
	})).Call("catch", onDialErr)
}

// startReading pumps the datagram reader. Each datagram goes to the
// Reassembler, which returns a complete audio chunk or nothing.
func (w *wtSource) startReading() {
	if w.closed || w.fallback != nil {
		return
	}
	dgrams := w.wt.Get("datagrams")
	if !dgrams.Truthy() || !dgrams.Get("readable").Truthy() {
		w.fallBackProbe(WTProbe{Supported: true, DialErr: errors.New("session has no datagram support")})
		return
	}
	w.reader = dgrams.Get("readable").Call("getReader")

	onReadErr := js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		w.fallBackProbe(WTProbe{Supported: true, DialErr: jsError(args, "datagram stream ended")})
		return nil
	})
	// Declared before it is defined because it re-arms itself: read()
	// resolves once per datagram, so the handler has to schedule the
	// next read from inside itself.
	var onChunk js.Func
	onChunk = js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		if w.closed || w.fallback != nil {
			return nil
		}
		if len(args) == 0 || args[0].Get("done").Bool() {
			w.fallBackProbe(WTProbe{Supported: true, DialErr: errors.New("datagram stream ended")})
			return nil
		}
		w.consume(args[0].Get("value"))
		// Re-arm before returning: read() resolves once per datagram.
		w.reader.Call("read").Call("then", onChunk).Call("catch", onReadErr)
		return nil
	})
	w.reader.Call("read").Call("then", onChunk).Call("catch", onReadErr)
}

// consume copies one datagram into Go and feeds it to the reassembler.
func (w *wtSource) consume(val js.Value) {
	if !val.Truthy() {
		return
	}
	n := val.Get("length").Int()
	if n <= 0 {
		return
	}
	if cap(w.buf) < n {
		w.buf = make([]byte, n)
	}
	b := w.buf[:n]
	js.CopyBytesToGo(b, val)
	payload := w.ra.Push(b)
	if payload == nil {
		return
	}
	if samples := BytesToFloat32(payload); len(samples) > 0 {
		w.ring.write(samples)
		w.ready = true
	}
}

// fallBackProbe runs the probe through the same decision function the
// constructor used, so there is exactly one place that decides what a
// failure means and how it is worded.
func (w *wtSource) fallBackProbe(p WTProbe) {
	_, reason := SelectTransport("wt", p)
	if reason == "" {
		// A probe carrying no failure at all; there is nothing to
		// report but we were called, so something went wrong.
		reason = "WebTransport stopped for an unstated reason — using WebSocket"
	}
	w.fallBack(reason)
}

// fallBack hands the job to a WebSocket Source. Idempotent: several of
// the promise handlers above can fire for the same failure (a rejected
// handshake also closes the session), and only the first one counts.
func (w *wtSource) fallBack(reason string) {
	if w.closed || w.fallback != nil {
		return
	}
	w.notice = reason
	w.ready = false
	w.ra.Reset()
	w.fallback = NewWebSocket(w.opts.WS)
	if !w.wt.IsUndefined() && w.wt.Truthy() {
		w.wt.Call("close")
		w.wt = js.Undefined()
	}
}

// Notice reports a fallback so the status overlay can say so. Empty while
// WebTransport is doing its job.
func (w *wtSource) Notice() string { return w.notice }

func (w *wtSource) TimeDomain(dst []float32) []float32 {
	if w.fallback != nil {
		return w.fallback.TimeDomain(dst)
	}
	if !w.ready || len(dst) == 0 {
		for i := range dst {
			dst[i] = 0
		}
		return dst
	}
	w.ring.latest(dst)
	return dst
}

func (w *wtSource) TimeDomainStereo(l, r []float32) {
	if w.fallback != nil {
		w.fallback.TimeDomainStereo(l, r)
		return
	}
	if len(l) != len(r) {
		panic("audiosrc: TimeDomainStereo requires len(l) == len(r)")
	}
	w.TimeDomain(l)
	copy(r, l) // mono stream: right mirrors left
}

func (w *wtSource) Drain(dst []float32) int {
	if w.fallback != nil {
		return w.fallback.Drain(dst)
	}
	if !w.ready {
		return 0
	}
	return w.ring.drain(dst)
}

func (w *wtSource) SampleRate() int {
	if w.fallback != nil {
		return w.fallback.SampleRate()
	}
	return w.opts.SampleRate
}

func (w *wtSource) Channels() int {
	if w.fallback != nil {
		return w.fallback.Channels()
	}
	if !w.ready {
		return 0
	}
	return 1
}

func (w *wtSource) Ready() bool {
	if w.fallback != nil {
		return w.fallback.Ready()
	}
	return w.ready && !w.closed
}

// Err reports only the fallback's errors. Nothing this source can do
// wrong is terminal — every failure of the WebTransport path ends in a
// WebSocket, so the error that matters is always that one's. What went
// wrong with WebTransport is a Notice, not an Err, because the audio is
// still arriving.
func (w *wtSource) Err() error {
	if w.fallback != nil {
		return w.fallback.Err()
	}
	return nil
}

func (w *wtSource) Close() {
	if w.closed {
		return
	}
	w.closed = true
	if w.fallback != nil {
		w.fallback.Close()
		return
	}
	if !w.wt.IsUndefined() && w.wt.Truthy() {
		w.wt.Call("close")
	}
}

// jsError turns a rejected promise's argument into a Go error, falling
// back to a description of what was being attempted. Browsers word these
// rejections differently and some of them carry nothing useful, so the
// caller's own phrasing is what the user usually sees.
func jsError(args []js.Value, fallback string) error {
	if len(args) > 0 && args[0].Truthy() {
		if msg := args[0].Get("message"); msg.Truthy() {
			return errors.New(msg.String())
		}
		if args[0].Type() == js.TypeString {
			return errors.New(args[0].String())
		}
	}
	return errors.New(fallback)
}

// openWebTransport is the WebTransport half of openWebSocket, for the same
// reason: the constructor throws rather than returning an error, and a throw
// reaching Go is a panic that ends the whole program in wasm.
func openWebTransport(url string, init js.Value) js.Value {
	if sh := js.Global().Get("__crWT"); sh.Truthy() {
		return sh.Call("open", url, init)
	}
	ctor := js.Global().Get("WebTransport")
	if ctor.IsUndefined() {
		return js.Value{}
	}
	return ctor.New(url, init) // the caller's recover() is the guard here
}
