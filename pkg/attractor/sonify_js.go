//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"

	"github.com/go-gl/mathgl/mgl32"
)

// Model Out — HEAR the attractor, two ways (inner knob on the MAP cell):
//
// FLOW (default) audifies the DYNAMICS: a private integrator steps the same
// vector field the renderer draws (flowFor / flowregistry.go) at audio rate, so
// what plays is x(t)/y(t) themselves, time-scaled into the audible band —
// what a real analog attractor computer patched into a speaker does. Pitch
// is emergent (the system's own orbital frequency): chaos chirps and
// whooshes, periodic windows lock into steady tones, and parameter changes
// are audible as bifurcations. The RATE knob transposes in exact musical
// intervals (A4 = one integrator step per sample; +1 octave = double).
//
// SCAN treats the rendered trail as one period of a stereo waveform and
// traces it at exactly RATE Hz — the "oscilloscope music" trick in reverse:
// stable, playable pitch; the shape is the timbre. SCAN is also FLOW's
// fallback for trail modes with no registered vector field (parametric
// curves, 4D equation modes). Geometry modes (cube, torus, polyhedra…) are
// silent: they don't write the trail buffer, so there is nothing honest to
// play.
//
// Either way, a MAP ring picks the projection to the speakers — CAM uses the
// current camera-relative x/y (what you see is what you hear: rotating the
// model changes the sound), XY/XZ/YZ pick raw axis pairs; the third
// coordinate is the axis the ear can't see. Attractor coordinates are
// arbitrary in offset and scale, so each channel is adaptively centered and
// normalized (fast attack, slow release, like the spectrogram's level
// independence). LVL is the output gain.

var (
	sonifyCtx    js.Value
	sonifyNode   js.Value
	sonifyFn     js.Func
	sonifyActive bool

	sonifyMap   = "off"  // off | cam | xy | xz | yz
	sonifyMode  = "flow" // flow (audify the dynamics) | scan (trail as wavetable)
	sonifyHz    = 110.0  // scan: trail traces/sec · flow: transposition (440 = ×1)
	sonifyLevel = 0.6

	sonifyPhase float64 // scan: fractional position along the trail, in cycles

	// FLOW state: a private integrator of the SAME vector field the renderer
	// draws (via flowFor4 — 4D equation modes included), stepped at audio
	// rate. Pitch is emergent — the attractor's own orbital frequency — and
	// the knob transposes it.
	sonFlowMode         string  // mode the flow state was seeded for
	sonFX, sonFY, sonFZ float64 // current state
	sonFW               float64 // hidden 4th state (4D flows)
	sonPX, sonPY, sonPZ float64 // previous state (for sub-step interp)
	sonAcc              float64 // fractional steps owed

	// Per-channel adaptive centering + span (slow EMA of the buffer's
	// min/max) so any attractor lands at a comfortable, DC-free level.
	sonCenL, sonCenR   float64
	sonSpanL, sonSpanR = 1.0, 1.0

	// Preallocated per-callback scratch (the callback runs ~23×/s forever
	// while playing — allocating there is steady-state GC pressure on the
	// same thread as the render loop; see fvfDrainScratch for the pattern).
	sonScrL, sonScrR []float32
	sonZero          []float32
)

// sonifyWrite copies a []float32 into a WebAudio channel-data Float32Array
// without allocating intermediate typed arrays.
func sonifyWrite(dst js.Value, src []float32) {
	u8 := js.Global().Get("Uint8Array").New(dst.Get("buffer"), dst.Get("byteOffset"), len(src)*4)
	js.CopyBytesToJS(u8, sliceToByteSlice(src))
}

// sonifySample projects trail point (x,y,z) to a stereo pair per the MAP ring.
func sonifySample(x, y, z float32) (float64, float64) {
	switch sonifyMap {
	case "xy":
		return float64(x), float64(y)
	case "xz":
		return float64(x), float64(z)
	case "yz":
		return float64(y), float64(z)
	default: // cam — camera-relative: screen x → L, screen y → R
		v := movMatrix.Mul4x1(mgl32.Vec4{x, y, z, 1})
		return float64(v[0]), float64(v[1])
	}
}

// sonifyProcess is the stereo ScriptProcessor callback (runs in Go on the
// main thread, so reading vertBuf/movMatrix needs no synchronization).
func sonifyProcess(_ js.Value, args []js.Value) interface{} {
	if !sonifyActive {
		return nil
	}
	out := args[0].Get("outputBuffer")
	outL := out.Call("getChannelData", 0)
	outR := out.Call("getChannelData", 1)
	frames := out.Get("length").Int()
	sr := out.Get("sampleRate").Float()

	if len(sonScrL) < frames {
		sonScrL = make([]float32, frames)
		sonScrR = make([]float32, frames)
		sonZero = make([]float32, frames)
	}

	n := steps
	if sonifyMap == "off" || !isAttractorMode(selectedMode) || n < 2 || len(vertBuf) < n*4 {
		sonifyWrite(outL, sonZero[:frames])
		sonifyWrite(outR, sonZero[:frames])
		return nil
	}

	l := sonScrL[:frames]
	r := sonScrR[:frames]
	minL, maxL := 1e30, -1e30
	minR, maxR := 1e30, -1e30
	push := func(i int, x, y, z float32) {
		vl, vr := sonifySample(x, y, z)
		if vl < minL {
			minL = vl
		}
		if vl > maxL {
			maxL = vl
		}
		if vr < minR {
			minR = vr
		}
		if vr > maxR {
			maxR = vr
		}
		l[i], r[i] = float32(vl), float32(vr)
	}

	sys, haveFlow := flowFor4(selectedMode)
	if sonifyMode == "flow" && haveFlow {
		// FLOW: audify the dynamics — integrate the mode's own vector field
		// at audio rate. sonifyHz transposes: 440 (A4) = one integrator step
		// per sample; each octave doubles the rate, so the knob moves the
		// emergent pitch by exact musical intervals.
		if sonFlowMode != selectedMode {
			ic := initCondFor(selectedMode)
			sonFX, sonFY, sonFZ = float64(ic[0]), float64(ic[1]), float64(ic[2])
			if sonFX == 0 && sonFY == 0 && sonFZ == 0 {
				sonFX, sonFY, sonFZ = 0.1, 0, 0 // don't strand at a fixed point
			}
			sonFW = sys.w()
			sonPX, sonPY, sonPZ = sonFX, sonFY, sonFZ
			sonAcc = 0
			sonFlowMode = selectedMode
		}
		dt := sys.dt()
		stepRate := sonifyHz / 440.0
		const lim = 1e5
		for i := 0; i < frames; i++ {
			sonAcc += stepRate
			for sonAcc >= 1 {
				sonAcc--
				sonPX, sonPY, sonPZ = sonFX, sonFY, sonFZ
				dx, dy, dz, dw := sys.f(sonFX, sonFY, sonFZ, sonFW)
				sonFX += dt * dx
				sonFY += dt * dy
				sonFZ += dt * dz
				sonFW += dt * dw
				if !(sonFX > -lim && sonFX < lim && sonFY > -lim && sonFY < lim && sonFZ > -lim && sonFZ < lim && sonFW > -lim && sonFW < lim) {
					ic := initCondFor(selectedMode)
					sonFX, sonFY, sonFZ = float64(ic[0]), float64(ic[1]), float64(ic[2])
					if sonFX == 0 && sonFY == 0 && sonFZ == 0 {
						sonFX = 0.1
					}
					sonFW = 0
					sonPX, sonPY, sonPZ = sonFX, sonFY, sonFZ
				}
			}
			t := sonAcc // 0..1 between prev and current state
			push(i,
				float32(sonPX+(sonFX-sonPX)*t),
				float32(sonPY+(sonFY-sonPY)*t),
				float32(sonPZ+(sonFZ-sonPZ)*t))
		}
	} else {
		// SCAN: trail as wavetable — sweep the whole drawn trail sonifyHz
		// times per second (exact, knob-set pitch). Also the FLOW fallback
		// for trail modes without a registered vector field (parametric
		// curves). Geometry modes never reach here (the
		// isAttractorMode gate above): they don't write vertBuf.
		inc := sonifyHz / sr
		for i := 0; i < frames; i++ {
			sonifyPhase += inc
			if sonifyPhase >= 1 {
				sonifyPhase -= float64(int(sonifyPhase))
			}
			f := sonifyPhase * float64(n-1)
			j := int(f)
			t := float32(f - float64(j))
			a, b := j*4, (j+1)*4
			push(i,
				vertBuf[a]*(1-t)+vertBuf[b]*t,
				vertBuf[a+1]*(1-t)+vertBuf[b+1]*t,
				vertBuf[a+2]*(1-t)+vertBuf[b+2]*t)
		}
	}

	// Slow-tracking center + span per channel (adapt fast on growth so a
	// suddenly-larger shape doesn't clip; relax slowly on shrink).
	adapt := func(cen, span *float64, mn, mx float64) {
		if mx <= mn {
			return
		}
		mid, half := (mx+mn)/2, (mx-mn)/2
		*cen += (mid - *cen) * 0.05
		if half > *span {
			*span = half
		} else {
			*span += (half - *span) * 0.01
		}
		if *span < 1e-6 {
			*span = 1e-6
		}
	}
	adapt(&sonCenL, &sonSpanL, minL, maxL)
	adapt(&sonCenR, &sonSpanR, minR, maxR)

	norm := func(buf []float32, cen, span float64) {
		g := sonifyLevel / span
		for i, v := range buf {
			s := (float64(v) - cen) * g
			if s > 1 {
				s = 1
			} else if s < -1 {
				s = -1
			}
			buf[i] = float32(s)
		}
	}
	norm(l, sonCenL, sonSpanL)
	norm(r, sonCenR, sonSpanR)
	sonifyWrite(outL, l)
	sonifyWrite(outR, r)
	return nil
}

// sonifySync starts the audio graph when the MAP ring leaves "off", stops it
// when it returns there — the ring is the power switch, like the generators'
// channel ring (the click is also the user gesture WebAudio needs).
func sonifySync() {
	if sonifyMap != "off" {
		startSonify()
	} else {
		stopSonify()
	}
}

// sonifyModeSync detaches the subgraph in modes Model Out can't sonify and
// reattaches it in trail modes (called on every mode change) — cheaper than
// letting the callback stream zeros. A disconnected ScriptProcessor doesn't
// fire; reconnecting to the same destination twice is a spec'd no-op, and it
// can't suspend the SHARED context (that would silence FVF/gen/test tone).
func sonifyModeSync() {
	if !sonifyActive || !sonifyNode.Truthy() {
		return
	}
	if isAttractorMode(selectedMode) {
		sonifyNode.Call("connect", sonifyCtx.Get("destination"))
	} else {
		sonifyNode.Call("disconnect")
	}
}

func startSonify() {
	if sonifyActive {
		acquireAudioCtx("sonify")
		return
	}
	sonifyCtx = acquireAudioCtx("sonify")
	if !sonifyCtx.Truthy() {
		return
	}
	sonifyNode = sonifyCtx.Call("createScriptProcessor", 2048, 0, 2)
	sonifyFn = trackedFuncOf(sonifyProcess)
	sonifyNode.Set("onaudioprocess", sonifyFn)
	sonifyNode.Call("connect", sonifyCtx.Get("destination"))
	sonifyActive = true
}

func stopSonify() {
	if !sonifyActive {
		return
	}
	if sonifyNode.Truthy() {
		sonifyNode.Set("onaudioprocess", js.Null())
		sonifyNode.Call("disconnect")
	}
	sonifyNode, sonifyCtx = js.Undefined(), js.Undefined()
	sonifyFn.Release()
	sonifyActive = false
	releaseAudioCtx("sonify")
}

// buildSonifyModule builds the Model Out module's knobs: a TRACE-rate knob
// with the generators' octave dial, a LVL knob, and a MAP selector ring whose
// "off" position is the power switch. The trace/lvl sliders + LEDs are
// registry-owned (adoptDescControl in Run); this only adds the knob layer.
func buildSonifyModule() {
	freq := doc.Call("getElementById", "sonify-freq")
	fstack := doc.Call("getElementById", "sonify-fstack")
	lvl := doc.Call("getElementById", "sonify-lvl")
	lstack := doc.Call("getElementById", "sonify-lstack")
	mp := doc.Call("getElementById", "sonify-map")
	mstack := doc.Call("getElementById", "sonify-mstack")
	if !freq.Truthy() || !fstack.Truthy() {
		return
	}
	fknob := makeKnob(freq, js.Undefined(), true, false, false)
	addOctaveDial(fknob)
	fstack.Call("appendChild", fknob)
	lstack.Call("appendChild", makeKnob(lvl, js.Undefined(), true, false, true))
	md := doc.Call("getElementById", "sonify-mode")
	mstk := stackKnobs(makeSelectorKnob(mp), makeSelectorKnob(md))
	addSelectorLabels(mstk, []string{"off", "CAM", "XY", "XZ", "YZ"}, mp, 50)
	addSelectorLabels(mstk, []string{"FLOW", "SCAN"}, md, 38)
	mstack.Call("appendChild", mstk)

	mp.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		sonifyMap = mp.Get("value").String()
		sonifySync()
		return nil
	}))
	md.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		sonifyMode = md.Get("value").String()
		return nil
	}))
	sonifyMap = mp.Get("value").String()
	sonifyMode = md.Get("value").String()
}

// sonifyFreqFromSlider maps the semitone slider (A0-anchored, like the
// generators) to trail traces per second.
func sonifyFreqFromSlider(v float64) float64 { return freqFromKnob(v) }

// sonifySliderFromFreq is the inverse, for typed LED entry.
func sonifySliderFromFreq(hz float64) float64 {
	if hz <= 0 {
		return 0
	}
	v := knobFromFreq(hz)
	if v < 0 {
		v = 0
	} else if v > float64(genSemitones) {
		v = float64(genSemitones)
	}
	// keep sub-semitone precision from typed Hz
	s, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'f', 2, 64), 64)
	return s
}
