// Package audiosrc provides browser audio sources for wasm-stuff
// visualizations that want live audio input (spectrogram, xy scope, and
// eventually audio-modulated attractor parameters).
//
// Sources are pull-based: consumers call TimeDomain(dst) once per
// animation frame to snapshot the most recent samples. This suits the
// requestAnimationFrame render loop and sidesteps the goroutine/callback
// complexity of a push-based design. Actual audio scheduling stays inside
// the browser's AudioContext where it belongs.
//
// All implementations are JS/WASM-only. This file has no build tag so
// non-WASM consumers can still refer to the Source type in shared code
// (e.g. mode dispatch); constructors live in *_js.go.
package audiosrc

// Source is a live audio input feeding time-domain samples in [-1, 1].
//
// Sources may take an unbounded amount of time to become Ready (mic
// permission prompt, WebSocket handshake, etc.). Consumers should call
// Ready() before trusting TimeDomain output; before that, the destination
// slice is left zero-filled.
type Source interface {
	// TimeDomain fills dst with the most recent len(dst) samples from
	// the primary channel and returns dst. If the source is not Ready,
	// dst is zeroed.
	TimeDomain(dst []float32) []float32

	// TimeDomainStereo fills l and r with the most recent samples from
	// the left and right channels respectively. len(l) must equal
	// len(r). If the underlying source is mono, r receives a copy of l
	// (callers wanting a pseudo-stereo Lissajous can post-process with
	// a lag). Not Ready → both zeroed.
	TimeDomainStereo(l, r []float32)

	// Drain copies the samples from the primary channel that have
	// arrived since the previous Drain call into dst (oldest first) and
	// returns the number copied (≤ len(dst)). Unlike TimeDomain, which
	// re-snapshots the latest window every call, Drain hands each sample
	// to the caller exactly once — the continuous stream a proper
	// overlapping STFT needs. If more than len(dst) samples arrived
	// since the last call, only the most recent len(dst) survive (older
	// ones are dropped, so call with a generous dst each frame). Not
	// Ready → returns 0.
	Drain(dst []float32) int

	// SampleRate returns the underlying AudioContext's sampleRate in
	// Hz (typically 44100 or 48000). Returns 0 before the context is
	// established.
	SampleRate() int

	// Channels reports the number of audio channels the source is
	// actually delivering (1 or 2). Returns 0 before Ready.
	Channels() int

	// Ready reports whether the source has been fully initialized and
	// is delivering samples. Mic sources are async (user permission);
	// they start not-Ready and flip once permission is granted.
	Ready() bool

	// Err returns any terminal error that has occurred (e.g., user
	// denied mic permission, WebSocket refused). Nil while the source
	// is either still initializing or working normally.
	Err() error

	// Close releases browser resources: stops MediaStream tracks,
	// disconnects AudioNodes, closes WebSockets. Safe to call multiple
	// times or before Ready.
	Close()
}

// ring is a fixed-capacity circular sample buffer shared by the source
// implementations. It is pure Go (no syscall/js) so it lives here rather
// than in a build-tagged file. All access happens on the single JS main
// thread (wasm is single-threaded; audio callbacks and the render loop
// never overlap), so no synchronization is needed.
type ring struct {
	buf        []float32
	writeTotal int // monotonic count of samples ever written
	readTotal  int // monotonic count consumed by drain
}

func newRing(size int) *ring { return &ring{buf: make([]float32, size)} }

// write appends samples, overwriting the oldest once full.
func (r *ring) write(s []float32) {
	n := len(r.buf)
	for _, v := range s {
		r.buf[r.writeTotal%n] = v
		r.writeTotal++
	}
}

// latest fills dst with the most recent len(dst) samples, oldest first.
// Positions with no data yet (early startup) read as 0.
func (r *ring) latest(dst []float32) {
	size := len(r.buf)
	n := len(dst)
	for i := 0; i < n; i++ {
		idx := r.writeTotal - n + i
		if idx < 0 {
			dst[i] = 0
			continue
		}
		dst[i] = r.buf[idx%size]
	}
}

// drain copies samples written since the previous drain into dst (oldest
// first) and returns the count. If the backlog exceeds the ring capacity,
// the oldest unread samples are discarded before copying.
func (r *ring) drain(dst []float32) int {
	size := len(r.buf)
	if r.writeTotal-r.readTotal > size {
		r.readTotal = r.writeTotal - size
	}
	n := r.writeTotal - r.readTotal
	if n > len(dst) {
		n = len(dst)
	}
	for i := 0; i < n; i++ {
		dst[i] = r.buf[(r.readTotal+i)%size]
	}
	r.readTotal += n
	return n
}
