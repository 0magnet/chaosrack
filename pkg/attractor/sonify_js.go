//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/dynamics"
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

// sonifier is Model Out: the audio graph that makes the attractor heard, and
// the running state of both ways of hearing it.
type sonifier struct {
	ctx     js.Value
	node    js.Value
	fn      js.Func
	active  bool
	mapping string  // off | cam | xy | xz | yz
	mode    string  // flow (audify the dynamics) | scan (trail as wavetable)
	hz      float64 // scan: trail traces/sec · flow: transposition (440 = ×1)
	level   float64
	phase   float64 // scan: fractional position along the trail, in cycles

	// FLOW state: a private integrator of the SAME vector field the renderer
	// draws (via dynamics.FlowFor4 — 4D equation modes included), stepped at audio
	// rate. Pitch is emergent — the attractor's own orbital frequency — and
	// the knob transposes it.
	flowMode   string  // mode the flow state was seeded for
	fx, fy, fz float64 // current state
	fw         float64 // hidden 4th state (4D flows)
	px, py, pz float64 // previous state (for sub-step interp)
	acc        float64 // fractional steps owed

	// Per-channel adaptive centering + span (slow EMA of the buffer's
	// min/max) so any attractor lands at a comfortable, DC-free level.
	cenL, cenR   float64
	spanL, spanR float64

	// Preallocated per-callback scratch (the callback runs ~23×/s forever
	// while playing — allocating there is steady-state GC pressure on the
	// same thread as the render loop; see fvfDrainScratch for the pattern).
	scrL, scrR []float32
	zero       []float32
}

var son = sonifier{
	mapping: "off",
	mode:    "flow",
	hz:      110.0,
	level:   0.6,
	spanL:   1.0,
	spanR:   1.0,
}

// sonifyWrite copies a []float32 into a WebAudio channel-data Float32Array
// without allocating intermediate typed arrays.
func sonifyWrite(dst js.Value, src []float32) {
	u8 := js.Global().Get("Uint8Array").New(dst.Get("buffer"), dst.Get("byteOffset"), len(src)*4)
	js.CopyBytesToJS(u8, sliceToByteSlice(src))
}

// sonifySample projects trail point (x,y,z) to a stereo pair per the MAP ring.
func (so *sonifier) sample(x, y, z float32) (float64, float64) {
	switch so.mapping {
	case "xy":
		return float64(x), float64(y)
	case "xz":
		return float64(x), float64(z)
	case "yz":
		return float64(y), float64(z)
	default: // cam — camera-relative: screen x → L, screen y → R
		v := view.modelMat.Mul4x1(mgl32.Vec4{x, y, z, 1})
		return float64(v[0]), float64(v[1])
	}
}

// sonifyProcess is the stereo ScriptProcessor callback (runs in Go on the
// main thread, so reading vertBuf/view.modelMat needs no synchronization).
func (so *sonifier) process(_ js.Value, args []js.Value) interface{} {
	if !so.active {
		return nil
	}
	out := args[0].Get("outputBuffer")
	outL := out.Call("getChannelData", 0)
	outR := out.Call("getChannelData", 1)
	frames := out.Get("length").Int()
	sr := out.Get("sampleRate").Float()

	if len(so.scrL) < frames {
		so.scrL = make([]float32, frames)
		so.scrR = make([]float32, frames)
		so.zero = make([]float32, frames)
	}

	n := steps
	if so.mapping == "off" || !isAttractorMode(selectedMode) || n < 2 || len(vertBuf) < n*4 {
		sonifyWrite(outL, so.zero[:frames])
		sonifyWrite(outR, so.zero[:frames])
		return nil
	}

	l := so.scrL[:frames]
	r := so.scrR[:frames]
	minL, maxL := 1e30, -1e30
	minR, maxR := 1e30, -1e30
	push := func(i int, x, y, z float32) {
		vl, vr := so.sample(x, y, z)
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

	sys, haveFlow := dynamics.FlowFor4(selectedMode)
	if so.mode == "flow" && haveFlow {
		// FLOW: audify the dynamics — integrate the mode's own vector field
		// at audio rate. sonifyHz transposes: 440 (A4) = one integrator step
		// per sample; each octave doubles the rate, so the knob moves the
		// emergent pitch by exact musical intervals.
		if so.flowMode != selectedMode {
			ic := dynamics.InitCondFor(selectedMode)
			so.fx, so.fy, so.fz = float64(ic[0]), float64(ic[1]), float64(ic[2])
			if so.fx == 0 && so.fy == 0 && so.fz == 0 {
				so.fx, so.fy, so.fz = 0.1, 0, 0 // don't strand at a fixed point
			}
			so.fw = sys.W()
			so.px, so.py, so.pz = so.fx, so.fy, so.fz
			so.acc = 0
			so.flowMode = selectedMode
		}
		dt := sys.Dt()
		stepRate := so.hz / 440.0
		const lim = 1e5
		for i := 0; i < frames; i++ {
			so.acc += stepRate
			for so.acc >= 1 {
				so.acc--
				so.px, so.py, so.pz = so.fx, so.fy, so.fz
				dx, dy, dz, dw := sys.F(so.fx, so.fy, so.fz, so.fw)
				so.fx += dt * dx
				so.fy += dt * dy
				so.fz += dt * dz
				so.fw += dt * dw
				if !(so.fx > -lim && so.fx < lim && so.fy > -lim && so.fy < lim && so.fz > -lim && so.fz < lim && so.fw > -lim && so.fw < lim) {
					ic := dynamics.InitCondFor(selectedMode)
					so.fx, so.fy, so.fz = float64(ic[0]), float64(ic[1]), float64(ic[2])
					if so.fx == 0 && so.fy == 0 && so.fz == 0 {
						so.fx = 0.1
					}
					so.fw = 0
					so.px, so.py, so.pz = so.fx, so.fy, so.fz
				}
			}
			t := so.acc // 0..1 between prev and current state
			push(i,
				float32(so.px+(so.fx-so.px)*t),
				float32(so.py+(so.fy-so.py)*t),
				float32(so.pz+(so.fz-so.pz)*t))
		}
	} else {
		// SCAN: trail as wavetable — sweep the whole drawn trail sonifyHz
		// times per second (exact, knob-set pitch). Also the FLOW fallback
		// for trail modes without a registered vector field (parametric
		// curves). Geometry modes never reach here (the
		// isAttractorMode gate above): they don't write vertBuf.
		inc := so.hz / sr
		for i := 0; i < frames; i++ {
			so.phase += inc
			if so.phase >= 1 {
				so.phase -= float64(int(so.phase))
			}
			f := so.phase * float64(n-1)
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
	adapt(&so.cenL, &so.spanL, minL, maxL)
	adapt(&so.cenR, &so.spanR, minR, maxR)

	norm := func(buf []float32, cen, span float64) {
		g := so.level / span
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
	norm(l, so.cenL, so.spanL)
	norm(r, so.cenR, so.spanR)
	sonifyWrite(outL, l)
	sonifyWrite(outR, r)
	return nil
}

// sonifySync starts the audio graph when the MAP ring leaves "off", stops it
// when it returns there — the ring is the power switch, like the generators'
// channel ring (the click is also the user gesture WebAudio needs).
func (so *sonifier) sync() {
	if so.mapping != "off" {
		so.start()
	} else {
		so.stop()
	}
}

// sonifyModeSync detaches the subgraph in modes Model Out can't sonify and
// reattaches it in trail modes (called on every mode change) — cheaper than
// letting the callback stream zeros. A disconnected ScriptProcessor doesn't
// fire; reconnecting to the same destination twice is a spec'd no-op, and it
// can't suspend the SHARED context (that would silence FVF/gen/test tone).
func (so *sonifier) modeSync() {
	if !so.active || !so.node.Truthy() {
		return
	}
	if isAttractorMode(selectedMode) {
		so.node.Call("connect", so.ctx.Get("destination"))
	} else {
		so.node.Call("disconnect")
	}
}

func (so *sonifier) start() {
	if so.active {
		acquireAudioCtx("sonify")
		return
	}
	so.ctx = acquireAudioCtx("sonify")
	if !so.ctx.Truthy() {
		return
	}
	so.node = so.ctx.Call("createScriptProcessor", 2048, 0, 2)
	so.fn = dom.FuncOf(so.process)
	so.node.Set("onaudioprocess", so.fn)
	so.node.Call("connect", so.ctx.Get("destination"))
	so.active = true
}

func (so *sonifier) stop() {
	if !so.active {
		return
	}
	if so.node.Truthy() {
		so.node.Set("onaudioprocess", js.Null())
		so.node.Call("disconnect")
	}
	so.node, so.ctx = js.Undefined(), js.Undefined()
	so.fn.Release()
	so.active = false
	releaseAudioCtx("sonify")
}

// buildSonifyModule builds the Model Out module's knobs: a TRACE-rate knob
// with the generators' octave dial, a LVL knob, and a MAP selector ring whose
// "off" position is the power switch. The trace/lvl sliders + LEDs are
// registry-owned (adoptDescControl in Run); this only adds the knob layer.
func (so *sonifier) buildModule() {
	freq := dom.Doc.Call("getElementById", "sonify-freq")
	fstack := dom.Doc.Call("getElementById", "sonify-fstack")
	lvl := dom.Doc.Call("getElementById", "sonify-lvl")
	lstack := dom.Doc.Call("getElementById", "sonify-lstack")
	mp := dom.Doc.Call("getElementById", "sonify-map")
	mstack := dom.Doc.Call("getElementById", "sonify-mstack")
	if !freq.Truthy() || !fstack.Truthy() {
		return
	}
	fknob := makeKnob(freq, js.Undefined(), true, false, false)
	addOctaveDial(fknob)
	fstack.Call("appendChild", fknob)
	lstack.Call("appendChild", makeKnob(lvl, js.Undefined(), true, false, true))
	md := dom.Doc.Call("getElementById", "sonify-mode")
	mstk := stackKnobs(selk.makeSelectorKnob(mp), selk.makeSelectorKnob(md))
	addSelectorLabels(mstk, []string{"off", "CAM", "XY", "XZ", "YZ"}, mp)
	addSelectorLabels(mstk, []string{"FLOW", "SCAN"}, md)
	mstack.Call("appendChild", mstk)

	// Both rings through the registry: the cell had no reset button, and Reset
	// All put the mapping back to off while leaving the mode wherever it was.
	// No PermaKey on either — "sm" and "sn" already carry them in permaCtls.
	adoptDescControl(ControlDesc{
		ID: "sonify-map", Label: "map", IsSelect: true, SelectDef: "off",
		ResetID: "rst-sonify-map",
		SelectApply: func(v string) {
			so.mapping = v
			so.sync()
		},
	})
	adoptDescControl(ControlDesc{
		ID: "sonify-mode", Label: "mode", IsSelect: true, SelectDef: "flow",
		ResetID:     "rst-sonify-map",
		SelectApply: func(v string) { so.mode = v },
	})
	so.mapping = mp.Get("value").String()
	so.mode = md.Get("value").String()
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
	s, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'f', 2, 64), 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	return s
}
