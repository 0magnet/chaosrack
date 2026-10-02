//go:build js && wasm

package attractor

import (
	"math"
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// Model Out — the model as a signal generator, which is what it is: an
// attractor is a system of equations whose state x, y and z moves through
// time, and those three ARE its signals, the outputs an analog computer
// solving the same equations would have on three jacks. Model Out runs the
// equations at audio rate and puts the three on the rack's channels as MODEL
// X, Y and Z (rackbus_js.go), to be patched, heard, drawn on any scope and
// analyzed like any other source.
//
// The integrator is the model's own: the vector field the renderer draws
// (dynamics.FlowFor4, 4D equation modes included), its parameters live, its
// step the model's dt. So what plays is what the model is doing — chaos
// chirps and whooshes, a periodic window locks into a steady tone, a turned
// parameter is heard as the bifurcation it is. SPD is how fast the model
// runs: integrator steps per sample, in semitones, 0 being one step a sample
// and every 12 up twice as fast; the pitch is the system's own and SPD moves
// it in exact intervals. LVL is the output level.
//
// A model with a trail but no vector field (a parametric curve) is played
// by scanning its drawn trail, once round at 440 traces a second at SPD 0 —
// its shape is its waveform. Geometry models (polyhedra, the torus…) write no
// trail and have no equations to run: silence.
//
// Each signal is centered and scaled to its own swing (fast attack, slow
// release), because an attractor's coordinates are arbitrary in offset and
// size: Lorenz's z lives around 25.

// modelOut is Model Out's two knobs, the bank positions modelOutParams
// declares.
var modelOut = struct{ speed, level float32 }{speed: -24, level: 60}

// modelOutParams are Model Out's controls as parameters, so the bank lays
// them out as the positions every model's controls are (rackcategory_js.go):
// shared by every model with a trajectory to play.
var modelOutParams = []paramDef{
	{"mo-spd", "spd", &modelOut.speed, -24, -48, 36, 1},
	{"mo-lvl", "lvl", &modelOut.level, 60, 0, 100, 1},
}

func init() {
	for _, p := range modelOutParams {
		quietParams[p.ID] = true // a sound, not the model: nothing to rebuild
	}
}

// modelSpeed is SPD as integrator steps per sample.
func modelSpeed() float64 { return math.Pow(2, float64(modelOut.speed)/12) }

// modelVoice is one running copy of the model as a signal: its integrator
// and how each output is scaled. Three play the one model, each with its own
// time, as the generators have their speaker copy and their analysis copy:
// the speakers', the rack signal's, and the scopes'.
type modelVoice struct {
	mode           string     // the model it was seeded for
	x, y, z, w     float64    // the state
	px, py, pz     float64    // the state a step ago, to interpolate between
	acc            float64    // fractional steps owed
	phase          float64    // scan: where along the trail, 0..1
	cen, span      [3]float64 // each output's center and half-swing
	scr            [3][]float32
	renderedAt, rn float64 // the frame and length a window was last made for
}

// render fills out[c][:n] with n samples of x, y and z at sample rate sr.
func (v *modelVoice) render(out *[3][]float32, n int, sr float64) {
	for c := range out {
		if cap(out[c]) < n {
			out[c] = make([]float32, n)
		}
		out[c] = out[c][:n]
	}
	mode := run.selectedMode
	if !isAttractorMode(mode) {
		for c := range out {
			clear(out[c])
		}
		return
	}
	sys, flow := dynamics.FlowFor4(mode)
	if flow {
		v.flow(out, n, sys, mode)
	} else if !v.scan(out, n, sr) {
		for c := range out {
			clear(out[c])
		}
		return
	}
	v.normalize(out, n)
}

// flow integrates the model's own equations, SPD steps a sample.
func (v *modelVoice) flow(out *[3][]float32, n int, sys dynamics.FlowSys4, mode string) {
	reseed := func() {
		ic := dynamics.InitCondFor(mode)
		v.x, v.y, v.z = float64(ic[0]), float64(ic[1]), float64(ic[2])
		if v.x == 0 && v.y == 0 && v.z == 0 {
			v.x = 0.1 // don't strand at a fixed point
		}
		v.w = sys.W()
		v.px, v.py, v.pz = v.x, v.y, v.z
		v.acc = 0
	}
	if v.mode != mode {
		v.mode = mode
		reseed()
	}
	dt := sys.Dt()
	rate := modelSpeed()
	const lim = 1e5
	for i := range n {
		v.acc += rate
		for v.acc >= 1 {
			v.acc--
			v.px, v.py, v.pz = v.x, v.y, v.z
			dx, dy, dz, dw := sys.F(v.x, v.y, v.z, v.w)
			v.x += dt * dx
			v.y += dt * dy
			v.z += dt * dz
			v.w += dt * dw
			if !(math.Abs(v.x) < lim && math.Abs(v.y) < lim && math.Abs(v.z) < lim && math.Abs(v.w) < lim) {
				reseed()
			}
		}
		t := v.acc // between the previous state and this one
		out[0][i] = float32(v.px + (v.x-v.px)*t)
		out[1][i] = float32(v.py + (v.y-v.py)*t)
		out[2][i] = float32(v.pz + (v.z-v.pz)*t)
	}
}

// scan plays the drawn trail as one period of a waveform, for a model with
// no equations to run. False when there is no trail.
func (v *modelVoice) scan(out *[3][]float32, n int, sr float64) bool {
	steps := sim.steps
	if steps < 2 || len(sim.vertBuf) < steps*4 || sr <= 0 {
		return false
	}
	inc := 440 * modelSpeed() / sr
	for i := range n {
		v.phase += inc
		v.phase -= math.Floor(v.phase)
		f := v.phase * float64(steps-1)
		j := int(f)
		t := float32(f - float64(j))
		a, b := j*4, min(j+1, steps-1)*4
		for c := range 3 {
			out[c][i] = sim.vertBuf[a+c]*(1-t) + sim.vertBuf[b+c]*t
		}
	}
	return true
}

// normalize centers each output on its swing and scales it to LVL, the
// swing tracked fast when it grows and slowly when it shrinks, so a
// suddenly larger orbit does not clip and a quiet one is not pumped.
func (v *modelVoice) normalize(out *[3][]float32, n int) {
	g := float64(modelOut.level) / 100
	for c := range out {
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, s := range out[c][:n] {
			lo, hi = math.Min(lo, float64(s)), math.Max(hi, float64(s))
		}
		if hi > lo {
			mid, half := (hi+lo)/2, (hi-lo)/2
			v.cen[c] += (mid - v.cen[c]) * 0.05
			if half > v.span[c] {
				v.span[c] = half
			} else {
				v.span[c] += (half - v.span[c]) * 0.01
			}
		}
		span := math.Max(v.span[c], 1e-6)
		for i, s := range out[c][:n] {
			out[c][i] = float32(math.Max(-1, math.Min(1, (float64(s)-v.cen[c])*g/span)))
		}
	}
}

// The three copies of the model: the speakers', the rack signal's, and the
// scopes'.
var modelHeard, modelRack, modelShown modelVoice

// busModel is the bus's model (audiosrc.Bus.Model): the next stretch of the
// rack signal's copy, or for a display the scopes' copy, run on by as much as
// it is asked for, once a frame however many scopes look.
func busModel(dst *[3][]float32, n int, advance bool) {
	sr := float64(aud.rackBus().SampleRate())
	if advance {
		modelRack.render(dst, n, sr)
		return
	}
	v := &modelShown
	if v.renderedAt != frameNowMs || v.rn != float64(n) {
		v.renderedAt, v.rn = frameNowMs, float64(n)
		v.render(&v.scr, n, sr)
	}
	for c := range dst {
		copy(dst[c], v.scr[c])
	}
}

// sonifier is Model Out's speaker path: a ScriptProcessor running the
// speakers' copy of the model, its three outputs into the Mixer's MODEL X, Y
// and Z, which put them on the speakers their pins say.
type sonifier struct {
	node   js.Value
	split  js.Value
	fn     js.Func
	active bool
	scr    [3][]float32
}

var son sonifier

// modelOutKeysMix are the Mixer's columns for the model's x, y and z.
var modelOutKeysMix = [3]string{"mx", "my", "mz"}

// modelIsHeard reports whether any of the model's outputs reaches a speaker:
// pinned to one on the Mixer, on a model that has a signal.
func modelIsHeard() bool {
	if !isAttractorMode(run.selectedMode) {
		return false
	}
	for _, k := range modelOutKeysMix {
		if mixOnSpeakers(mixSrcIndex(k)) {
			return true
		}
	}
	return false
}

// sync starts the speaker path while the model is heard and stops it when
// it is not. Called whenever a pin or the model changes.
func (so *sonifier) sync() {
	if modelIsHeard() {
		so.start()
	} else {
		so.stop()
	}
}

// process is the ScriptProcessor callback (in Go, on the main thread, so
// the trail and the parameters need no synchronization).
func (so *sonifier) process(_ js.Value, args []js.Value) any {
	out := args[0].Get("outputBuffer")
	n := out.Get("length").Int()
	modelHeard.render(&so.scr, n, out.Get("sampleRate").Float())
	for c := range so.scr {
		sonifyWrite(out.Call("getChannelData", c), so.scr[c])
	}
	return nil
}

// sonifyWrite copies a []float32 into a WebAudio channel-data Float32Array
// without allocating intermediate typed arrays.
func sonifyWrite(dst js.Value, src []float32) {
	u8 := js.Global().Get("Uint8Array").New(dst.Get("buffer"), dst.Get("byteOffset"), len(src)*4)
	js.CopyBytesToJS(u8, sliceToByteSlice(src))
}

func (so *sonifier) start() {
	if so.active {
		return
	}
	ctx := mixAcquire()
	if !ctx.Truthy() {
		return
	}
	so.node = ctx.Call("createScriptProcessor", 2048, 0, 3)
	so.split = ctx.Call("createChannelSplitter", 3)
	so.node.Call("connect", so.split)
	for c, k := range modelOutKeysMix {
		so.split.Call("connect", mixIn(k), c)
	}
	so.fn = dom.FuncOf(so.process)
	so.node.Set("onaudioprocess", so.fn)
	so.active = true
}

func (so *sonifier) stop() {
	if !so.active {
		return
	}
	so.node.Set("onaudioprocess", js.Null())
	so.node.Call("disconnect")
	so.split.Call("disconnect")
	so.node, so.split = js.Undefined(), js.Undefined()
	so.fn.Release()
	so.active = false
	mixRelease()
}

// sonifyFreqFromSlider maps a semitone slider (A0-anchored, the generators')
// to hertz.
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

// modelOutKeys are the link keys of modelOutParams, in order.
var modelOutKeys = []string{"ov", "ol"}

// applyModelOutKey sets the Model Out knob a link key names, and reports
// whether it named one.
func applyModelOutKey(key, val string) bool {
	for i, k := range modelOutKeys {
		if k != key {
			continue
		}
		if el := dom.Doc.Call("getElementById", modelOutParams[i].ID); el.Truthy() {
			el.Set("value", val)
			dom.Fire(el, "input")
		}
		return true
	}
	return false
}
