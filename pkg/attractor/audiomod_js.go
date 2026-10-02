//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/colormap"
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/led"
	"strconv"
	"syscall/js"
)

// Per-parameter audio modulation. Any attractor-specific parameter can be
// individually routed from an audio feature (a channel: L / R / mono, and
// a band) with a signed level. Nothing is modulated by default — the user
// enables it per parameter via the controls that appear under each param
// when "Audio mod" is on.
//
// A routed parameter's value for the integration step is:
//
//	value = base + level * feature * (max - min)
//
// where base is the slider value (still authoritative — audio swings
// around it), level is the signed per-parameter depth, and feature is the
// smoothed 0..1 source. Keep level small for the "very low level" nudges
// these delicate attractors want; negative inverts. Applied only for the
// integration step, then restored, so the sliders never drift.

// paramMod is the per-parameter routing config, keyed by paramDef.ID. The
// source is a graphic EQ: a channel plus a band-weight curve over the
// spectrum (numEQBands). The modulation signal is the weighted-average energy
// of the selected bands on that channel.
type paramMod struct {
	channel string    // "" = off; "mono" | "L" | "R"
	bands   []float32 // band weights 0..1 (len numEQBands); nil/zero = off
	level   float32
}

// paramModulation is per-parameter audio modulation: which parameters follow
// which feature, and what was applied last frame.
type paramModulation struct {
	params map[string]paramMod

	// hold is the quantizer's memory: the whole-grid value each COUNT parameter
	// was last given, keyed by paramDef.ID. quantizeHeld needs a previous value to
	// be sticky about, and the only place that survives between frames is here.
	// Keys are parameter ids, so it is bounded by the number of parameters in the
	// build and never grows with time.
	hold        map[string]float32
	appliedPrev []appliedMod
	appliedCur  []appliedMod
}

var pmod = paramModulation{
	params: map[string]paramMod{},
	hold:   map[string]float32{},
}

// paramIsModulated reports whether the sound is currently driving a parameter,
// so that a generator reading its own knob can tell the value it is holding
// from the value somebody set.
//
// It exists for the audio embeddings' camera fit. Those modes frame the camera
// to a bound computed from GAIN, so the fit has to be redone when GAIN moves —
// but applyAudioModulation writes the modulated value straight into the
// parameter for the duration of the step, so a GAIN with a modulator on it
// "moves" on almost every frame. Re-fitting to that would put the camera back
// under the music, which is the one thing every comment in takens_js.go is
// there to prevent. A modulated gain therefore keeps the frame it had: the
// bound the viewer chose is the base value, and the base value is what the
// camera should be framed to whatever the sound does to it afterwards.
//
// The test is collectAudioModulation's own, so a parameter counts as modulated
// exactly when that function would act on it.
func (p *paramModulation) paramIsModulated(id string) bool {
	if !audioMod {
		return false
	}
	m, ok := p.params[id]
	return ok && m.channel != "" && m.level != 0
}

// modChannels are the modulation sources, the Mod matrix's columns: the key
// its column is marked with, the name a route stores (paramMod.channel), and
// the group it is in. What each is, is the manual's (routing.md,
// mod-src=key). A route with no source is off, and has no column.
var modChannels = []struct{ key, name, group string }{
	{"st", "mono", "rack"},
	{"L", "L", "rack"},
	{"R", "R", "rack"},
	// The Mixer's two sends to modulation, which nothing else reads.
	{"A", modSrcSendA, "send"},
	{"B", modSrcSendB, "send"},
	// The model itself, closing the loop: the attractor's own current
	// output driving its own constants. A different system from the one
	// named on the dial, and deliberately so — see modelmod.go.
	{"x", modSrcModelX, "head"},
	{"y", modSrcModelY, "head"},
	{"z", modSrcModelZ, "head"},
	{"r", modSrcModelR, "head"},
}

type savedParam struct {
	p *float32
	v float32
}

// appliedMod is one parameter and the value modulation actually gave it this
// frame. pmod.appliedPrev/pmod.appliedCur hold consecutive frames' worth, in
// parameter order, so a frame can ask whether anything CHANGED rather than
// whether anything was modulated — see applyAudioModulation.
type appliedMod struct {
	id string
	v  float32
}

// applyAudioModulation overrides each routed parameter of the current
// attractor for this integration step and returns the saved originals.
func applyAudioModulation(mode string) []savedParam {
	saved := pmod.collectAudioModulation(mode)
	// Attractors regenerate every frame; a static model (sphere/torus/globe/…)
	// only rebuilds when marked dirty, so a rebuild has to be forced whenever
	// this frame's values differ from the mesh already on the GPU.
	//
	// The test used to be "was anything modulated", which for a continuous
	// parameter asks the same thing — a float driven by audio has a new value
	// every frame. For a COUNT it emphatically does not: the whole point of
	// quantizing is that most frames produce the same integer, and a mesh
	// rebuild per frame is the one thing this feature must not reintroduce.
	// staticGeomCached's own comment records what that costs — 45% of all
	// allocation in globe.generate and a 66-100ms collector pause every 400ms,
	// the stutter that could be SEEN — and dirtying the flag unconditionally
	// while a count knob was routed would hand all of it straight back.
	// Simulated over ten seconds of steady tone driving sphere latitude, the
	// number of rebuilds goes 600 (one per frame) → 127 (quantized) → 2
	// (quantized, with the deadband).
	//
	// Comparing values rather than counting them also gets two cases right that
	// the old test got wrong. Modulation switched OFF used to leave the last
	// modulated mesh on the GPU with nothing to mark it stale, because the
	// frame that stopped modulating also stopped setting the flag; now the
	// disappearance of a value IS a change and rebuilds once. And a float
	// parameter under a genuinely constant feature no longer rebuilds an
	// identical mesh sixty times a second.
	if pmod.applyChanged() && !isAttractorMode(mode) {
		gpu.staticDirty = true
	}
	return saved
}

// collectAudioModulation is applyAudioModulation's parameter loop, split out so
// the rebuild decision above runs on every path — including the two early
// returns, where the interesting case is precisely that nothing was applied
// this frame although something was applied last frame.
func (p *paramModulation) collectAudioModulation(mode string) []savedParam {
	p.appliedCur = p.appliedCur[:0]
	if !audioMod {
		return nil
	}
	params := modParams(mode)
	if len(params) == 0 {
		return nil
	}
	var saved []savedParam
	for _, pd := range params {
		m, ok := p.params[pd.ID]
		if !ok || m.channel == "" || m.level == 0 {
			continue
		}
		f := af.eqModValue(m.channel, m.bands)
		base := *pd.Value
		v := clampF(base+m.level*f*(pd.Max-pd.Min), pd.Min, pd.Max)
		// A step with no decimals is a COUNT — lines, subdivisions, samples of
		// delay, a position on a labeled dial. It is modulated exactly like a
		// continuous parameter and then read off the parameter's own grid, so
		// the sound moves it between 8 and 14 without ever asking for half a
		// line. Continuous parameters are deliberately left alone: quantizing
		// lorenz-dt to its 0.001 step would coarsen modulation that works.
		//
		// Every integral-step parameter in the build today really is a count or
		// an index (see paramdefs_js.go and the spect/rec/takens/stereo sets) —
		// there is no continuous quantity wearing a step of 1. If one is ever
		// added, the fix is to give it the finer step it always wanted rather
		// than an exception here, because the same coarse step is already
		// quantizing its knob, its wheel and its LED.
		if led.StepDecimals(pd.Step) == 0 {
			held, has := p.hold[pd.ID]
			v = quantizeHeld(v, held, has, pd.Min, pd.Max, pd.Step)
			p.hold[pd.ID] = v
		}
		saved = append(saved, savedParam{pd.Value, base})
		*pd.Value = v
		p.appliedCur = append(p.appliedCur, appliedMod{pd.ID, v})
	}
	return saved
}

// applyChanged reports whether this frame's applied modulation differs from
// the previous frame's, and takes this frame's as the new baseline.
//
// Positional comparison is enough because both lists are built by walking
// attractorParams[mode] in order, so equal length plus equal entries means the
// same parameters carrying the same values. A mode change shuffles the ids and
// reads as a change, which is correct and in any case redundant — changing mode
// dirties the geometry by itself.
//
// The two slices are reused rather than reallocated. This runs once a frame for
// the life of the tab, and the js/wasm builds (TinyGo especially) pay for
// garbage in collector pauses, which is the very cost this function exists to
// avoid.
func (p *paramModulation) applyChanged() bool {
	if len(p.appliedCur) != len(p.appliedPrev) {
		p.appliedPrev = append(p.appliedPrev[:0], p.appliedCur...)
		return true
	}
	for i, a := range p.appliedCur {
		if p.appliedPrev[i] != a {
			p.appliedPrev = append(p.appliedPrev[:0], p.appliedCur...)
			return true
		}
	}
	return false
}

// restoreAudioModulation restores base parameter values after the step.
//
// Counts go back through exactly this path and for exactly the reason floats
// do: the slider is still the authoritative value and the audio only borrows
// the parameter for one integration step. A quantized value left written over
// the base would ratchet the slider onto the grid and leave it there — visible
// as a knob that drifts on its own whenever the music plays.
func restoreAudioModulation(saved []savedParam) {
	for _, s := range saved {
		*s.p = s.v
	}
}

// viewModTarget is a non-parameter float control (camera/motion knob) that
// can also be audio-modulated. ptr is the cached var the render loop reads.
type viewModTarget struct {
	id, label string
	ptr       *float32
	min, max  float32
	anchor    string // slider id whose View&Motion cell the mod row sits under
}

var viewModTargets = []viewModTarget{
	{"view-zoom", "zoom", &view.ctl.zoom, -95, 95, "camera-zoom"},
	{"view-panx", "pan X", &view.ctl.panX, -8, 8, "pan-x"},
	{"view-pany", "pan Y", &view.ctl.panY, -8, 8, "pan-y"},
	{"view-spinx", "spin X", &view.ctl.spinX, -1, 1, "rotation-controls-x"},
	{"view-spiny", "spin Y", &view.ctl.spinY, -1, 1, "rotation-controls-y"},
	{"view-spinz", "spin Z", &view.ctl.spinZ, -1, 1, "rotation-controls-z"},
	{"view-rfreq", "period", &style.gradientFreq, 0.05, 20, "rainbow-freq"},
	{"view-trail", "trail", &style.trailModFrac, 0.02, 1, "trail-slider"},
	// APPENDED, not slotted in beside the period it belongs with. midi_js.go
	// hands CC 21+i to viewModTargets[i], so the position in this slice is a
	// controller's knob assignment: inserting in the middle would silently
	// re-map somebody's hardware, and a MIDI mapping breaking is a thing that
	// gets noticed hours later and blamed on the controller. The panel groups
	// the mod cards by their own list (panelbuild_js.go), so the visible order
	// is unaffected — only the patchbay's column order follows this one.
	{"view-pshift", "shift", &gradientShift, -1, 1, "palette-shift"},
}

// updateViewModRows injects an audio-mod routing row directly beneath each
// view knob (spin/pos/zoom) in the View & Motion section when Audio mod is on,
// so the modulation controls sit next to the control they drive. Removes them
// otherwise. Called from buildParamPanel (which runs on every relevant state
// change: audio-mod toggle, mode change, reset, permalink restore).
// updateViewModRows is retained as a no-op cleanup: view/camera/motion mod
// controls now live as cards in the dedicated Modulation module (buildParamPanel)
// rather than injected under each view knob, so nothing is mixed into other
// modules. Any stray legacy rows are removed defensively.
func updateViewModRows() {
	ex := dom.Doc.Call("querySelectorAll", ".viewmod-row")
	for i := ex.Get("length").Int() - 1; i >= 0; i-- {
		ex.Index(i).Call("remove")
	}
}

// applyViewModulation modulates the camera/motion cached vars (zoom, pan, spin
// rates, line width) in place before the render loop consumes them; the caller
// restores them afterward (restoreAudioModulation). No-op in the audio display
// modes, whose camera is managed specially.
func (p *paramModulation) applyViewModulation() []savedParam {
	if !audioMod {
		return nil
	}
	if isSpectroSurface(run.selectedMode) || isAudioMode(run.selectedMode) {
		return nil
	}
	var saved []savedParam
	for _, vt := range viewModTargets {
		m, ok := p.params[vt.id]
		if !ok || m.channel == "" || m.level == 0 {
			continue
		}
		f := af.eqModValue(m.channel, m.bands)
		base := *vt.ptr
		saved = append(saved, savedParam{vt.ptr, base})
		if vt.id == "view-trail" {
			// Trail can't grow past its buffer, so the signal contracts it from
			// full: level>0 shortens with energy (±level still inverts).
			*vt.ptr = clampF(1-m.level*f, vt.min, vt.max)
		} else if vt.id == "view-pshift" {
			// WRAPPED, not clamped, and it is the only target here for which
			// that is even meaningful: the palette window is periodic in the
			// shift — ±1 is one whole period of the shader's fold — so there is
			// no end for the value to run into. Clamping it would reintroduce
			// exactly the failure the reflection exists to avoid, the sweep
			// jamming against a limit for the loud half of the music while
			// every fragment holds one color. The wrap is invisible because the
			// two periods are the same 2 by construction (pkg/colormap).
			*vt.ptr = colormap.WrapShift(base + m.level*f*(vt.max-vt.min))
		} else {
			*vt.ptr = clampF(base+m.level*f*(vt.max-vt.min), vt.min, vt.max)
		}
	}
	return saved
}

// makeEQStrip builds the graphic-EQ band-picker for parameter id: numEQBands
// draggable columns (low→high) whose heights are the band weights in
// pmod.params[id].bands. Drag across to paint the curve.
func makeEQStrip(id string) js.Value {
	m := pmod.params[id]
	if m.bands == nil {
		m.bands = make([]float32, numEQBands)
		pmod.params[id] = m
	}
	wrap := dom.Doc.Call("createElement", "div")
	wrap.Set("className", "eqstrip")
	wrap.Call("setAttribute", "data-no-drag", "")
	wrap.Set("title", docf("mod-eq.strip", "control", id))

	fills := make([]js.Value, numEQBands)
	for i := range numEQBands {
		bar := dom.Doc.Call("createElement", "div")
		bar.Set("className", "eqbar")
		fill := dom.Doc.Call("createElement", "div")
		fill.Set("className", "eqfill")
		bar.Call("appendChild", fill)
		wrap.Call("appendChild", bar)
		fills[i] = fill
	}
	render := func() {
		mm := pmod.params[id]
		for i := 0; i < numEQBands && i < len(mm.bands); i++ {
			fills[i].Get("style").Set("height", strconv.FormatFloat(float64(mm.bands[i]*100), 'f', 0, 64)+"%")
		}
	}
	render()

	apply := func(e js.Value) {
		r := wrap.Call("getBoundingClientRect")
		w := r.Get("width").Float()
		h := r.Get("height").Float()
		if w <= 0 || h <= 0 {
			return
		}
		idx := int((e.Get("clientX").Float() - r.Get("left").Float()) / w * float64(numEQBands))
		if idx < 0 {
			idx = 0
		} else if idx >= numEQBands {
			idx = numEQBands - 1
		}
		v := 1 - (e.Get("clientY").Float()-r.Get("top").Float())/h
		if v < 0.06 { // snap the bottom of the strip to 0 so a band clears easily
			v = 0
		} else if v > 1 {
			v = 1
		}
		mm := pmod.params[id]
		if mm.bands == nil {
			mm.bands = make([]float32, numEQBands)
		}
		mm.bands[idx] = float32(v)
		pmod.params[id] = mm
		render()
	}
	dragging := false
	dom.On(wrap, "pointerdown", func(this js.Value, a []js.Value) any {
		a[0].Call("preventDefault")
		a[0].Call("stopPropagation")
		dragging = true
		apply(a[0])
		return nil
	})
	dom.On(wrap, "pointermove", func(this js.Value, a []js.Value) any {
		if dragging {
			apply(a[0])
		}
		return nil
	})
	stop := dom.FuncOf(func(this js.Value, a []js.Value) any {
		if dragging {
			dragging = false
			perma.syncPermalinkNow()
		}
		return nil
	})
	wrap.Call("addEventListener", "pointerup", stop)
	wrap.Call("addEventListener", "pointerleave", stop)
	return wrap
}

// modParams is the parameters modulation can route on mode: its own, or for
// Custom, the constants its equations name (customEquation.modDefs). Custom's
// are not in attractorParams, whose readers would put them in a link a second
// time beside Custom's own keys.
func modParams(mode string) []paramDef {
	if mode == "custom" {
		return custom.modDefs
	}
	return attractorParams[mode]
}

// The routes' names for the Mixer's sends to modulation, MOD A and MOD B.
const (
	modSrcSendA = "modA"
	modSrcSendB = "modB"
)

// modReadsSends reports whether any route reads MOD A or B: the only reason
// to analyze them.
func modReadsSends() bool {
	for _, m := range pmod.params {
		if m.level != 0 && (m.channel == modSrcSendA || m.channel == modSrcSendB) {
			return true
		}
	}
	return false
}
