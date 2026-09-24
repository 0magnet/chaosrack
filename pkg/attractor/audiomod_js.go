//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/colormap"
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/led"
	"strconv"
	"strings"
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

var paramMods = map[string]paramMod{}

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
func paramIsModulated(id string) bool {
	if !audioMod {
		return false
	}
	m, ok := paramMods[id]
	return ok && m.channel != "" && m.level != 0
}

// modChannels is the channel selector offered per parameter.
var modChannels = []struct{ label, name, desc string }{
	{"— off —", "", "off — this parameter is not modulated by audio"},
	{"stereo", "mono", "stereo — both channels summed drive this parameter"},
	{"left", "L", "left — the left channel alone drives this parameter"},
	{"right", "R", "right — the right channel alone drives this parameter"},
	// The model itself, closing the loop: the attractor's own current
	// output driving its own constants. A different system from the one
	// named on the dial, and deliberately so — see modelmod.go.
	{"model x", modSrcModelX, "model x — the attractor's own current x drives this parameter, so the system feeds back into itself. Small depths drift the shape; large ones make a different system, which is the point. Smoothed, because a loop that answers within a frame of its own output is an oscillator at the frame rate."},
	{"model y", modSrcModelY, "model y — as model x, on the y coordinate"},
	{"model z", modSrcModelZ, "model z — as model x, on the z coordinate. The classic one to try: a Lorenz whose rho follows its own z."},
	{"model r", modSrcModelR, "model r — the attractor's distance from the origin drives this parameter: a source that does not care which way the orbit went, only how far out it is"},
}

type savedParam struct {
	p *float32
	v float32
}

// modHold is the quantizer's memory: the whole-grid value each COUNT parameter
// was last given, keyed by paramDef.ID. quantizeHeld needs a previous value to
// be sticky about, and the only place that survives between frames is here.
// Keys are parameter ids, so it is bounded by the number of parameters in the
// build and never grows with time.
var modHold = map[string]float32{}

// appliedMod is one parameter and the value modulation actually gave it this
// frame. modAppliedPrev/modAppliedCur hold consecutive frames' worth, in
// parameter order, so a frame can ask whether anything CHANGED rather than
// whether anything was modulated — see applyAudioModulation.
type appliedMod struct {
	id string
	v  float32
}

var (
	modAppliedPrev []appliedMod
	modAppliedCur  []appliedMod
)

// applyAudioModulation overrides each routed parameter of the current
// attractor for this integration step and returns the saved originals.
func applyAudioModulation(mode string) []savedParam {
	saved := collectAudioModulation(mode)
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
	// allocation in generateGlobe and a 66-100ms collector pause every 400ms,
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
	if modApplyChanged() && !isAttractorMode(mode) {
		gpu.staticDirty = true
	}
	return saved
}

// collectAudioModulation is applyAudioModulation's parameter loop, split out so
// the rebuild decision above runs on every path — including the two early
// returns, where the interesting case is precisely that nothing was applied
// this frame although something was applied last frame.
func collectAudioModulation(mode string) []savedParam {
	modAppliedCur = modAppliedCur[:0]
	if !audioMod {
		return nil
	}
	params, ok := attractorParams[mode]
	if !ok {
		return nil
	}
	var saved []savedParam
	for _, pd := range params {
		m, ok := paramMods[pd.ID]
		if !ok || m.channel == "" || m.level == 0 {
			continue
		}
		f := eqModValue(m.channel, m.bands)
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
			held, has := modHold[pd.ID]
			v = quantizeHeld(v, held, has, pd.Min, pd.Max, pd.Step)
			modHold[pd.ID] = v
		}
		saved = append(saved, savedParam{pd.Value, base})
		*pd.Value = v
		modAppliedCur = append(modAppliedCur, appliedMod{pd.ID, v})
	}
	return saved
}

// modApplyChanged reports whether this frame's applied modulation differs from
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
func modApplyChanged() bool {
	if len(modAppliedCur) != len(modAppliedPrev) {
		modAppliedPrev = append(modAppliedPrev[:0], modAppliedCur...)
		return true
	}
	for i, a := range modAppliedCur {
		if modAppliedPrev[i] != a {
			modAppliedPrev = append(modAppliedPrev[:0], modAppliedCur...)
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
	{"view-rfreq", "period", &gradientFreq, 0.05, 20, "rainbow-freq"},
	{"view-trail", "trail", &trailModFrac, 0.02, 1, "trail-slider"},
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
func applyViewModulation() []savedParam {
	if !audioMod {
		return nil
	}
	if isSpectroSurface(selectedMode) || isAudioMode(selectedMode) {
		return nil
	}
	var saved []savedParam
	for _, vt := range viewModTargets {
		m, ok := paramMods[vt.id]
		if !ok || m.channel == "" || m.level == 0 {
			continue
		}
		f := eqModValue(m.channel, m.bands)
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

// buildModUnit builds the compact "MOD / LVL" half of a parameter unit: the
// concentric channel(ring)+level(inner) knob with its rotary-switch labels and
// the level numeric, stacked vertically. Always present in a unit (so toggling
// Audio mod never reflows the panel — it just dims); state lives in
// paramMods[id]. The channel <select> + level <range> stay hidden in the DOM,
// driven by the knob.
func buildModUnit(id, label string) js.Value {
	cur := paramMods[id]
	sel := dom.Doc.Call("createElement", "select")
	sel.Set("title", "Audio channel driving "+label)
	sel.Set("style", "display:none;")
	for _, s := range modChannels {
		opt := dom.Doc.Call("createElement", "option")
		opt.Set("value", s.name)
		opt.Set("textContent", s.label)
		// The parameter's own name goes in, so a panel of fourteen mod cards
		// is fourteen different sentences rather than one repeated fourteen
		// times — each detent says what IT does to THIS parameter.
		opt.Set("title", strings.Replace(s.desc, "this parameter", label, 1))
		if s.name == cur.channel {
			opt.Set("selected", true)
		}
		sel.Call("appendChild", opt)
	}
	sel.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, args []js.Value) interface{} {
		m := paramMods[id]
		m.channel = sel.Get("value").String()
		paramMods[id] = m
		syncPermalinkNow()
		return nil
	}))

	// Level range ±4 (not ±1): the modulation offset is level·f·(paramMax−min),
	// and the audio value f averages well below 1 for real music, so ±1 could
	// only overdrive on rare peaks. ±4 gives enough gain to clearly (over)drive
	// a parameter — dt into chaos — around level ~1.5, with headroom to spare.
	lvl := dom.Doc.Call("createElement", "input")
	lvl.Set("type", "range")
	lvl.Set("min", "-4")
	lvl.Set("max", "4")
	lvl.Set("step", "0.01")
	lvl.Set("title", "Audio-mod depth control for "+label+" — how strongly the selected channel drives it (hidden range behind the inner knob)")
	lvl.Set("value", strconv.FormatFloat(float64(cur.level), 'g', -1, 32))
	lvl.Set("style", "display:none;")
	lvlNum := dom.Doc.Call("createElement", "input")
	lvlNum.Set("type", "text")
	lvlNum.Set("inputmode", "decimal")
	// No min/max/step: see buildParamUnit. They do nothing on a text input.
	lvlNum.Set("value", led.Format(float64(cur.level), 1, 2, true))
	lvlNum.Set("title", "Mod depth for "+label+" (± inverts, 0 = off; ~1.5+ overdrives)")
	lvlNum.Set("className", "numin u-modval")
	lvl.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if v, err := strconv.ParseFloat(lvl.Get("value").String(), 32); err == nil {
			if v > -0.005 && v < 0.005 {
				v = 0
			}
			m := paramMods[id]
			m.level = float32(v)
			paramMods[id] = m
			lvlNum.Set("value", led.Format(v, 1, 2, true))
		}
		return nil
	}))
	lvlNum.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if v, err := strconv.ParseFloat(lvlNum.Get("value").String(), 32); err == nil {
			m := paramMods[id]
			m.level = float32(v)
			paramMods[id] = m
			lvl.Set("value", strconv.FormatFloat(v, 'g', -1, 64))
		}
		return nil
	}))
	chStack := stackKnobs(makeSelectorKnob(sel), makeKnob(lvl, lvlNum, true, false, false))
	addSelectorLabels(chStack, []string{"off", "st", "L", "R"}, sel)

	mod := dom.Doc.Call("createElement", "div")
	mod.Set("className", "punit-mod")
	lbl := dom.Doc.Call("createElement", "span")
	lbl.Set("className", "u-modlbl")
	lbl.Set("textContent", "MOD / LVL")
	mod.Call("appendChild", lbl)
	mod.Call("appendChild", chStack)
	mod.Call("appendChild", sel) // hidden, driven by the ring
	mod.Call("appendChild", lvl) // hidden, driven by the inner knob
	mod.Call("appendChild", lvlNum)
	return mod
}

// makeEQStrip builds the graphic-EQ band-picker for parameter id: numEQBands
// draggable columns (low→high) whose heights are the band weights in
// paramMods[id].bands. Drag across to paint the curve.
func makeEQStrip(id string) js.Value {
	m := paramMods[id]
	if m.bands == nil {
		m.bands = make([]float32, numEQBands)
		paramMods[id] = m
	}
	wrap := dom.Doc.Call("createElement", "div")
	wrap.Set("className", "eqstrip")
	wrap.Call("setAttribute", "data-no-drag", "")
	wrap.Set("title", "EQ for "+id+" — drag to pick which frequency bands (low→high) drive the "+id+" parameter")

	fills := make([]js.Value, numEQBands)
	for i := 0; i < numEQBands; i++ {
		bar := dom.Doc.Call("createElement", "div")
		bar.Set("className", "eqbar")
		fill := dom.Doc.Call("createElement", "div")
		fill.Set("className", "eqfill")
		bar.Call("appendChild", fill)
		wrap.Call("appendChild", bar)
		fills[i] = fill
	}
	render := func() {
		mm := paramMods[id]
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
		mm := paramMods[id]
		if mm.bands == nil {
			mm.bands = make([]float32, numEQBands)
		}
		mm.bands[idx] = float32(v)
		paramMods[id] = mm
		render()
	}
	dragging := false
	wrap.Call("addEventListener", "pointerdown", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		a[0].Call("preventDefault")
		a[0].Call("stopPropagation")
		dragging = true
		apply(a[0])
		return nil
	}))
	wrap.Call("addEventListener", "pointermove", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		if dragging {
			apply(a[0])
		}
		return nil
	}))
	stop := dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		if dragging {
			dragging = false
			syncPermalinkNow()
		}
		return nil
	})
	wrap.Call("addEventListener", "pointerup", stop)
	wrap.Call("addEventListener", "pointerleave", stop)
	return wrap
}
