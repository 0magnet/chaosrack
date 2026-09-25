//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/colormap"
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"github.com/0magnet/chaosrack/pkg/spectcol"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
)

// The spectrogram is a texture *provider*: it maintains a scrolling 2D
// texture (newest FFT column at the right, older columns wrapping around a
// ring) and lets any geometry display it. The "spectrogram" model draws it
// on a plane through the shared 3D pipeline (so it rotates/zooms like every
// other model); the skin feature paints the same texture onto surface
// models. Drawing lives in textured_js.go; this file only fills the texture.
//
// Sample flow: audiosrc.Source → Drain (continuous) → overlapping STFT, with
// the transform size and hop taken from the dft and ovlp knobs → color column
// → queue → flushed to the texture at a steady wall-clock rate so the scroll
// never stutters.

// Fixed texture size, independent of canvas: width = time columns, height =
// frequency bins.
//
// Height is one row per bin — spectcol.Rows(DFTSize) — which makes the bin→row
// mapping exactly 1:1 at any sample rate, so we keep the full FFT resolution
// with no resampling. audioprism's own core UI carries 1024 rows for the same
// 512 bins — its map works out to bin = y/2, so every bin is stored twice —
// which is the same picture at twice the memory; this is that picture without
// the duplication.
//
// Width matches audioprism's history exactly. It keeps 2048 columns and draws
// one per screen pixel, so at 1920 across it is showing the newest 1920 of
// them; at 1024 this held half the history and stretched it over whatever width
// the plane occupied, which is softer in time than the original for no reason
// other than the number.
const (
	spectTexW = 2048

	spectMaxQueue     = 120 // fast-forward the scroll if we fall this far behind
	spectQueueCatchup = 60

	// spectQueueTarget is the depth the queue is drained toward. Not zero: the
	// producer and the flush are on different clocks, so a column or two in
	// hand is what keeps the scroll smooth rather than stuttering whenever one
	// arrives a moment late.
	spectQueueTarget = 2

	// The most audio one update may pull in. A source backed by a generator
	// rather than a buffer never reports "nothing left", so the drain needs a
	// ceiling of its own or it runs until the heap does. 32768 samples is about
	// 0.7 s at 44.1 kHz — far more than a frame can turn into visible columns,
	// and small enough that the worst case is a fraction of a second of audio
	// held, not gigabytes.
	spectMaxAccum = 1 << 15
)

// spectrogram is the spectrogram texture provider: the scrolling texture, the
// columns queued for it, and the analysis window.
type spectrogram struct {
	// autoMap is whether the MAP ring holds the colormap followMode put there,
	// so leaving the mode can hand the ring back without taking a choice
	// somebody made while it was up.
	autoMap bool

	// texH follows the transform size — spectcol.Rows(DFTSize) — because the
	// dft knob can change it while the mode is running. It is not a constant for
	// that reason and for no other; at the default 1024-point transform it is the
	// 512 it always was.
	texH     int
	texture  js.Value
	ready    bool
	texCol   int
	colUint8 js.Value // reused Uint8Array, spect.texH*4 bytes

	// Overlapping-STFT state. accum buffers drained samples until a
	// full StepSize hop is available; overlap is the sliding window.
	overlap  []float32
	accum    []float32
	drainBuf []float32

	// Column pipeline: produced sample-locked (bursty) into the queue,
	// flushed to the texture at a steady wall-clock rate.
	colQueue [][]byte
	lastMs   float64
	colFrac  float64

	// Auto-rotate is disabled for a legible face-on default and restored
	// when leaving spectrogram mode, so other models keep their setting.
	savedAutoRotate bool
	autoRotateSaved bool

	// fill fixes the spectrogram/FVF plane face-on across the whole
	// canvas (the "Fill" switch) instead of the rotatable 3D placement.
	fill bool
}

var spect = spectrogram{
	texH: spectcol.Rows(sg.S.GetDFTSize()),
}

func (s *spectrogram) initSpectrogram() {
	if s.ready {
		return
	}
	s.texture = glctx.GL.Call("createTexture")
	glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), s.texture)
	glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_MIN_FILTER"), glctx.GL.Get("LINEAR"))
	glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_MAG_FILTER"), glctx.GL.Get("LINEAR"))
	glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_WRAP_S"), glctx.GL.Get("CLAMP_TO_EDGE"))
	glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_WRAP_T"), glctx.GL.Get("CLAMP_TO_EDGE"))
	zeroU8 := js.Global().Get("Uint8Array").New(spectTexW * s.texH * 4)
	glctx.GL.Call("texImage2D",
		glctx.GL.Get("TEXTURE_2D"), 0, glctx.GL.Get("RGBA"),
		spectTexW, s.texH, 0,
		glctx.GL.Get("RGBA"), glctx.GL.Get("UNSIGNED_BYTE"), zeroU8)

	s.colUint8 = js.Global().Get("Uint8Array").New(s.texH * 4)

	// The go-dsp FFT worker pool is pure overhead on single-threaded wasm.
	sg.SetSingleThreaded()

	s.overlap = make([]float32, sg.S.GetDFTSize())
	s.accum = s.accum[:0]
	s.drainBuf = make([]float32, 8192)
	s.colQueue = s.colQueue[:0]
	s.lastMs = 0
	s.colFrac = 0
	s.texCol = 0
	s.ready = true
}

// resizeSpectrogram rebuilds everything that is sized by the transform, after
// the dft knob has changed it.
//
// The texture goes with it: one row per bin means a different height, and a
// texture cannot be resized in place. The history is lost, which is honest —
// the columns already on it were computed at the old resolution and are not
// spectra of the same thing. The half-filled window goes too, for the same
// reason, and the queue with it.
func (s *spectrogram) resizeSpectrogram() {
	if !s.ready {
		return
	}
	s.ready = false
	if s.texture.Truthy() {
		glctx.GL.Call("deleteTexture", s.texture)
	}
	s.texH = spectcol.Rows(sg.S.GetDFTSize())
	s.initSpectrogram()
}

// renderSpectrogramMode is the "spectrogram" model's per-frame entry point,
// called from generateForMode. It keeps the scrolling texture current and
// draws it on the shared plane through texp.program (so camera/rotation from
// the normal render loop apply). nowMs is the rAF timestamp.
func (s *spectrogram) renderSpectrogramMode(nowMs float64) {
	if !s.ready {
		s.initSpectrogram()
	}
	spectCtl.applySpectSettings()
	aud.ensureAudioSource()
	s.updateSpectrogramTexture(nowMs)
	offset := float32(s.texCol) / float32(spectTexW)
	texp.drawTexturedPlane(s.texture, offset)
	aud.maybeShowAudioStatus()
}

// updateSpectrogramTexture drains the audio stream, advances the STFT, and
// flushes queued columns onto the texture. No geometry is drawn here.
func (s *spectrogram) updateSpectrogramTexture(nowMs float64) {
	// The channel knob is pushed to the source rather than applied on read:
	// the fold happens as frames arrive, so what is already in the ring keeps
	// the fold it was written with.
	applySpectChannel()
	if src := aud.activeAudioSource(); src != nil && src.Ready() {
		fvfOn := run.selectedMode == "fvf"
		// When the FVF audio engine is running it is the single drainer of the
		// source (and plays it out), and the tap switches its upstream to that
		// engine's already-processed output so display matches sound. Reading
		// the tap therefore covers both states, and this no longer reaches into
		// fvf.vis itself — doing that was what made the spectrogram the only
		// display FVF worked with.
		listening := fvfOn && fvf.audioActive
		if fvfOn && !listening {
			fvf.ensureFVFProc()
		}
		// BOUNDED, because "drain until the source runs dry" assumes the source
		// can run dry. A GENERATOR CANNOT: FuncGen.Drain synthesizes on demand
		// and always returns a full buffer, so a loop that only stops on a
		// partial fill never stops at all. It appended 32 KB per turn until the
		// heap was gone — 3.8 GB in use, then a fatal out-of-memory on the next
		// doubling — and a wasm fatal error is a blank page, not one broken
		// feature. Switching the test tone on and then the spectrogram backdrop
		// was the whole reproduction.
		//
		// The cap is what one call can use. flushSpectColumns is paced by the
		// wall clock and fast-forwards anything past spectMaxQueue, so samples
		// drained beyond about a frame's worth become columns that are thrown
		// away as they arrive.
		for len(s.accum) < spectMaxAccum {
			n := tapRead(&spectCursor, s.drainBuf)
			// Under FVF the tap already carries processed samples, so only the
			// not-listening case still has to run the filter here.
			if fvfOn && !listening && fvf.proc != nil {
				for i := range n {
					s.drainBuf[i] = fvf.proc.Process(s.drainBuf[i])
				}
			}
			if n == 0 {
				break
			}
			s.accum = append(s.accum, s.drainBuf[:n]...)
			if n < len(s.drainBuf) {
				break
			}
		}
		size, step := len(s.overlap), sg.S.StepSize()
		if step < 1 {
			step = 1
		}
		if step > size {
			step = size
		}
		consumed := 0
		for len(s.accum)-consumed >= step {
			// Slide the window by one hop: keep the size-step samples the next
			// window shares with this one, append the step that follows them.
			// Those two counts are equal only at 50% overlap, which is why they
			// are written out separately rather than both called "step".
			copy(s.overlap, s.overlap[step:])
			copy(s.overlap[size-step:], s.accum[consumed:consumed+step])
			consumed += step
			if col := s.buildSpectColumn(spectcol.Mags(s.overlap)); col != nil {
				s.colQueue = append(s.colQueue, col)
			}
		}
		s.accum = append(s.accum[:0], s.accum[consumed:]...)
	}
	s.flushSpectColumns(nowMs)
}

// flushSpectColumns pushes queued columns onto the texture at the audio
// column rate (SampleRate/StepSize per second), paced by wall-clock time
// rather than frame/burst timing. Backlog beyond spectMaxQueue is
// fast-forwarded so we never fall permanently behind.
func (s *spectrogram) flushSpectColumns(nowMs float64) {
	if s.lastMs == 0 {
		s.lastMs = nowMs
	}
	elapsed := nowMs - s.lastMs
	s.lastMs = nowMs
	if elapsed < 0 {
		elapsed = 0
	}

	sampleRate := 24000
	if src := aud.activeAudioSource(); src != nil && src.SampleRate() > 0 {
		sampleRate = src.SampleRate()
	}
	step := max(sg.S.StepSize(), 1)
	colsPerMs := float64(sampleRate) / float64(step) / 1000.0

	s.colFrac += elapsed * colsPerMs
	toFlush := int(s.colFrac)
	s.colFrac -= float64(toFlush)

	for i := 0; i < toFlush && len(s.colQueue) > 0; i++ {
		s.uploadSpectColumn(s.colQueue[0])
		s.colQueue = s.colQueue[1:]
	}
	// Work off a standing backlog. The pacing above flushes at exactly the rate
	// columns are produced, so a queue — however it formed — is never worked
	// off: it just becomes permanent delay between what is heard and what is
	// drawn. Behind the Takens embedding this parked at seventeen columns, a
	// third of a second, and stayed there.
	//
	// One extra column per call clears that in about as long as it represents,
	// and one column is a single texel of scroll, so the correction is not
	// visible as a jump. The fast-forward below still handles the large
	// backlogs this is too gentle for.
	if len(s.colQueue) > spectQueueTarget {
		s.uploadSpectColumn(s.colQueue[0])
		s.colQueue = s.colQueue[1:]
	}
	if len(s.colQueue) > spectMaxQueue {
		drop := len(s.colQueue) - spectQueueCatchup
		for i := range drop {
			s.uploadSpectColumn(s.colQueue[i])
		}
		s.colQueue = s.colQueue[drop:]
	}
	if len(s.colQueue) == 0 {
		s.colQueue = s.colQueue[:0]
	}
}

// uploadSpectColumn writes one prepared RGBA column at the current write
// position and advances the scroll cursor.
func (s *spectrogram) uploadSpectColumn(col []byte) {
	js.CopyBytesToJS(s.colUint8, col)
	glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), s.texture)
	glctx.GL.Call("texSubImage2D",
		glctx.GL.Get("TEXTURE_2D"), 0,
		s.texCol, 0, 1, s.texH,
		glctx.GL.Get("RGBA"), glctx.GL.Get("UNSIGNED_BYTE"), s.colUint8)
	s.texCol = (s.texCol + 1) % spectTexW
}

// buildSpectColumn maps FFT magnitudes to one RGBA column (spect.texH*4 bytes),
// full 0..Nyquist with 0 Hz at the bottom, matching audioprism-go. The mapping
// itself is in pkg/spectcol, without a build tag, so that `uitool spec` can run
// the identical arithmetic on a machine and be diffed against the original's
// own WAV→PNG render.
func (s *spectrogram) buildSpectColumn(mags []float64) []byte {
	// Through the MAP ring, like everything else in the rack. The spectrogram
	// used to carry its own colormap knob naming the same six maps in the same
	// order, and two knobs that had to be kept in step by hand meant the
	// spectrogram and the trace beside it could disagree about what a value
	// looks like — which is the one thing sharing the library's tables was for.
	return spectcol.ColumnWith(mags, s.texH, spectrogramPixel)
}

// setSpectrogramCamera frames the plane at a sensible default distance,
// faces it toward the camera (identity pose), and stops it tumbling —
// randomizeOrientation's random pose + per-axis spin rates are great for
// attractors but make the spectrogram unreadable. Auto-rotate is turned
// off for a static default and restored on leaving the mode. Rotation
// stays available via drag, the X/Y/Z sliders, and the auto-rotate box.
// Used instead of autoFitCamera (which reads attractor vertices).
func (s *spectrogram) setSpectrogramCamera() {
	view.initDist = 4.5
	view.defaultDist = 4.5
	view.ctl.zoom = 0
	if camPanel.cameraControl.Truthy() {
		camPanel.cameraControl.Set("value", "0")
	}
	if camPanel.sliderZoom.Truthy() {
		camPanel.sliderZoom.Set("textContent", "0")
	}

	view.angleX, view.angleY, view.angleZ = 0, 0, 0
	view.rebuildModelMatrix()
	zeroRotationSliders()
	rotKnobs.update()

	if !s.autoRotateSaved {
		s.savedAutoRotate = view.ctl.autoRotate
		s.autoRotateSaved = true
	}
	clearAutoRotateFlag() // Y spin already zeroed above

	view.updateViewMatrix()
	view.updateModelMatrix()
}

// restoreAutoRotateAfterSpectrogram puts auto-rotate back to whatever it
// was before spectrogram mode disabled it. Called when switching to a
// non-spectrogram model.
func (s *spectrogram) restoreAutoRotateAfterSpectrogram() {
	if !s.autoRotateSaved {
		return
	}
	s.autoRotateSaved = false
	setAutoRotate(s.savedAutoRotate) // re-add the Y-rate contribution if it was on
}

// zeroRotationSliders resets the X/Y/Z rotation-rate sliders (and the
// Go-side cache) to zero so the plane holds still.
func zeroRotationSliders() {
	for _, id := range []string{"rotation-controls-x", "rotation-controls-y", "rotation-controls-z"} {
		el := dom.Doc.Call("getElementById", id)
		if !el.Truthy() {
			continue
		}
		el.Set("value", "0")
		// the registry's input listener updates the cache + LED format
		el.Call("dispatchEvent", js.Global().Get("Event").New("input"))
	}
	syncKnobs()
}

// spectrogramMap is the MAP position the spectrogram comes up on: heat, the
// default of the spectrogram's own color knob before that knob became the
// MAP ring. mapDefault is the ring's default, the two-color ramp.
var spectrogramMap = strconv.Itoa(colormap.First)

const mapDefault = "2"

// followMode moves the MAP ring onto the spectrogram's colormap when a
// spectrogram comes up and hands it back when it goes.
//
// When the spectrogram had its own color knob it opened on heat. Folding that
// knob into the MAP ring left it opening on the ring's default, a two-color
// ramp made for coloring a trace by a coordinate — the wrong language for a
// magnitude, and not what anyone who knew the old spectrogram expects to see.
//
// It is emb.autoSet's rule: never over a choice. The ring is moved only from
// its default, and moved back only if it still holds what this put there, so
// a map chosen on the panel or carried in a link stays where it was put.
func (s *spectrogram) followMode(mode string) {
	cur := strconv.Itoa(style.gradientColors)
	if isSpectroSurface(mode) {
		if cur == mapDefault && (inPageRack{}).Set("gradient-colors", spectrogramMap) == nil {
			s.autoMap = true
		}
		return
	}
	if s.autoMap {
		s.autoMap = false
		if cur == spectrogramMap {
			_ = inPageRack{}.Set("gradient-colors", mapDefault)
		}
	}
}
