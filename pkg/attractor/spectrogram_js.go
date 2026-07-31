//go:build js && wasm

package attractor

import (
	"syscall/js"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
)

// The spectrogram is a texture *provider*: it maintains a scrolling 2D
// texture (newest FFT column at the right, older columns wrapping around a
// ring) and lets any geometry display it. The "spectrogram" model draws it
// on a plane through the shared 3D pipeline (so it rotates/zooms like every
// other model); the skin feature paints the same texture onto surface
// models. Drawing lives in textured_js.go; this file only fills the texture.
//
// Sample flow: audiosrc.Source → Drain (continuous) → overlapping STFT
// (1024 FFT, 512 hop, 50% overlap) → color column → queue → flushed to the
// texture at a steady wall-clock rate so the scroll never stutters.

// Fixed texture size, independent of canvas: width = time columns, height =
// frequency bins. Height == FFTSize/2 makes the bin→row mapping exactly 1:1
// at any sample rate, so we keep the full FFT resolution with no resampling.
const (
	spectTexW = 1024
	spectTexH = 512

	spectMaxQueue     = 120 // fast-forward the scroll if we fall this far behind
	spectQueueCatchup = 60
)

var (
	spectTexture  js.Value
	spectReady    bool
	spectTexCol   int
	spectColUint8 js.Value // reused Uint8Array, spectTexH*4 bytes

	// Overlapping-STFT state. spectAccum buffers drained samples until a
	// full StepSize hop is available; spectOverlap is the sliding window.
	spectOverlap  []float32
	spectAccum    []float32
	spectDrainBuf []float32

	// Column pipeline: produced sample-locked (bursty) into the queue,
	// flushed to the texture at a steady wall-clock rate.
	spectColQueue [][]byte
	spectLastMs   float64
	spectColFrac  float64

	// Auto-rotate is disabled for a legible face-on default and restored
	// when leaving spectrogram mode, so other models keep their setting.
	specSavedAutoRotate bool
	specAutoRotateSaved bool

	// spectFill fixes the spectrogram/FVF plane face-on across the whole
	// canvas (the "Fill" switch) instead of the rotatable 3D placement.
	spectFill bool
)

func initSpectrogram() {
	if spectReady {
		return
	}
	spectTexture = gl.Call("createTexture")
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), spectTexture)
	gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_MIN_FILTER"), gl.Get("LINEAR"))
	gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_MAG_FILTER"), gl.Get("LINEAR"))
	gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_WRAP_S"), gl.Get("CLAMP_TO_EDGE"))
	gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_WRAP_T"), gl.Get("CLAMP_TO_EDGE"))
	zeroU8 := js.Global().Get("Uint8Array").New(spectTexW * spectTexH * 4)
	gl.Call("texImage2D",
		gl.Get("TEXTURE_2D"), 0, gl.Get("RGBA"),
		spectTexW, spectTexH, 0,
		gl.Get("RGBA"), gl.Get("UNSIGNED_BYTE"), zeroU8)

	spectColUint8 = js.Global().Get("Uint8Array").New(spectTexH * 4)

	// The go-dsp FFT worker pool is pure overhead on single-threaded wasm.
	sg.SetSingleThreaded()

	spectOverlap = make([]float32, sg.FFTSize)
	spectAccum = spectAccum[:0]
	spectDrainBuf = make([]float32, 8192)
	spectColQueue = spectColQueue[:0]
	spectLastMs = 0
	spectColFrac = 0
	spectTexCol = 0
	spectReady = true
}

// renderSpectrogramMode is the "spectrogram" model's per-frame entry point,
// called from generateForMode. It keeps the scrolling texture current and
// draws it on the shared plane through texProgram (so camera/rotation from
// the normal render loop apply). nowMs is the rAF timestamp.
func renderSpectrogramMode(nowMs float64) {
	if !spectReady {
		initSpectrogram()
	}
	ensureAudioSource()
	updateSpectrogramTexture(nowMs)
	offset := float32(spectTexCol) / float32(spectTexW)
	drawTexturedPlane(spectTexture, offset)
	maybeShowAudioStatus()
}

// updateSpectrogramTexture drains the audio stream, advances the STFT, and
// flushes queued columns onto the texture. No geometry is drawn here.
func updateSpectrogramTexture(nowMs float64) {
	if src := audioSource; src != nil && src.Ready() {
		fvfOn := selectedMode == "fvf"
		// When the FVF audio engine is running it is the single drainer of
		// the source (and plays it out); the spectrogram then reads its
		// already-processed output (fvfVis) so display matches sound. Off,
		// the spectrogram drains the source and processes it here.
		listening := fvfOn && fvfAudioActive
		if fvfOn && !listening {
			ensureFVFProc()
		}
		for {
			var n int
			if listening {
				n = fvfVis.drain(spectDrainBuf)
			} else {
				n = src.Drain(spectDrainBuf)
				if fvfOn && fvfProc != nil {
					for i := 0; i < n; i++ {
						spectDrainBuf[i] = fvfProc.Process(spectDrainBuf[i])
					}
				}
			}
			if n == 0 {
				break
			}
			spectAccum = append(spectAccum, spectDrainBuf[:n]...)
			if n < len(spectDrainBuf) {
				break
			}
		}
		step := sg.StepSize
		consumed := 0
		for len(spectAccum)-consumed >= step {
			// Slide the window: drop the oldest StepSize, append the next
			// StepSize. At 50% overlap FFTSize-StepSize == StepSize.
			copy(spectOverlap, spectOverlap[step:])
			copy(spectOverlap[sg.FFTSize-step:], spectAccum[consumed:consumed+step])
			consumed += step
			if col := buildSpectColumn(computeFFTMags(spectOverlap)); col != nil {
				spectColQueue = append(spectColQueue, col)
			}
		}
		spectAccum = append(spectAccum[:0], spectAccum[consumed:]...)
	}
	flushSpectColumns(nowMs)
}

// flushSpectColumns pushes queued columns onto the texture at the audio
// column rate (SampleRate/StepSize per second), paced by wall-clock time
// rather than frame/burst timing. Backlog beyond spectMaxQueue is
// fast-forwarded so we never fall permanently behind.
func flushSpectColumns(nowMs float64) {
	if spectLastMs == 0 {
		spectLastMs = nowMs
	}
	elapsed := nowMs - spectLastMs
	spectLastMs = nowMs
	if elapsed < 0 {
		elapsed = 0
	}

	sampleRate := 24000
	if audioSource != nil && audioSource.SampleRate() > 0 {
		sampleRate = audioSource.SampleRate()
	}
	colsPerMs := float64(sampleRate) / float64(sg.StepSize) / 1000.0

	spectColFrac += elapsed * colsPerMs
	toFlush := int(spectColFrac)
	spectColFrac -= float64(toFlush)

	for i := 0; i < toFlush && len(spectColQueue) > 0; i++ {
		uploadSpectColumn(spectColQueue[0])
		spectColQueue = spectColQueue[1:]
	}
	if len(spectColQueue) > spectMaxQueue {
		drop := len(spectColQueue) - spectQueueCatchup
		for i := 0; i < drop; i++ {
			uploadSpectColumn(spectColQueue[i])
		}
		spectColQueue = spectColQueue[drop:]
	}
	if len(spectColQueue) == 0 {
		spectColQueue = spectColQueue[:0]
	}
}

// uploadSpectColumn writes one prepared RGBA column at the current write
// position and advances the scroll cursor.
func uploadSpectColumn(col []byte) {
	js.CopyBytesToJS(spectColUint8, col)
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), spectTexture)
	gl.Call("texSubImage2D",
		gl.Get("TEXTURE_2D"), 0,
		spectTexCol, 0, 1, spectTexH,
		gl.Get("RGBA"), gl.Get("UNSIGNED_BYTE"), spectColUint8)
	spectTexCol = (spectTexCol + 1) % spectTexW
}

// buildSpectColumn maps FFT magnitudes to one RGBA column (spectTexH*4
// bytes), full 0..Nyquist with 0 Hz at the bottom (v=0), matching
// audioprism-go. With spectTexH == FFTSize/2 the bin→row map is 1:1.
func buildSpectColumn(mags []float64) []byte {
	if len(mags) == 0 {
		return nil
	}
	col := make([]byte, spectTexH*4)
	sampleRate := 24000
	if audioSource != nil && audioSource.SampleRate() > 0 {
		sampleRate = audioSource.SampleRate()
	}
	maxFreq := float64(sampleRate) / 2.0
	for y := 0; y < spectTexH; y++ {
		freq := float64(y) / float64(spectTexH) * maxFreq
		bin := int(freq * float64(sg.FFTSize) / float64(sampleRate))
		if bin < 0 || bin >= len(mags) {
			col[y*4+3] = 255
			continue
		}
		c := sg.MagnitudeToPixel(mags[bin])
		r, g, b, a := c.RGBA()
		col[y*4+0] = byte(r >> 8)
		col[y*4+1] = byte(g >> 8)
		col[y*4+2] = byte(b >> 8)
		col[y*4+3] = byte(a >> 8)
	}
	return col
}

// setSpectrogramCamera frames the plane at a sensible default distance,
// faces it toward the camera (identity pose), and stops it tumbling —
// randomizeOrientation's random pose + per-axis spin rates are great for
// attractors but make the spectrogram unreadable. Auto-rotate is turned
// off for a static default and restored on leaving the mode. Rotation
// stays available via drag, the X/Y/Z sliders, and the auto-rotate box.
// Used instead of autoFitCamera (which reads attractor vertices).
func setSpectrogramCamera() {
	initCameraDist = 4.5
	defaultCameraDist = 4.5
	cachedZoom = 0
	if cameraControl.Truthy() {
		cameraControl.Set("value", "0")
	}
	if sliderZoom.Truthy() {
		sliderZoom.Set("textContent", "0")
	}

	angleX, angleY, angleZ = 0, 0, 0
	rebuildModelMatrix()
	zeroRotationSliders()
	updateRotKnobs()

	if !specAutoRotateSaved {
		specSavedAutoRotate = autoRotate
		specAutoRotateSaved = true
	}
	clearAutoRotateFlag() // Y spin already zeroed above

	updateViewMatrix()
	updateModelMatrix()
}

// restoreAutoRotateAfterSpectrogram puts auto-rotate back to whatever it
// was before spectrogram mode disabled it. Called when switching to a
// non-spectrogram model.
func restoreAutoRotateAfterSpectrogram() {
	if !specAutoRotateSaved {
		return
	}
	specAutoRotateSaved = false
	setAutoRotate(specSavedAutoRotate) // re-add the Y-rate contribution if it was on
}

// zeroRotationSliders resets the X/Y/Z rotation-rate sliders (and the
// Go-side cache) to zero so the plane holds still.
func zeroRotationSliders() {
	for _, id := range []string{"rotation-controls-x", "rotation-controls-y", "rotation-controls-z"} {
		el := doc.Call("getElementById", id)
		if !el.Truthy() {
			continue
		}
		el.Set("value", "0")
		// the registry's input listener updates the cache + LED format
		el.Call("dispatchEvent", js.Global().Get("Event").New("input"))
	}
	syncKnobs()
}
