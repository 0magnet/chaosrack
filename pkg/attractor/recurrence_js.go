//go:build js && wasm

package attractor

import "syscall/js"

// The recurrence plot — R[i][j] lit when |s(t_i) − s(t_j)| < ε (Eckmann,
// Kamphorst & Ruelle, 1987). It is the one display here that shows the TIME
// structure of a signal as a shape rather than as something moving: a steady
// tone draws unbroken diagonals spaced by its period, a chaotic or noisy
// signal breaks them into short segments, a held note is a solid block, and
// the moment the source changes is a visible edge running across the square.
//
// It rides the spectrogram's rendering path — a texture on a plane through the
// shared 3D pipeline (textured_js.go) — so it rotates and zooms like every
// other model, on a SQUARE quad rather than the spectrogram's landscape one,
// because both of its axes are the same axis and the main diagonal has to come
// out at 45°.
//
// The matrix itself is computed in embedding.go, untagged, so the property
// that makes the picture legible — an exactly periodic signal gives a matrix
// invariant under shifting both indices by its period — is verified by a
// native test rather than by looking at it.
//
// ε IS A KNOB AND HOLDS STILL. The obvious convenience is to pick ε from the
// window's own spread each frame, and that is the same mistake this repo's
// Takens mode already made and undid: a threshold derived from the current
// audio changes the density of the plot in time with the music, so the picture
// pulses and none of it means anything. ε here is a fraction of full scale and
// nothing moves it but the user.

const (
	// rpN is the matrix side, and the texture is rpN×rpN. 256 columns over
	// the window is enough for the diagonals to be legible and costs 65536
	// comparisons per frame, which is nothing next to a frame of audio.
	rpN = 256

	// rpDrainCap bounds the per-frame drain the same way the Takens mode
	// does: the signal generator synthesizes on demand and always fills the
	// buffer, so an uncapped drain-until-dry loop never terminates.
	rpDrainCap = 16384
)

var (
	rpTexture js.Value
	rpU8      js.Value // reused Uint8Array, rpN*rpN bytes
	rpReady   bool

	rpRing    []float32
	rpW       int // monotonic write cursor
	rpScratch []float32
	rpPts     []float64 // the rpN sampled points of the current window
	rpMat     []byte    // rpN*rpN, one byte per cell

	// rpWin is the span the square covers, in milliseconds — how much history
	// the plot is of. 100 ms is a few dozen periods of a musical pitch across
	// the square, and at 48 kHz it decimates by 18, which leaves the fundamental
	// range intact after the box filter below.
	//
	// rpEps is the recurrence threshold as a fraction of full scale, so it
	// means the same thing at any level (see the file comment on why it is not
	// derived from the signal).
	rpWin float32 = 100
	rpEps float32 = 0.05
)

func init() {
	registerGenerate("recurrence", generateRecurrence)
	attractorParams["recurrence"] = []paramDef{
		{"rec-win", "win", &rpWin, 100, 20, 2000, 20},
		{"rec-eps", "ε", &rpEps, 0.05, 0.005, 0.5, 0.005},
	}
}

func initRecurrencePlot() {
	if rpReady {
		return
	}
	rpTexture = gl.Call("createTexture")
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), rpTexture)
	// NEAREST, not LINEAR: the cells are a yes/no answer, and interpolating
	// between them invents half-recurrences that are not in the signal — at
	// this size it also smears the single-pixel diagonals into a haze.
	gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_MIN_FILTER"), gl.Get("NEAREST"))
	gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_MAG_FILTER"), gl.Get("NEAREST"))
	gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_WRAP_S"), gl.Get("CLAMP_TO_EDGE"))
	gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_WRAP_T"), gl.Get("CLAMP_TO_EDGE"))

	// LUMINANCE rather than RGBA: a binary matrix needs one byte per cell, not
	// four, and the shared textured shader reads the single channel into all
	// three color channels for free.
	rpMat = make([]byte, rpN*rpN)
	rpU8 = js.Global().Get("Uint8Array").New(rpN * rpN)
	gl.Call("texImage2D",
		gl.Get("TEXTURE_2D"), 0, gl.Get("LUMINANCE"),
		rpN, rpN, 0,
		gl.Get("LUMINANCE"), gl.Get("UNSIGNED_BYTE"), rpU8)

	rpPts = make([]float64, rpN)
	rpScratch = make([]float32, 8192)
	rpReady = true
}

// rpWindow converts the WIN knob into a sample count and the stride that fits
// it into rpN columns. Split out from the generator so it can be tested
// without a GL context, as takensWindow is: it decides what the picture is OF.
func rpWindow(winMS float32, sampleRate int) (span, stride int) {
	if sampleRate <= 0 {
		sampleRate = 24000
	}
	span = int(winMS / 1000 * float32(sampleRate))
	if span < rpN {
		span = rpN
	}
	stride = span / rpN
	if stride < 1 {
		stride = 1
	}
	return stride * rpN, stride
}

// generateRecurrence drains the audio into the ring, rebuilds the matrix from
// the newest window and uploads it. Called from generateForMode.
func generateRecurrence() {
	if !rpReady {
		initRecurrencePlot()
	}
	src := ensureAudioSource()
	sr := 24000
	if src != nil && src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	span, stride := rpWindow(rpWin, sr)
	if need := span + 1; len(rpRing) < need {
		rpRing = make([]float32, need+need/2)
		rpW = 0
	}
	if src != nil && src.Ready() {
		for drained := 0; drained < rpDrainCap; {
			n := src.Drain(rpScratch)
			if n <= 0 {
				break
			}
			for i := 0; i < n; i++ {
				rpRing[rpW%len(rpRing)] = rpScratch[i]
				rpW++
			}
			drained += n
			if n < len(rpScratch) {
				break
			}
		}
	}
	avail := rpW
	if avail > len(rpRing) {
		avail = len(rpRing)
	}
	if avail >= span {
		// Decimate the window to one point per column by AVERAGING each
		// stride, not by taking one sample out of it. Taking one is what this
		// did first, and the plot came out as speckle with a diagonal through
		// it: a 500 ms window at 48 kHz is a stride of 93, which puts the
		// decimated Nyquist at 258 Hz, and the signal generator's own default
		// chord has a 294 Hz tone in it. What was being drawn was the
		// recurrences of an aliased signal — real structure, belonging to a
		// signal nobody was playing. A box average is the anti-alias filter
		// the decimation needs; it costs the detail above the new Nyquist,
		// which is detail the column count cannot represent either way.
		rn := len(rpRing)
		base := rpW - span
		for i := 0; i < rpN; i++ {
			var sum float32
			for k := 0; k < stride; k++ {
				sum += rpRing[(base+i*stride+k)%rn]
			}
			rpPts[i] = float64(sum) / float64(stride)
		}
		RecurrenceMatrix(rpPts, float64(rpEps), rpMat)
		js.CopyBytesToJS(rpU8, rpMat)
		gl.Call("bindTexture", gl.Get("TEXTURE_2D"), rpTexture)
		gl.Call("texSubImage2D",
			gl.Get("TEXTURE_2D"), 0, 0, 0, rpN, rpN,
			gl.Get("LUMINANCE"), gl.Get("UNSIGNED_BYTE"), rpU8)
	}
	drawTexturedSquare(rpTexture)
	maybeShowAudioStatus()
}
