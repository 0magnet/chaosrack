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
//
// The knobs themselves, what they tell audioprism-go and the column pipeline
// are SpectroControls and SpectroFeed (spectrofeed.go), which a native rack
// runs too.

import (
	"strconv"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
)

var spectCtl = *NewSpectroControls()

// The last three are lookup tables from perceptually uniform maps, added
// upstream in audioprism-go. Appended rather than reordered: the knob
// position is persisted as an index.
//
// spectColNames is the colormap order the MAP ring and pkg/colormap's maps
// both follow. The spectrogram no longer has a knob of its own to label with
// it — it reads the ring — but the order is still the contract between the
// library's tables and ours, so it is named here where the library is
// imported.
var spectColNames = []string{"heat", "blue", "grayscale", "turbo", "viridis", "magma"}

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
// screen at once, because the Spectro group keeps these controls while the
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

// applySpectSettings applies the knobs (SpectroControls.Apply), and rebuilds
// the texture when the transform size has changed.
func (s *SpectroControls) applySpectSettings() {
	if s.Apply() {
		spect.resizeSpectrogram()
	}
}

// monoMode turns the knob into the fold the source applies.
func (s *SpectroControls) monoMode() audiosrc.MonoMode {
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
