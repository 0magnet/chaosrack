//go:build js && wasm

package audiosrc

// Two channels over the WebSocket, and what a mono reader gets from them.
//
// The capture records the source as it is: a monitor of a stereo sink has two
// channels, and folding them together at the capture throws away the only thing
// a phase display has to show. So the server interleaves L,R,L,R… when asked
// (?ch=2) and this de-interleaves into a ring per channel.
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
