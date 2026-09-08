//go:build js && wasm

package audiosrc

import "strings"

// Two channels over a server feed, and what a mono reader gets from them.
//
// The capture records the source as it is: a monitor of a stereo sink has two
// channels, and folding them together at the capture throws away the only thing
// a phase display has to show. So the server interleaves L,R,L,R… when asked
// (?ch=2) and this de-interleaves into a ring per channel.
//
// Everything here is transport-agnostic on purpose. The WebSocket and the
// WebTransport feeds carry IDENTICAL bytes — that is stated at the top of
// wire.go and it is the reason there is one decoder — so they must also make
// identical sense of two channels. When only the WebSocket did, the Stereo
// Embedding drew the diagonal and reported "mono" on a stereo capture the
// moment the page preferred WebTransport, which looks exactly like a broken
// display. So the rings, the fold and the query parameter live here and both
// transports use them rather than each keeping its own idea of what ?ch=2 did.
//
// Most readers here want one signal — the spectrogram's STFT, the Takens
// embedding, the frequency counter. They keep asking for one, and get whichever
// fold MonoMode names. The default is the mix, because the question those
// displays answer is "what is playing", and half of what is playing is not it.

// MonoMode is how two channels are folded into the one that TimeDomain and
// Drain deliver.
type MonoMode int

const (
	// MonoMix is (L+R)/2 — everything that is playing, which is what a
	// spectrogram of "this machine's audio" is expected to show.
	MonoMix MonoMode = iota
	// MonoLeft and MonoRight take one channel as it is. A stereo mix hides
	// things in one channel that the sum buries: a hard-panned instrument,
	// one dead side of an interface, a channel that is out of polarity with
	// the other and cancels in the sum.
	MonoLeft
	MonoRight
)

// fold reduces one interleaved pair to the single sample a mono reader gets.
func (m MonoMode) fold(l, r float32) float32 {
	switch m {
	case MonoLeft:
		return l
	case MonoRight:
		return r
	default:
		return (l + r) * 0.5
	}
}

// String is what the UI shows for the current fold.
func (m MonoMode) String() string {
	switch m {
	case MonoLeft:
		return "left"
	case MonoRight:
		return "right"
	default:
		return "mix"
	}
}

// deinterleave splits an interleaved stereo frame into its channels, and folds
// the pair into the mono stream the single-signal readers consume.
//
// An odd tail is dropped rather than guessed at. A frame that ends mid-pair
// means the stream is not what it claims to be, and inventing the missing
// sample would put a click in every reader rather than in none.
func deinterleave(src []float32, mode MonoMode) (l, r, mono []float32) {
	n := len(src) / 2
	if n == 0 {
		return nil, nil, nil
	}
	l = make([]float32, n)
	r = make([]float32, n)
	mono = make([]float32, n)
	for i := 0; i < n; i++ {
		l[i] = src[2*i]
		r[i] = src[2*i+1]
		mono[i] = mode.fold(l[i], r[i])
	}
	return l, r, mono
}

// withChannels adds the ?ch= the server reads to an endpoint URL.
//
// A server that does not know the parameter ignores it and sends mono, which
// every reader here still decodes — the page degrades to one channel rather
// than to nothing. That is why it is a query parameter and not a handshake.
func withChannels(url string, channels int) string {
	if channels != 2 {
		return url
	}
	if strings.Contains(url, "?") {
		return url + "&ch=2"
	}
	return url + "?ch=2"
}

// stereoRings is the sample retention behind a server feed: the channels as
// received, plus the fold that every single-signal reader consumes.
//
// The fold is kept as a THIRD ring rather than derived on read. Deriving it
// would mean folding a whole window on every TimeDomain call — once per frame
// per consumer, against a snapshot that is mostly the same samples as last
// frame's — where writing it costs one add per sample received, once. It also
// keeps Drain honest: Drain hands each sample over exactly once, so there is
// nowhere to do the fold lazily that would not either repeat it or lose it.
type stereoRings struct {
	// fold is the mono stream TimeDomain and Drain deliver. On a one-channel
	// stream it IS the stream, and l and r are nil.
	fold *ring
	l, r *ring
	mono MonoMode
}

// newStereoRings sizes the rings for a stream of the given channel count.
// Anything but 2 is one channel, which is what every reader written before
// two-channel capture expects.
func newStereoRings(size, channels int) *stereoRings {
	s := &stereoRings{fold: newRing(size)}
	if channels == 2 {
		s.l, s.r = newRing(size), newRing(size)
	}
	return s
}

// write takes one decoded chunk as it arrived — interleaved when this is a
// two-channel stream — and reports whether anything landed. False means a
// chunk too short to hold a single pair, which is nothing to show and must not
// be read as the source having become ready.
func (s *stereoRings) write(samples []float32) bool {
	if len(samples) == 0 {
		return false
	}
	if s.l == nil {
		s.fold.write(samples)
		return true
	}
	l, r, mono := deinterleave(samples, s.mono)
	if len(mono) == 0 {
		return false
	}
	s.l.write(l)
	s.r.write(r)
	s.fold.write(mono)
	return true
}

// latest fills dst with the newest samples of the mono fold, oldest first.
func (s *stereoRings) latest(dst []float32) { s.fold.latest(dst) }

// latestStereo fills l and r with the newest samples of each channel. A
// one-channel stream mirrors the fold into both, which is the Source contract.
func (s *stereoRings) latestStereo(l, r []float32) {
	if s.l == nil {
		s.fold.latest(l)
		copy(r, l)
		return
	}
	s.l.latest(l)
	s.r.latest(r)
}

// drain hands over the mono fold's unread samples; see Source.Drain.
func (s *stereoRings) drain(dst []float32) int { return s.fold.drain(dst) }

// channels is what Source.Channels reports for a ready source.
func (s *stereoRings) channels() int {
	if s.l == nil {
		return 1
	}
	return 2
}

// setMono chooses the fold. It takes effect on the next chunk received; what is
// already in the ring keeps the fold it was written with, which is a few
// milliseconds of the previous choice and not worth re-deriving the ring to
// erase — and cannot be re-derived anyway once the source is one channel.
func (s *stereoRings) setMono(m MonoMode) { s.mono = m }
