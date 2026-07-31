//go:build js && wasm

package attractor

import (
	"math"
	"strconv"
	"strings"
	"syscall/js"
)

// Frequency range for the generators: exactly 10 octaves anchored to notes,
// A0 (27.5 Hz, the lowest key on a piano) → A10 (28160 Hz). The knob's hidden
// slider is linear 0..genSemitones with ONE UNIT PER SEMITONE, and frequency
// maps exponentially, so equal turn = equal octaves (a logarithmic knob) and one
// coarse step / scroll click = one semitone. Because the low end is a real note
// in A440 tuning, every detent lands on a concert-pitch note — turning the knob
// steps through every note of the chromatic scale (12 per octave). 120 divides
// the range cleanly, so the slider reaches its max exactly (→ 28160, no short).

// addOctaveDial draws a tick ring around the (log) frequency knob with one tick
// per octave — so equal turn = equal octaves reads at a glance — plus the min /
// max Hz labeled at the sweep ends. Styled like the value dial (behind the
// knob, decorative). The octave ticks land at genFreqLo·2ⁿ, which are evenly
// spaced around the sweep because the knob is logarithmic.
func addOctaveDial(wrap js.Value) {
	dial := doc.Call("createElement", "div")
	dial.Set("className", "knob-dial value-dial")
	nOct := int(math.Log2(genFreqHi / genFreqLo)) // whole octaves in range
	for n := 0; n <= nOct; n++ {
		f := genFreqLo * math.Pow(2, float64(n))
		deg := -knobSweepDeg/2 + knobSweepDeg*knobFromFreq(f)/genSemitones
		l, tp := dialLabelPos(deg, 41)
		tk := doc.Call("createElement", "span")
		cls := "vdial-tick"
		if n%4 == 0 { // a longer tick every 4 octaves for a readable rhythm
			cls += " major"
		}
		tk.Set("className", cls)
		st := tk.Get("style")
		st.Set("left", l)
		st.Set("top", tp)
		st.Set("transform", "translate(-50%,-50%) rotate("+strconv.FormatFloat(deg, 'f', 1, 64)+"deg)")
		dial.Call("appendChild", tk)
	}
	// Note names at the sweep ends (both in the lower half, clear of the LED).
	// Every octave tick is an A, so the ends are A0 and A10; the LED shows exact
	// Hz.
	for _, e := range []struct {
		f float64
		s string
	}{{genFreqLo, "A0"}, {genFreqHi, "A10"}} {
		deg := -knobSweepDeg/2 + knobSweepDeg*knobFromFreq(e.f)/genSemitones
		l, tp := dialLabelPos(deg, 48)
		lab := doc.Call("createElement", "span")
		lab.Set("className", "knob-dial-lab")
		lab.Set("textContent", e.s)
		lab.Get("style").Set("left", l)
		lab.Get("style").Set("top", tp)
		dial.Call("appendChild", lab)
	}
	wrap.Call("insertBefore", dial, wrap.Get("firstChild"))
	wrap.Get("classList").Call("add", "has-dial")
}

// addPianoKeys builds a one-octave piano strip under the frequency knob and
// lights the key of the note the knob is currently on (pitch class, A=0 to match
// semitones above A0). Clicking a key sets that note in the octave currently
// shown. It's a live note indicator for the semitone-stepped log knob.
func addPianoKeys(freq js.Value) js.Value {
	// Key tooltips carry the owning oscillator so the three keyboards (and
	// their twelve notes each) stay globally unique.
	owner := "Gen"
	if id := freq.Get("id").String(); len(id) > 4 && id[:4] == "gen-" {
		owner = "Gen " + strings.ToUpper(id[4:5])
	}
	wrap := doc.Call("createElement", "span")
	wrap.Set("className", "gen-piano")
	wrap.Call("setAttribute", "data-no-drag", "")
	whites := doc.Call("createElement", "span")
	whites.Set("className", "pk-whites")
	blacks := doc.Call("createElement", "span")
	blacks.Set("className", "pk-blacks")

	// Pitch classes with A=0 (0=A,1=A#,2=B,3=C,…). White keys C..B left→right;
	// black keys sit on the white-key boundaries that have them.
	whitePC := []int{3, 5, 7, 8, 10, 0, 2}
	whiteName := []string{"C", "D", "E", "F", "G", "A", "B"}
	blackPC := []int{4, 6, 9, 11, 1}
	blackName := []string{"C#", "D#", "F#", "G#", "A#"}
	blackCenter := []float64{1.0 / 7, 2.0 / 7, 4.0 / 7, 5.0 / 7, 6.0 / 7} // white-boundary fractions
	const blackW = 9.0                                                    // % of strip width

	var keyEls []js.Value
	mk := func(pc int, name string, black bool, leftPct float64) js.Value {
		el := doc.Call("createElement", "span")
		if black {
			el.Set("className", "pk-key pk-black")
			el.Get("style").Set("left", strconv.FormatFloat(leftPct, 'f', 2, 64)+"%")
		} else {
			el.Set("className", "pk-key pk-white")
		}
		el.Call("setAttribute", "data-pc", strconv.Itoa(pc))
		el.Set("title", owner+" — set note "+name+" (in the octave currently shown)")
		el.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			// Set this note in the octave currently shown on the keyboard (the C..B
			// register the current note is in), snapping out any detune.
			cur := math.Round(fgFloat(freq))           // current note, semitones above A0
			off := ((pc-3)%12 + 12) % 12               // this key's offset above the register's C
			regC := 3 + 12*int(math.Floor((cur-3)/12)) // C of the current note's octave
			s := float64(regC + off)
			for s < 0 {
				s += 12
			}
			for s > genSemitones {
				s -= 12
			}
			freq.Set("value", strconv.FormatFloat(s, 'f', 0, 64))
			freq.Call("dispatchEvent", js.Global().Get("Event").New("input"))
			return nil
		}))
		keyEls = append(keyEls, el)
		return el
	}
	for i, pc := range whitePC {
		whites.Call("appendChild", mk(pc, whiteName[i], false, 0))
	}
	for i, pc := range blackPC {
		blacks.Call("appendChild", mk(pc, blackName[i], true, blackCenter[i]*100-blackW/2))
	}
	wrap.Call("appendChild", whites)
	wrap.Call("appendChild", blacks)

	highlight := func() {
		// The hidden slider is in semitones, so the fractional part IS the detune:
		// off ∈ [-0.5,+0.5] semitone = ∓50…+50 cents from the nearest note.
		sv := fgFloat(freq)
		nearest := math.Round(sv)
		off := sv - nearest
		pc := ((int(nearest) % 12) + 12) % 12
		want := strconv.Itoa(pc)
		// Tuning gradient: full green at dead-center; as you detune, red grows on
		// the side you're heading toward — right when sharp (off>0), left when flat
		// (off<0) — flipping at the ±50¢ boundary where the next note takes over.
		a := math.Min(1, math.Abs(off)*2) // 0 in-tune … 1 at the boundary
		var grad string
		if off >= 0 { // sharp → green left, red grows right
			grad = "linear-gradient(90deg,#16f06a 0%,#16f06a " + strconv.FormatFloat((1-a)*100, 'f', 0, 64) + "%,#ff2410 100%)"
		} else { // flat → red grows left, green right
			grad = "linear-gradient(90deg,#ff2410 0%,#16f06a " + strconv.FormatFloat(a*100, 'f', 0, 64) + "%,#16f06a 100%)"
		}
		for _, el := range keyEls {
			lit := el.Call("getAttribute", "data-pc").String() == want
			el.Get("classList").Call("toggle", "lit", lit)
			if lit {
				el.Get("style").Set("background", grad)
			} else {
				el.Get("style").Set("background", "")
			}
		}
	}
	highlight()
	freq.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		highlight()
		return nil
	}))
	return wrap
}

// The Generator module: three independent oscillators (X / Y / Z), each a
// concentric knob (outer ring = waveform, inner = frequency) with a Hz LED and
// a speaker-channel dropdown. It drives the shared FuncGen (scope / features)
// and, when "Listen" is on, a parallel Web Audio graph for the speakers.

var genOscIDs = []string{"gen-x", "gen-y", "gen-z"}
var genOscIdx = map[string]int{"gen-x": 0, "gen-y": 1, "gen-z": 2}

// waveSVG holds a tiny glyph per waveform (index matches the wave <select>:
// 0 sine, 1 triangle, 2 square, 3 saw), stroked in currentColor so CSS can dim
// the ring and light the active one.
var waveSVG = []string{
	`<svg viewBox="0 0 24 12"><path d="M1,6 C3.2,1 5.8,1 8,6 C10.2,11 12.8,11 15,6 C17.2,1 19.8,1 22,6" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
	`<svg viewBox="0 0 24 12"><path d="M2,10 L7,2 L12,10 L17,2 L22,10" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
	`<svg viewBox="0 0 24 12"><path d="M2,10 L2,3 L8.5,3 L8.5,10 L15,10 L15,3 L21.5,3 L21.5,10" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
	`<svg viewBox="0 0 24 12"><path d="M2,10 L8,3 L8,10 L14,3 L14,10 L20,3 L20,10" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
}

// addSelectorWaveDial adds an inner ring of waveform glyphs around a selector
// stack (one per option, at the knob's detent angles), lighting the active one
// and clickable to select — a graphic counterpart to addSelectorLabels for the
// waveform inner knob.
func addSelectorWaveDial(stack, sel js.Value, off float64) {
	n := len(waveSVG)
	dial := doc.Call("createElement", "div")
	dial.Set("className", "knob-dial")
	circle := doc.Call("createElement", "div")
	circle.Set("className", "knob-ring-circle")
	dia := strconv.FormatFloat(2*off, 'f', 1, 64) + "%"
	circle.Get("style").Set("width", dia)
	circle.Get("style").Set("height", dia)
	dial.Call("appendChild", circle)
	els := make([]js.Value, n)
	for i := 0; i < n; i++ {
		deg := -knobSweepDeg/2 + knobSweepDeg*float64(i)/float64(n-1)
		l, t := dialLabelPos(deg, off)
		ic := doc.Call("createElement", "span")
		ic.Set("className", "knob-dial-wave clickable")
		ic.Set("innerHTML", waveSVG[i])
		ic.Get("style").Set("left", l)
		ic.Get("style").Set("top", t)
		els[i] = ic
		idx := i
		ic.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			sel.Set("selectedIndex", idx)
			sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
			return nil
		}))
		dial.Call("appendChild", ic)
	}
	hi := func() {
		ci := sel.Get("selectedIndex").Int()
		for j, e := range els {
			e.Get("classList").Call("toggle", "wave-active", j == ci)
		}
	}
	sel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} { hi(); return nil }))
	hi()
	stack.Call("insertBefore", dial, stack.Get("firstChild"))
	stack.Get("classList").Call("add", "has-dial")
}

// waveTypeName maps our waveform index to the Web Audio OscillatorNode type.
func waveTypeName(w int) string {
	switch w {
	case 1:
		return "triangle"
	case 2:
		return "square"
	case 3:
		return "sawtooth"
	default:
		return "sine"
	}
}

// buildGeneratorModule wires the three per-oscillator modules. Each fills its
// 1-unit module with three cells: a frequency knob (log, equal turn per octave)
// with a Hz LED, a level knob with a % LED, and a dual concentric knob whose
// outer ring selects the speaker channel and whose inner knob selects the
// waveform (shown in the cell's readout). The channel ring's "off" position
// mutes that oscillator (no separate Listen).
func buildGeneratorModule() {
	for _, id := range genOscIDs {
		idx := genOscIdx[id]
		freq := doc.Call("getElementById", id+"-freq")
		freqLED := doc.Call("getElementById", id+"-led")
		wave := doc.Call("getElementById", id+"-wave")
		fstack := doc.Call("getElementById", id+"-fstack")
		lvl := doc.Call("getElementById", id+"-lvl")
		lvlLED := doc.Call("getElementById", id+"-lvl-led")
		out := doc.Call("getElementById", id+"-out")
		lstack := doc.Call("getElementById", id+"-lstack")
		ostack := doc.Call("getElementById", id+"-ostack")
		if !freq.Truthy() || !fstack.Truthy() {
			continue
		}

		// Freq cell: single log-frequency knob with an octave tick ring. The hidden
		// slider is in semitones (step 1 = one semitone), so scrolling the knob or
		// the LED steps note-by-note (12 per octave); dragging still sweeps
		// continuously and the fine disc trims sub-semitone. The LED is zero-padded
		// to the widest value (so 20480 never clips to "2048") with a fixed decimal,
		// like every other readout.
		fdig := intDigits(genFreqHi)
		freqLED.Set("value", formatLED(freqFromKnob(fgFloat(freq)), fdig, 1, false))
		sizeLEDField(freqLED, genFreqLo, genFreqHi, 1, false)
		fknob := makeKnob(freq, js.Undefined(), true, false, false)
		addOctaveDial(fknob)
		fstack.Call("appendChild", fknob)
		wheelNudge(freqLED, freq, 1, 0, genSemitones)
		// One-octave piano strip under the knob, lighting the current note.
		fstack.Get("parentNode").Get("parentNode").Call("appendChild", addPianoKeys(freq))

		// Level cell: single level knob with a 0..100 value dial; LED matches the
		// zero-padded, fixed-decimal style.
		lvlLED.Set("value", formatLED(fgFloat(lvl), intDigits(100), 1, false))
		sizeLEDField(lvlLED, 0, 100, 1, false)
		lstack.Call("appendChild", makeKnob(lvl, js.Undefined(), true, false, true))

		// Out cell: dual concentric knob — outer ring = speaker channel (labeled),
		// inner = waveform. Two dial rings: the sink labels (outer) and a ring of
		// waveform glyphs (inner) that lights the selected wave.
		ostk := stackKnobs(makeSelectorKnob(out), makeSelectorKnob(wave))
		addSelectorLabels(ostk, []string{"off", "L", "R", "L+R"}, out, 50)
		addSelectorWaveDial(ostk, wave, 38)
		ostack.Call("appendChild", ostk)

		freq.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			hz := freqFromKnob(fgFloat(freq))
			fg().SetFreq(idx, hz)
			freqLED.Set("value", formatLED(hz, fdig, 1, false))
			genAudioUpdate(idx)
			return nil
		}))
		freqLED.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			if hz, err := strconv.ParseFloat(freqLED.Get("value").String(), 64); err == nil && hz > 0 {
				freq.Set("value", strconv.FormatFloat(knobFromFreq(hz), 'f', 1, 64))
				freq.Call("dispatchEvent", js.Global().Get("Event").New("input"))
			}
			return nil
		}))
		lvl.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			v := fgFloat(lvl)
			fg().SetAmp(idx, v/100)
			lvlLED.Set("value", formatLED(v, intDigits(100), 1, false))
			genAudioUpdate(idx)
			return nil
		}))
		lvlLED.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			if v, err := strconv.ParseFloat(lvlLED.Get("value").String(), 64); err == nil {
				lvl.Set("value", strconv.FormatFloat(v, 'f', 0, 64))
				lvl.Call("dispatchEvent", js.Global().Get("Event").New("input"))
			}
			return nil
		}))
		wave.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			if w, err := strconv.Atoi(wave.Get("value").String()); err == nil {
				fg().SetWave(idx, w)
				genAudioUpdate(idx)
			}
			return nil
		}))
		// Channel ring: off mutes; any other value plays. Starts/stops the audio
		// graph as needed (no separate Listen toggle).
		out.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			genAudioSync()
			return nil
		}))
		// Push HTML defaults into the FuncGen.
		fg().SetFreq(idx, freqFromKnob(fgFloat(freq)))
		fg().SetAmp(idx, fgFloat(lvl)/100)
	}
}

// genAudioSync starts the Web Audio graph if any oscillator is routed to a
// channel (not "off"), stops it if none are, and otherwise just refreshes the
// running nodes.
func genAudioSync() {
	any := false
	for _, id := range genOscIDs {
		if o := doc.Call("getElementById", id+"-out"); o.Truthy() && o.Get("value").String() != "off" {
			any = true
		}
	}
	switch {
	case any && !genRunning:
		genAudioStart()
	case !any && genRunning:
		genAudioStop()
	case genRunning:
		for i := 0; i < 3; i++ {
			genAudioUpdate(i)
		}
	}
}

func fgFloat(el js.Value) float64 {
	v, _ := strconv.ParseFloat(el.Get("value").String(), 64)
	return v
}

// ── Web Audio output ──────────────────────────────────────────────────────

var (
	genCtx     js.Value
	genOsc     [3]js.Value
	genGain    [3]js.Value
	genPan     [3]js.Value
	genRunning bool
)

// genAudioStart builds the Web Audio graph (one OscillatorNode per generator →
// gain → stereo panner → speakers) and starts it, mirroring the FuncGen params.
func genAudioStart() {
	if genRunning {
		return
	}
	// We're inside a user-gesture handler, so the acquire's resume is allowed
	// under the browser autoplay policy.
	genCtx = acquireAudioCtx("gen")
	if !genCtx.Truthy() {
		return
	}
	for i := 0; i < 3; i++ {
		osc := genCtx.Call("createOscillator")
		gain := genCtx.Call("createGain")
		pan := genCtx.Call("createStereoPanner")
		osc.Call("connect", gain)
		gain.Call("connect", pan)
		pan.Call("connect", genCtx.Get("destination"))
		osc.Call("start")
		genOsc[i], genGain[i], genPan[i] = osc, gain, pan
	}
	genRunning = true
	for i := 0; i < 3; i++ {
		genAudioUpdate(i)
	}
}

func genAudioStop() {
	if !genRunning {
		return
	}
	for i := 0; i < 3; i++ {
		if genOsc[i].Truthy() {
			genOsc[i].Call("stop")
			genOsc[i].Call("disconnect")
		}
		if genPan[i].Truthy() {
			genPan[i].Call("disconnect")
		}
		genOsc[i], genGain[i], genPan[i] = js.Undefined(), js.Undefined(), js.Undefined()
	}
	genCtx = js.Undefined()
	genRunning = false
	releaseAudioCtx("gen")
}

// genAudioUpdate pushes oscillator i's waveform / frequency / channel routing to
// its Web Audio nodes.
func genAudioUpdate(i int) {
	if !genRunning || !genOsc[i].Truthy() {
		return
	}
	genOsc[i].Set("type", waveTypeName(fg().Wave(i)))
	genOsc[i].Get("frequency").Set("value", fg().Freq(i))
	// Channel routing from the dropdown: off / L / R / both.
	route := "off"
	if o := doc.Call("getElementById", genOscIDs[i]+"-out"); o.Truthy() {
		route = o.Get("value").String()
	}
	gain, pan := 0.0, 0.0
	switch route {
	case "l":
		gain, pan = fg().Amp(i), -1
	case "r":
		gain, pan = fg().Amp(i), 1
	case "both":
		gain, pan = fg().Amp(i), 0
	}
	genGain[i].Get("gain").Set("value", gain*0.3) // headroom
	genPan[i].Get("pan").Set("value", pan)
}
