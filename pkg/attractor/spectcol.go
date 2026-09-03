package attractor

// The spectrogram's column mapping, kept out of the wasm-only file so that it
// can be run on a machine as well as in a browser.
//
// The point of it being here is comparison. audioprism — the C++ original this
// all descends from — will render a WAV straight to a PNG, deterministically,
// with no audio device and no window. That is a reference worth having, and it
// is only usable as one if this side can be asked the same question the same
// way: same file in, same picture out. Reaching that through the browser would
// mean screenshotting a live canvas, which compares two different moments of
// two different renderings and settles nothing.
//
// So the mapping lives here, the wasm renderer calls it for the texture, and
// `uitool spec` calls it to write a PNG. Both get the same columns from the
// same code, which is the only way the comparison means anything.

import (
	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
)

// SpectrogramMaxRows caps how tall a column may be.
//
// A column carries one row per frequency bin, so an 8192-point transform wants
// 4096 of them, and the scrolling texture is 2048 columns wide — 32 MB of
// texture for a picture that no display has the pixels to show. Past this the
// rows are mapped by frequency onto fewer of them, which is a resampling and is
// said plainly rather than left to be discovered as a GPU allocation failure.
const SpectrogramMaxRows = 2048

// SpectrogramRows is how many frequency rows a column carries for a given
// transform size. It is size/2 — every bin the transform produces and nothing
// invented — so the bin→row map below is 1:1 at any sample rate and no
// resampling happens, up to the cap above.
//
// audioprism's own core UI carries twice this for the same bins (its map works
// out to bin = y/2, so each is stored twice); this is that picture without the
// duplication.
func SpectrogramRows(dftSize int) int {
	rows := dftSize / 2
	if rows > SpectrogramMaxRows {
		rows = SpectrogramMaxRows
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

// SpectrogramColumn maps one frame's FFT magnitudes to a column of RGBA bytes,
// bottom row = 0 Hz, top row = Nyquist, colored by audioprism's own scale.
//
// The color is sg.MagnitudeToPixel, which applies the configured magnitude
// scale and window and then the chosen color map — the same function the
// original uses, so a difference in the output is a difference in the
// magnitudes and not in how they were painted.
// The sample rate is not a parameter because it cancels. Row y stands for
// y/rows of the way up to Nyquist, so its frequency is y/rows × rate/2, and the
// bin holding that frequency is freq × size/rate — the rate divides out and
// what is left is y/rows × size/2, which is y/rows of the way up the bins. The
// mapping is therefore 1:1 whenever there is one row per bin, at any rate, and
// a proportional resampling when there is not.
func SpectrogramColumn(mags []float64, rows int) []byte {
	if len(mags) < 2 || rows < 1 {
		return nil
	}
	// len(mags)-1 rather than len(mags): the magnitudes run from DC up to and
	// including Nyquist, so there are size/2+1 of them and size/2 intervals.
	// The top row is Nyquist, not one bin short of it.
	bins := len(mags) - 1
	col := make([]byte, rows*4)
	for y := 0; y < rows; y++ {
		bin := int(float64(y) / float64(rows) * float64(bins))
		if bin < 0 || bin >= len(mags) {
			col[y*4+3] = 255
			continue
		}
		r, g, b, a := sg.MagnitudeToPixel(mags[bin]).RGBA()
		col[y*4+0] = byte(r >> 8 & 0xFF)
		col[y*4+1] = byte(g >> 8 & 0xFF)
		col[y*4+2] = byte(b >> 8 & 0xFF)
		col[y*4+3] = byte(a >> 8 & 0xFF)
	}
	return col
}

// SpectrogramMags is the magnitude spectrum of one frame, windowed with
// whichever window function the settings currently name and transformed the way
// the live pipeline does it — exported so an offline renderer runs the same
// arithmetic rather than a lookalike.
//
// It reads the window from the settings rather than taking it as an argument
// because that is where the control writes it, and a renderer that took it
// separately could be asked for one window while the picture was painted with
// another.
func SpectrogramMags(frame []float32) []float64 {
	return computeFFTMagsWindow(frame, sg.S.WindowFunc())
}
