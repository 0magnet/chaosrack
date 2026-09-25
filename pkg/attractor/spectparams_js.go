//go:build js && wasm

package attractor

// The spectrogram's controls: everything the original audioprism can be told,
// as knobs.
//
// It had none. The mode rendered whatever audioprism-go's package defaults
// happened to be and there was no way to ask it for anything else — not a
// window function, not a magnitude window, not even a transform size — while
// the original has offered all of it from the command line and the keyboard
// since its first release. A port that renders one fixed configuration is a
// screenshot of the thing it is a port of.
//
// The seven here are the original's seven: --dft-size, --overlap, --window,
// --colors, --magnitude-scale, --magnitude-min, --magnitude-max. Its remaining
// options are about a window and a canvas — width, height, orientation,
// fullscreen — which this panel already governs by other means.

import (
	"math"
	"strconv"

	"github.com/0magnet/chaosrack/pkg/audiosrc"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
)

// spectControls is the spectrogram's controls.
type spectControls struct {
	// The knob values. Floats because every control in this panel is a float
	// behind a hidden slider, which is what makes reset, permalinks and audio
	// modulation work the same way for all of them.
	dftf   float32 // index into spectDFTSizes, not the size itself
	ovlF   float32
	winF   float32
	scaleF float32
	minF   float32
	maxF   float32

	// chanF is the knob, an index into spectChanNames.
	chanF float32
}

var spectCtl = spectControls{
	dftf: 4,
	ovlF: 50,
	maxF: 45,
}

// spectDFTSizes is the original's range, 64 to 8192, every power of two. The
// knob carries the index rather than the size so that its positions are evenly
// spaced — a knob running 64 to 8192 linearly would spend three quarters of its
// travel between 2048 and 8192 and be unable to stop on 128 at all.
var spectDFTSizes = []int{64, 128, 256, 512, 1024, 2048, 4096, 8192}

var (
	spectWinNames = []string{"hann", "hamming", "bartlett", "rectangular"}
	// The last three are lookup tables from perceptually uniform maps, added
	// upstream in audioprism-go. Appended rather than reordered: the knob
	// position is persisted as an index.
	// spectColNames is the colormap order the MAP ring and pkg/colormap's
	// maps both follow. The spectrogram no longer has a knob of its own to
	// label with it — it reads the ring — but the order is still the contract
	// between the library's tables and ours, so it is named here where the
	// library is imported.
	spectColNames   = []string{"heat", "blue", "grayscale", "turbo", "viridis", "magma"}
	spectScaleNames = []string{"logarithmic", "linear"}
)

func spectDFTNames() []string {
	out := make([]string, len(spectDFTSizes))
	for i, n := range spectDFTSizes {
		out[i] = strconv.Itoa(n) + "-point"
	}
	return out
}

// spectParams are the controls as the Parameters module builds them.
//
// WFN, not "win". The three delay embeddings label a window LENGTH in
// milliseconds "win", and this is a window FUNCTION — and the two can be on
// screen at once, because the Spectro module keeps these controls while the
// spectrogram is being used as a backdrop behind another model. One label
// meaning two things three inches apart is a label doing harm.
//
// The knob's id stays spect-win, so permalinks and patch routings written
// before the rename still land on it: the id is the wire and the label is what
// is painted on the panel beside it.
//
// The magnitude pair runs over the logarithmic limits, -80 to 80, because that
// is the scale the spectrogram is in unless it is told otherwise. In linear
// mode the original allows up to 1000, but its own scale toggle lands at 0..50
// and 1000 is only reachable by holding a key; a knob that could reach it would
// give up all its useful resolution to do so. Negative values are clamped away
// in linear mode, where a magnitude cannot be one.
var spectParams = []paramDef{
	{"spect-dft", "dft", &spectCtl.dftf, 4, 0, float32(len(spectDFTSizes) - 1), 1},
	{"spect-ovl", "ovlp", &spectCtl.ovlF, 50, 5, 95, 5},
	{"spect-win", "wfn", &spectCtl.winF, 0, 0, float32(len(spectWinNames) - 1), 1},
	{"spect-chan", "chan", &spectCtl.chanF, 0, 0, float32(len(spectChanNames) - 1), 1},
	{"spect-scale", "scale", &spectCtl.scaleF, 0, 0, float32(len(spectScaleNames) - 1), 1},
	{"spect-min", "min", &spectCtl.minF, 0, -80, 80, 1},
	{"spect-max", "max", &spectCtl.maxF, 45, -80, 80, 1},
}

// pick reads a knob as an index into a list, since a knob can be dragged past
// either end of one by audio modulation or by a permalink written by hand.
func pick(v float32, n int) int {
	i := max(int(v+0.5), 0)
	if i >= n {
		i = n - 1
	}
	return i
}

// applySpectSettings copies the knobs into the shared audioprism-go settings,
// which is where every renderer on this side reads them from — the live texture
// here and `uitool spec` offline both.
//
// Called every frame rather than on change. The knobs can move from the panel,
// from a MIDI controller, from an audio modulator or from a permalink being
// applied, and there is no single place all of those pass through; reading them
// once a frame is a handful of comparisons and cannot get out of step.
func (s *spectControls) applySpectSettings() {
	sg.S.SetWindowByName(spectWinNames[pick(s.winF, len(spectWinNames))])
	sg.S.SetScaleByName(spectScaleNames[pick(s.scaleF, len(spectScaleNames))])

	lo, hi := float64(s.minF), float64(s.maxF)
	if sg.S.MagScale() == sg.ScaleLinear && lo < 0 {
		lo = 0
	}
	// The window must not invert: Normalize would divide by a negative span and
	// paint the picture backwards rather than fail, so the ceiling gives way to
	// the floor rather than the other way around.
	if hi <= lo {
		hi = lo + 1
	}
	// Ceiling first — setting a floor above the old ceiling and then raising it
	// would pass through an inverted state.
	sg.S.SetMagMax(hi)
	sg.S.SetMagMin(lo)

	sg.S.SetOverlap(float64(s.ovlF) / 100.0)

	if size := spectDFTSizes[pick(s.dftf, len(spectDFTSizes))]; size != sg.S.GetDFTSize() {
		sg.S.SetDFTSize(size)
		spect.resizeSpectrogram()
	}
}

// spectChanNames are the folds of a stereo source the spectrogram can show.
//
// The mix is first because it is the honest answer to "what is playing", and a
// spectrogram of one channel of a stereo mix is a spectrogram of half of it.
// The single channels are worth having because the sum hides things: an
// instrument panned hard, a dead side of an interface, a channel out of
// polarity with the other that cancels in the sum and looks like silence.
//
// A mono source ignores this: both channels are the same signal, so every
// position shows the same picture.
var spectChanNames = []string{"mix", "left", "right"}

// monoMode turns the knob into the fold the source applies.
func (s *spectControls) monoMode() audiosrc.MonoMode {
	switch pick(s.chanF, len(spectChanNames)) {
	case 1:
		return audiosrc.MonoLeft
	case 2:
		return audiosrc.MonoRight
	default:
		return audiosrc.MonoMix
	}
}

// applySpectChannel pushes the knob to the source. The source folds as frames
// arrive rather than on read, so this only has to happen when the knob moves —
// but it is cheap, and calling it per frame means there is no separate place
// that has to remember to.
func applySpectChannel() {
	type monoSetter interface{ SetMonoMode(audiosrc.MonoMode) }
	if s, ok := aud.ensureAudioSource().(monoSetter); ok {
		s.SetMonoMode(spectCtl.monoMode())
	}
}

// The magnitude half of the spectrogram's coloring, split out so palette_js.go
// can normalize a magnitude without reaching into the library's settings lock
// at three separate call sites.
//
// spectMagnitude applies the scale knob: logarithmic is the decibel reading the
// MIN and MAX knobs are calibrated in, linear is the raw magnitude.
func spectMagnitude(v float64) float64 {
	if sg.S.MagScale() == sg.ScaleLog {
		return 20 * math.Log10(v+1e-10)
	}
	return v
}

// spectMagMin and spectMagMax are the window the normalization runs over — the
// MIN and MAX knobs, read back through the library so there is one copy of
// them rather than two that can drift.
func spectMagMin() float64 { lo, _ := sg.S.MagWindow(); return lo }
func spectMagMax() float64 { _, hi := sg.S.MagWindow(); return hi }
