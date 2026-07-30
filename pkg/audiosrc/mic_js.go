//go:build js && wasm

package audiosrc

import (
	"errors"
	"syscall/js"
	"unsafe"
)

// MicOptions configures a microphone Source.
type MicOptions struct {
	// Stereo requests a two-channel capture. Falls back to mono if the
	// device or browser only exposes one channel; check Channels() after
	// Ready to know what you actually got.
	Stereo bool

	// BufferSize is the ScriptProcessorNode buffer size in sample frames
	// (a power of two, 256..16384). Larger = fewer callbacks but more
	// latency. Default 4096.
	BufferSize int

	// RingSize is the number of samples retained per channel. Must exceed
	// the largest snapshot window and per-frame drain backlog. Default
	// 16384.
	RingSize int
}

// NewMic requests microphone access via getUserMedia and captures the
// continuous sample stream with a ScriptProcessorNode whose onaudioprocess
// callback runs in Go (no separate JS worklet module — keeps everything in
// wasm). Samples flow into per-channel ring buffers that serve both
// TimeDomain (latest window, e.g. xy scope) and Drain (continuous stream
// for the overlapping STFT spectrogram).
//
// getUserMedia requires a secure context (https or localhost). On file://
// or plain http:// LAN pages, Err() is set once the browser refuses;
// callers should render a fallback.
func NewMic(opts MicOptions) Source {
	if opts.BufferSize == 0 {
		opts.BufferSize = 4096
	}
	if opts.RingSize == 0 {
		opts.RingSize = 16384
	}
	m := &micSource{opts: opts, ringL: newRing(opts.RingSize)}
	nav := js.Global().Get("navigator")
	if nav.IsUndefined() || nav.Get("mediaDevices").IsUndefined() {
		m.err = errors.New("navigator.mediaDevices unavailable (need https or localhost)")
		return m
	}
	constraints := map[string]interface{}{
		"audio": map[string]interface{}{
			"channelCount":     boolTernary(opts.Stereo, 2, 1),
			"echoCancellation": false,
			"noiseSuppression": false,
			"autoGainControl":  false,
		},
		"video": false,
	}
	promise := nav.Get("mediaDevices").Call("getUserMedia", constraints)
	promise.Call("then", js.FuncOf(m.onStream)).Call("catch", js.FuncOf(m.onError))
	return m
}

func boolTernary(b bool, t, f int) int {
	if b {
		return t
	}
	return f
}

type micSource struct {
	opts MicOptions
	err  error

	stream    js.Value
	audioCtx  js.Value
	src       js.Value // MediaStreamAudioSourceNode
	processor js.Value // ScriptProcessorNode
	onProcess js.Func

	ringL *ring
	ringR *ring // nil when mono

	// Scratch reused every callback to move a channel's Float32Array into
	// Go without per-sample JS round-trips.
	byteScratch []byte

	sampleRate int
	channels   int
	ready      bool
	closed     bool
}

func (m *micSource) onStream(_ js.Value, args []js.Value) interface{} {
	if m.closed {
		if len(args) > 0 {
			stopTracks(args[0])
		}
		return nil
	}
	m.stream = args[0]

	ac := js.Global().Get("AudioContext")
	if ac.IsUndefined() {
		ac = js.Global().Get("webkitAudioContext")
	}
	if ac.IsUndefined() {
		m.err = errors.New("AudioContext unavailable")
		stopTracks(m.stream)
		return nil
	}
	m.audioCtx = ac.New()
	m.sampleRate = m.audioCtx.Get("sampleRate").Int()
	m.src = m.audioCtx.Call("createMediaStreamSource", m.stream)

	// Detect the real channel count from the track.
	tracks := m.stream.Call("getAudioTracks")
	if tracks.Length() > 0 {
		if v := tracks.Index(0).Call("getSettings").Get("channelCount"); !v.IsUndefined() {
			m.channels = v.Int()
		}
	}
	if m.channels == 0 {
		m.channels = 1
	}
	if m.channels >= 2 {
		m.ringR = newRing(m.opts.RingSize)
	}

	m.byteScratch = make([]byte, m.opts.BufferSize*4)

	// ScriptProcessorNode: deprecated but universally supported and — key
	// for the no-JS-worklet constraint — its callback runs here in Go.
	m.processor = m.audioCtx.Call("createScriptProcessor", m.opts.BufferSize, m.channels, 1)
	m.onProcess = js.FuncOf(m.handleProcess)
	m.processor.Set("onaudioprocess", m.onProcess)
	m.src.Call("connect", m.processor)
	// Must reach the destination for onaudioprocess to fire. We never
	// write the output buffer, so it stays silent (no mic→speaker echo).
	m.processor.Call("connect", m.audioCtx.Get("destination"))

	m.ready = true
	return nil
}

// handleProcess pulls one buffer of input samples per channel into the
// rings. Runs on the JS main thread, serialized with the render loop.
func (m *micSource) handleProcess(_ js.Value, args []js.Value) interface{} {
	if m.closed || len(args) == 0 {
		return nil
	}
	inBuf := args[0].Get("inputBuffer")
	m.pullChannel(inBuf, 0, m.ringL)
	if m.ringR != nil {
		m.pullChannel(inBuf, 1, m.ringR)
	}
	return nil
}

func (m *micSource) pullChannel(inBuf js.Value, ch int, r *ring) {
	data := inBuf.Call("getChannelData", ch) // Float32Array
	n := data.Get("length").Int()
	byteLen := n * 4
	if byteLen > len(m.byteScratch) {
		byteLen = len(m.byteScratch)
		n = byteLen / 4
	}
	u8 := js.Global().Get("Uint8Array").New(data.Get("buffer"))
	js.CopyBytesToGo(m.byteScratch[:byteLen], u8)
	// Reinterpret the little-endian bytes as float32 (host-native order on
	// wasm is little-endian, matching Float32Array's layout).
	samples := unsafe.Slice((*float32)(unsafe.Pointer(&m.byteScratch[0])), n)
	r.write(samples)
}

func (m *micSource) onError(_ js.Value, args []js.Value) interface{} {
	msg := "mic permission denied"
	if len(args) > 0 {
		if name := args[0].Get("name"); !name.IsUndefined() {
			msg = name.String() + ": " + args[0].Get("message").String()
		}
	}
	m.err = errors.New(msg)
	return nil
}

func (m *micSource) TimeDomain(dst []float32) []float32 {
	if !m.ready || len(dst) == 0 {
		for i := range dst {
			dst[i] = 0
		}
		return dst
	}
	m.ringL.latest(dst)
	return dst
}

func (m *micSource) TimeDomainStereo(l, r []float32) {
	if len(l) != len(r) {
		panic("audiosrc: TimeDomainStereo requires len(l) == len(r)")
	}
	if !m.ready {
		for i := range l {
			l[i] = 0
			r[i] = 0
		}
		return
	}
	m.ringL.latest(l)
	if m.ringR != nil {
		m.ringR.latest(r)
	} else {
		copy(r, l)
	}
}

func (m *micSource) Drain(dst []float32) int {
	if !m.ready {
		return 0
	}
	return m.ringL.drain(dst)
}

func (m *micSource) SampleRate() int { return m.sampleRate }
func (m *micSource) Channels() int   { return m.channels }
func (m *micSource) Ready() bool     { return m.ready && !m.closed }
func (m *micSource) Err() error      { return m.err }

func (m *micSource) Close() {
	if m.closed {
		return
	}
	m.closed = true
	if !m.processor.IsUndefined() {
		m.processor.Call("disconnect")
		m.processor.Set("onaudioprocess", js.Null())
	}
	if !m.src.IsUndefined() {
		m.src.Call("disconnect")
	}
	if !m.stream.IsUndefined() {
		stopTracks(m.stream)
	}
	if !m.audioCtx.IsUndefined() {
		m.audioCtx.Call("close")
	}
	if m.onProcess.Truthy() {
		m.onProcess.Release()
	}
}

// stopTracks stops every audio track on a MediaStream so the browser mic
// indicator turns off promptly.
func stopTracks(stream js.Value) {
	if stream.IsUndefined() || stream.IsNull() {
		return
	}
	tracks := stream.Call("getTracks")
	n := tracks.Length()
	for i := 0; i < n; i++ {
		tracks.Index(i).Call("stop")
	}
}
