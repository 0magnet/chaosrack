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
	"strconv"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
)

// The knob values. Floats because every control in this panel is a float
// behind a hidden slider, which is what makes reset, permalinks and audio
// modulation work the same way for all of them.
var (
	spectDFTF   float32 = 4 // index into spectDFTSizes, not the size itself
	spectOvlF   float32 = 50
	spectWinF   float32
	spectColF   float32
	spectScaleF float32
	spectMinF   float32
	spectMaxF   float32 = 45
)

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
// The magnitude pair runs over the logarithmic limits, -80 to 80, because that
// is the scale the spectrogram is in unless it is told otherwise. In linear
// mode the original allows up to 1000, but its own scale toggle lands at 0..50
// and 1000 is only reachable by holding a key; a knob that could reach it would
// give up all its useful resolution to do so. Negative values are clamped away
// in linear mode, where a magnitude cannot be one.
var spectParams = []paramDef{
	{"spect-dft", "dft", &spectDFTF, 4, 0, float32(len(spectDFTSizes) - 1), 1},
	{"spect-ovl", "ovlp", &spectOvlF, 50, 5, 95, 5},
	{"spect-win", "win", &spectWinF, 0, 0, float32(len(spectWinNames) - 1), 1},
	{"spect-col", "color", &spectColF, 0, 0, float32(len(spectColNames) - 1), 1},
	{"spect-scale", "scale", &spectScaleF, 0, 0, float32(len(spectScaleNames) - 1), 1},
	{"spect-min", "min", &spectMinF, 0, -80, 80, 1},
	{"spect-max", "max", &spectMaxF, 45, -80, 80, 1},
}

// pick reads a knob as an index into a list, since a knob can be dragged past
// either end of one by audio modulation or by a permalink written by hand.
func pick(v float32, n int) int {
	i := int(v + 0.5)
	if i < 0 {
		i = 0
	}
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
func applySpectSettings() {
	sg.S.SetWindowByName(spectWinNames[pick(spectWinF, len(spectWinNames))])
	sg.S.SetColorByName(spectColNames[pick(spectColF, len(spectColNames))])
	sg.S.SetScaleByName(spectScaleNames[pick(spectScaleF, len(spectScaleNames))])

	lo, hi := float64(spectMinF), float64(spectMaxF)
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

	sg.S.SetOverlap(float64(spectOvlF) / 100.0)

	if size := spectDFTSizes[pick(spectDFTF, len(spectDFTSizes))]; size != sg.S.GetDFTSize() {
		sg.S.SetDFTSize(size)
		resizeSpectrogram()
	}
}
