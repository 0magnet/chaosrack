//go:build js && wasm

package attractor

import (
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

var paramMods = map[string]paramMod{}

// modChannels is the channel selector offered per parameter.
var modChannels = []struct{ label, name string }{
	{"— off —", ""}, {"stereo", "mono"}, {"left", "L"}, {"right", "R"},
}

type savedParam struct {
	p *float32
	v float32
}

// applyAudioModulation overrides each routed parameter of the current
// attractor for this integration step and returns the saved originals.
func applyAudioModulation(mode string) []savedParam {
	if !audioMod {
		return nil
	}
	params, ok := attractorParams[mode]
	if !ok {
		return nil
	}
	var saved []savedParam
	modulated := false
	for _, pd := range params {
		// Integer settings (line counts, subdivisions) aren't meaningfully
		// modulatable — only float params.
		if decimalsForStep(pd.Step) == 0 {
			continue
		}
		m, ok := paramMods[pd.ID]
		if !ok || m.channel == "" || m.level == 0 {
			continue
		}
		f := eqModValue(m.channel, m.bands)
		base := *pd.Value
		saved = append(saved, savedParam{pd.Value, base})
		*pd.Value = clampF(base+m.level*f*(pd.Max-pd.Min), pd.Min, pd.Max)
		modulated = true
	}
	// Attractors regenerate every frame; a static model (sphere/torus/…) only
	// rebuilds when marked dirty, so force a rebuild this frame with the
	// modulated value.
	if modulated && !isAttractorMode(mode) {
		staticGeomDirty = true
	}
	return saved
}

// restoreAudioModulation restores base parameter values after the step.
func restoreAudioModulation(saved []savedParam) {
	for _, s := range saved {
		*s.p = s.v
	}
}

func clampF(x, lo, hi float32) float32 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
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
	{"view-zoom", "zoom", &cachedZoom, -95, 95, "camera-zoom"},
	{"view-panx", "pan X", &cachedPanX, -8, 8, "pan-x"},
	{"view-pany", "pan Y", &cachedPanY, -8, 8, "pan-y"},
	{"view-spinx", "spin X", &cachedRotX, -1, 1, "rotation-controls-x"},
	{"view-spiny", "spin Y", &cachedRotY, -1, 1, "rotation-controls-y"},
	{"view-spinz", "spin Z", &cachedRotZ, -1, 1, "rotation-controls-z"},
	{"view-rfreq", "rainbow", &gradientFreq, 0.05, 20, "rainbow-freq"},
	{"view-trail", "trail", &trailModFrac, 0.02, 1, "trail-slider"},
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
	ex := doc.Call("querySelectorAll", ".viewmod-row")
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
	if selectedMode == "spectrogram" || selectedMode == "fvf" || isAudioMode(selectedMode) {
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
		} else {
			*vt.ptr = clampF(base+m.level*f*(vt.max-vt.min), vt.min, vt.max)
		}
	}
	return saved
}

// testToneNodes holds the live Web Audio graph for the built-in test signal
// generator ([ctx, osc, tremoloLFO, sweepLFO]); nil when off.
var testToneNodes []js.Value

// setTestTone plays (or stops) a built-in test signal out the speakers: a
// harmonic-rich sawtooth whose pitch slowly sweeps (moving the spectrum across
// bass/mid/treble) with a tremolo on the amplitude (so amp/beat pulse). The
// server's monitor capture picks it back up and streams it over the websocket,
// exactly like external audio — a self-contained way to exercise the attractor
// modulation. The toggle click is the user gesture that lets the AudioContext
// start.
func setTestTone(on bool) {
	if on {
		if len(testToneNodes) > 0 {
			return
		}
		ctor := js.Global().Get("AudioContext")
		if !ctor.Truthy() {
			ctor = js.Global().Get("webkitAudioContext")
		}
		if !ctor.Truthy() {
			return
		}
		ctx := ctor.New()
		ctx.Call("resume")
		osc := ctx.Call("createOscillator")
		osc.Set("type", "sawtooth")
		osc.Get("frequency").Set("value", 300)
		gain := ctx.Call("createGain")
		gain.Get("gain").Set("value", 0.1)
		// tremolo: modulate the output gain so amplitude/beat features pulse
		trem := ctx.Call("createOscillator")
		trem.Set("type", "sine")
		trem.Get("frequency").Set("value", 2.3)
		tremGain := ctx.Call("createGain")
		tremGain.Get("gain").Set("value", 0.07)
		trem.Call("connect", tremGain)
		tremGain.Call("connect", gain.Get("gain"))
		// slow pitch sweep so the spectral centroid / bands move over time
		sweep := ctx.Call("createOscillator")
		sweep.Set("type", "sine")
		sweep.Get("frequency").Set("value", 0.13)
		sweepGain := ctx.Call("createGain")
		sweepGain.Get("gain").Set("value", 240) // 300 ± 240 Hz → ~60..540 Hz
		sweep.Call("connect", sweepGain)
		sweepGain.Call("connect", osc.Get("frequency"))
		osc.Call("connect", gain)
		gain.Call("connect", ctx.Get("destination"))
		osc.Call("start")
		trem.Call("start")
		sweep.Call("start")
		testToneNodes = []js.Value{ctx, osc, trem, sweep}
	} else {
		if len(testToneNodes) > 0 {
			testToneNodes[0].Call("close") // closing the context stops the graph
		}
		testToneNodes = nil
	}
}

// buildModUnit builds the compact "MOD / LVL" half of a parameter unit: the
// concentric channel(ring)+level(inner) knob with its rotary-switch labels and
// the level numeric, stacked vertically. Always present in a unit (so toggling
// Audio mod never reflows the panel — it just dims); state lives in
// paramMods[id]. The channel <select> + level <range> stay hidden in the DOM,
// driven by the knob.
func buildModUnit(id, label string) js.Value {
	cur := paramMods[id]
	sel := doc.Call("createElement", "select")
	sel.Set("title", "Audio channel driving "+label)
	sel.Set("style", "display:none;")
	for _, s := range modChannels {
		opt := doc.Call("createElement", "option")
		opt.Set("value", s.name)
		opt.Set("textContent", s.label)
		if s.name == cur.channel {
			opt.Set("selected", true)
		}
		sel.Call("appendChild", opt)
	}
	sel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
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
	lvl := doc.Call("createElement", "input")
	lvl.Set("type", "range")
	lvl.Set("min", "-4")
	lvl.Set("max", "4")
	lvl.Set("step", "0.01")
	lvl.Set("title", "Audio-mod depth for "+label+" — how strongly the selected channel drives the "+label+" parameter")
	lvl.Set("value", strconv.FormatFloat(float64(cur.level), 'g', -1, 32))
	lvl.Set("style", "display:none;")
	lvlNum := doc.Call("createElement", "input")
	lvlNum.Set("type", "text")
	lvlNum.Set("inputmode", "decimal")
	lvlNum.Set("min", "-4")
	lvlNum.Set("max", "4")
	lvlNum.Set("step", "0.01")
	lvlNum.Set("value", formatLED(float64(cur.level), 1, 2, true))
	lvlNum.Set("title", "Mod depth for "+label+" (± inverts, 0 = off; ~1.5+ overdrives)")
	lvlNum.Set("className", "numin u-modval")
	lvl.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if v, err := strconv.ParseFloat(lvl.Get("value").String(), 32); err == nil {
			if v > -0.005 && v < 0.005 {
				v = 0
			}
			m := paramMods[id]
			m.level = float32(v)
			paramMods[id] = m
			lvlNum.Set("value", formatLED(v, 1, 2, true))
		}
		return nil
	}))
	lvlNum.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if v, err := strconv.ParseFloat(lvlNum.Get("value").String(), 32); err == nil {
			m := paramMods[id]
			m.level = float32(v)
			paramMods[id] = m
			lvl.Set("value", strconv.FormatFloat(v, 'g', -1, 64))
		}
		return nil
	}))
	chStack := stackKnobs(makeSelectorKnob(sel), makeKnob(lvl, lvlNum, true, false, false))
	addSelectorLabels(chStack, []string{"off", "st", "L", "R"}, sel, 50)

	mod := doc.Call("createElement", "div")
	mod.Set("className", "punit-mod")
	lbl := doc.Call("createElement", "span")
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
	wrap := doc.Call("createElement", "div")
	wrap.Set("className", "eqstrip")
	wrap.Call("setAttribute", "data-no-drag", "")
	wrap.Set("title", "EQ for "+id+" — drag to pick which frequency bands (low→high) drive the "+id+" parameter")

	fills := make([]js.Value, numEQBands)
	for i := 0; i < numEQBands; i++ {
		bar := doc.Call("createElement", "div")
		bar.Set("className", "eqbar")
		fill := doc.Call("createElement", "div")
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
	wrap.Call("addEventListener", "pointerdown", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		a[0].Call("preventDefault")
		a[0].Call("stopPropagation")
		dragging = true
		apply(a[0])
		return nil
	}))
	wrap.Call("addEventListener", "pointermove", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if dragging {
			apply(a[0])
		}
		return nil
	}))
	stop := trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
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
