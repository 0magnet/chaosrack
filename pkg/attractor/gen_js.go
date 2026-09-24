//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/skirt"
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
	dial := dom.Doc.Call("createElement", "span")
	dial.Set("className", "knob-dial value-dial")
	nOct := int(math.Log2(genFreqHi / genFreqLo)) // whole octaves in range
	for n := 0; n <= nOct; n++ {
		f := genFreqLo * math.Pow(2, float64(n))
		deg := -skirt.SweepDeg/2 + skirt.SweepDeg*knobFromFreq(f)/genSemitones
		l, tp := dialLabelPos(deg, 41)
		tk := dom.Doc.Call("createElement", "span")
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
		deg := -skirt.SweepDeg/2 + skirt.SweepDeg*knobFromFreq(e.f)/genSemitones
		l, tp := dialLabelPos(deg, 48)
		lab := dom.Doc.Call("createElement", "span")
		lab.Set("className", "knob-dial-lab")
		lab.Set("textContent", e.s)
		// Which end, and what it is in hertz — the ring says A0 and A10 because
		// every tick between them is an A, and the number is the thing a
		// measurement actually needs.
		end := "the lowest"
		if e.f > genFreqLo {
			end = "the highest"
		}
		lab.Set("title", e.s+" — "+strconv.FormatFloat(e.f, 'f', -1, 64)+" Hz, "+end+" this knob goes")
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
	wrap := dom.Doc.Call("createElement", "span")
	wrap.Set("className", "gen-piano")
	wrap.Call("setAttribute", "data-no-drag", "")
	whites := dom.Doc.Call("createElement", "span")
	whites.Set("className", "pk-whites")
	blacks := dom.Doc.Call("createElement", "span")
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
		el := dom.Doc.Call("createElement", "span")
		if black {
			el.Set("className", "pk-key pk-black")
			el.Get("style").Set("left", strconv.FormatFloat(leftPct, 'f', 2, 64)+"%")
		} else {
			el.Set("className", "pk-key pk-white")
		}
		el.Call("setAttribute", "data-pc", strconv.Itoa(pc))
		el.Set("title", owner+" — set note "+name+" (in the octave currently shown)")
		el.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, a []js.Value) any {
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
	freq.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, a []js.Value) any {
		highlight()
		return nil
	}))
	return wrap
}

// The Generator module: three independent oscillators (X / Y / Z), each a
// concentric knob (outer ring = waveform, inner = frequency) with a Hz LED and
// a speaker-channel dropdown. It drives the shared FuncGen (scope / features)
// and, when "Listen" is on, a parallel Web Audio graph for the speakers.

// genOscSpec is one oscillator of the signal generator: which DOM ids belong to it,
// which slot of the function generator it drives, and where its two knobs sit
// when nothing has been touched.
//
// One type because the same three oscillators were written out three times —
// genOscIDs for the ids, genOscIdx for the index, and an anonymous struct
// inside onResetAll carrying the defaults that the markup also carries. Three
// places to keep in step, and the defaults were already stated twice: once as
// the range input's value attribute and once in the reset. A ControlDesc's Def
// is now the only copy, and Reset All restores through the control rather than
// by writing remembered numbers back into the DOM.
type genOscSpec struct {
	id   string  // DOM id prefix: "gen-x" → gen-x-freq, gen-x-lvl, gen-x-out…
	idx  int     // slot in the function generator
	freq float64 // default frequency knob position, in semitones above A0
	lvl  float64 // default level, 0..100
}

// genOscs are the three, in panel order.
var genOscs = []genOscSpec{
	{id: "gen-x", idx: 0, freq: 34, lvl: 80},
	{id: "gen-y", idx: 1, freq: 41, lvl: 80},
	{id: "gen-z", idx: 2, freq: 29, lvl: 80},
}

// waveSVG holds a tiny glyph per waveform (index matches the wave <select>:
// 0 sine, 1 triangle, 2 square, 3 saw, 4 shift-register noise), stroked in
// currentColor so CSS can dim the ring and light the active one.
var waveSVG = []string{
	`<svg viewBox="0 0 24 12"><path d="M1,6 C3.2,1 5.8,1 8,6 C10.2,11 12.8,11 15,6 C17.2,1 19.8,1 22,6" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
	`<svg viewBox="0 0 24 12"><path d="M2,10 L7,2 L12,10 L17,2 L22,10" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
	`<svg viewBox="0 0 24 12"><path d="M2,10 L2,3 L8.5,3 L8.5,10 L15,10 L15,3 L21.5,3 L21.5,10" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
	`<svg viewBox="0 0 24 12"><path d="M2,10 L8,3 L8,10 L14,3 L14,10 L20,3 L20,10" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
	`<svg viewBox="0 0 24 12"><path d="M1,7 L3,7 L3,3 L5,3 L5,9 L8,9 L8,4 L10,4 L10,2 L13,2 L13,8 L15,8 L15,5 L18,5 L18,10 L20,10 L20,6 L23,6" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round"/></svg>`,
}

// addSelectorWaveDial adds an inner ring of waveform glyphs around a selector
// stack (one per option, at the knob's detent angles), lighting the active one
// and clickable to select — a graphic counterpart to addSelectorLabels for the
// waveform inner knob.
func addSelectorWaveDial(stack, sel js.Value, off float64) {
	// One glyph per OPTION (not per known waveform) — a select offering only
	// the periodic waves must not grow a phantom noise detent.
	n := min(sel.Get("options").Get("length").Int(), len(waveSVG))
	dial := dom.Doc.Call("createElement", "span")
	dial.Set("className", "knob-dial")
	circle := dom.Doc.Call("createElement", "span")
	circle.Set("className", "knob-ring-circle")
	dia := strconv.FormatFloat(2*off, 'f', 1, 64) + "%"
	circle.Get("style").Set("width", dia)
	circle.Get("style").Set("height", dia)
	dial.Call("appendChild", circle)
	els := make([]js.Value, n)
	for i := range n {
		deg := -skirt.SweepDeg/2 + skirt.SweepDeg*float64(i)/float64(n-1)
		l, t := dialLabelPos(deg, off)
		ic := dom.Doc.Call("createElement", "span")
		ic.Set("className", "knob-dial-wave clickable")
		ic.Set("innerHTML", waveSVG[i])
		ic.Get("style").Set("left", l)
		ic.Get("style").Set("top", t)
		dialPosTitle(ic, sel, i)
		els[i] = ic
		idx := i
		ic.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, a []js.Value) any {
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
	sel.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, a []js.Value) any { hi(); return nil }))
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
	for _, osc := range genOscs {
		id, idx := osc.id, osc.idx
		freq := dom.Doc.Call("getElementById", id+"-freq")
		wave := dom.Doc.Call("getElementById", id+"-wave")
		fstack := dom.Doc.Call("getElementById", id+"-fstack")
		lvl := dom.Doc.Call("getElementById", id+"-lvl")
		out := dom.Doc.Call("getElementById", id+"-out")
		lstack := dom.Doc.Call("getElementById", id+"-lstack")
		ostack := dom.Doc.Call("getElementById", id+"-ostack")
		if !freq.Truthy() || !fstack.Truthy() {
			continue
		}

		// Freq cell: single log-frequency knob with an octave tick ring. The hidden
		// slider is in semitones (step 1 = one semitone), so scrolling the knob or
		// the LED steps note-by-note (12 per octave); dragging still sweeps
		// continuously and the fine disc trims sub-semitone. The LED is zero-padded
		// to the widest value (so 20480 never clips to "2048") with a fixed decimal,
		// like every other readout — all of which the descriptor below sets up,
		// along with the typed entry and the wheel nudge that used to be written
		// out here beside every other cell that wanted them.
		fknob := makeKnob(freq, js.Undefined(), true, false, false)
		addOctaveDial(fknob)
		fstack.Call("appendChild", fknob)
		// One-octave piano strip under the knob, lighting the current note.
		fstack.Get("parentNode").Get("parentNode").Call("appendChild", addPianoKeys(freq))

		// Level cell: single level knob with a 0..100 value dial; the LED is the
		// descriptor's, in the same zero-padded, fixed-decimal style as the rest.
		lstack.Call("appendChild", makeKnob(lvl, js.Undefined(), true, false, true))

		// Out cell: dual concentric knob — outer ring = speaker channel (labeled),
		// inner = waveform. Two dial rings: the sink labels (outer) and a ring of
		// waveform glyphs (inner) that lights the selected wave.
		ostk := stackKnobs(selk.makeSelectorKnob(out), selk.makeSelectorKnob(wave))
		addSelectorLabels(ostk, []string{"off", "L", "R", "L+R"}, out)
		addSelectorWaveDial(ostk, wave, 38)
		ostack.Call("appendChild", ostk)

		// The two value knobs go through the descriptor path, which is what
		// gives them their LED formatting, typed entry, wheel nudge, reset and
		// the Control that Reset All drives — rather than each re-implementing
		// all of it beside the next one. Everything specific to an oscillator
		// is the Apply closure; the rest is the same machinery every parameter
		// cell in the rack already used.
		//
		// The frequency mapping is the one sonify_js.go already defines and,
		// until now, used exactly once: the slider is in semitones and the LED
		// is in hertz, which is precisely what SliderToVal / ValToSlider are
		// for.
		adoptDescControl(ControlDesc{
			ID: id + "-freq", Label: "freq", Min: 0, Max: float64(genSemitones), Step: 1, Def: osc.freq,
			LEDID: id + "-led", ResetID: "rst-" + id + "-freq",
			Apply: func(v float64) {
				aud.fg().SetFreq(idx, freqFromKnob(v))
				gen.audioUpdate(idx)
			},
			SliderToVal: sonifyFreqFromSlider,
			ValToSlider: sonifySliderFromFreq,
			LEDMin:      genFreqLo, LEDMax: genFreqHi, LEDStep: 1,
		})
		adoptDescControl(ControlDesc{
			ID: id + "-lvl", Label: "lvl", Min: 0, Max: 100, Step: 1, Def: osc.lvl,
			LEDID: id + "-lvl-led", ResetID: "rst-" + id + "-lvl",
			Apply: func(v float64) {
				aud.fg().SetAmp(idx, v/100)
				gen.audioUpdate(idx)
			},
		})
		// The two rings go through the descriptor path for the same reason the
		// two knobs above it do. They were the last part of an oscillator with
		// no way back: the freq and level knobs had reset buttons, the routing
		// and the waveform sharing the cell beside them had none, and Reset All
		// reached them only because it named them by hand.
		//
		// Channel ring: off mutes; any other value plays. Starts/stops the audio
		// graph as needed (no separate Listen toggle).
		adoptDescControl(ControlDesc{
			ID: id + "-out", Label: "out", IsSelect: true, SelectDef: "off",
			ResetID:     "rst-" + id + "-out",
			SelectApply: func(string) { gen.audioSync() },
		})
		adoptDescControl(ControlDesc{
			ID: id + "-wave", Label: "wave", IsSelect: true, SelectDef: "0",
			ResetID: "rst-" + id + "-out",
			SelectApply: func(v string) {
				if w, err := strconv.Atoi(v); err == nil {
					aud.fg().SetWave(idx, w)
					gen.audioUpdate(idx)
				}
			},
		})
		// Push HTML defaults into the FuncGen.
		aud.fg().SetFreq(idx, freqFromKnob(fgFloat(freq)))
		aud.fg().SetAmp(idx, fgFloat(lvl)/100)
	}
}

// audioSync starts the Web Audio graph if any oscillator is routed to a
// channel (not "off"), stops it if none are, and otherwise just refreshes the
// running nodes.
func (g *generator) audioSync() {
	any := false
	for _, osc := range genOscs {
		id := osc.id
		if o := dom.Doc.Call("getElementById", id+"-out"); o.Truthy() && o.Get("value").String() != "off" {
			any = true
		}
	}
	switch {
	case any && !g.running:
		g.audioStart()
	case !any && g.running:
		g.audioStop()
	case g.running:
		for i := range 3 {
			g.audioUpdate(i)
		}
	}
}

func fgFloat(el js.Value) float64 {
	v, _ := strconv.ParseFloat(el.Get("value").String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	return v
}

// ── Web Audio output ──────────────────────────────────────────────────────

// generator is the signal generator's audio graph.
type generator struct {
	ctx      js.Value
	osc      [3]js.Value
	kind     [3]string // "osc" or "noise" — which node type gen.osc[i] holds
	gain     [3]js.Value
	pan      [3]js.Value
	envGain  js.Value // Envelope module's master shaper (pans → env → out)
	noiseBuf js.Value // shared 2-s LFSR noise loop
	running  bool
}

var gen generator

// noiseBuffer builds (once) the shift-register noise loop the noise wave
// plays through an AudioBufferSourceNode — the same 15-bit LFSR the FuncGen
// analysis path steps, so what you hear is what the scope sees. The freq
// knob maps to playbackRate, sweeping the noise from rumble to hiss.
func (g *generator) noiseBuffer(ctx js.Value) js.Value {
	if g.noiseBuf.Truthy() {
		return g.noiseBuf
	}
	sr := int(ctx.Get("sampleRate").Float())
	buf := ctx.Call("createBuffer", 1, sr*2, sr)
	data := buf.Call("getChannelData", 0)
	lfsr := uint32(0x4001)
	for i := range sr * 2 {
		bit := (lfsr ^ (lfsr >> 1)) & 1
		lfsr = (lfsr >> 1) | (bit << 14)
		v := -1.0
		if lfsr&1 == 1 {
			v = 1
		}
		data.SetIndex(i, v)
	}
	g.noiseBuf = buf
	return buf
}

// ensureNode makes gen.osc[i] the right node type for the waveform —
// OscillatorNode for the periodic waves, a looped AudioBufferSourceNode of
// LFSR noise for wave 4 — replacing the node when the kind changes.
func (g *generator) ensureNode(i int, noise bool) {
	want := "osc"
	if noise {
		want = "noise"
	}
	if g.kind[i] == want && g.osc[i].Truthy() {
		return
	}
	if g.osc[i].Truthy() {
		g.osc[i].Call("stop")
		g.osc[i].Call("disconnect")
	}
	var node js.Value
	if noise {
		node = g.ctx.Call("createBufferSource")
		node.Set("buffer", g.noiseBuffer(g.ctx))
		node.Set("loop", true)
	} else {
		node = g.ctx.Call("createOscillator")
	}
	node.Call("connect", g.gain[i])
	node.Call("start")
	g.osc[i], g.kind[i] = node, want
}

// audioStart builds the Web Audio graph (one OscillatorNode per generator →
// gain → stereo panner → speakers) and starts it, mirroring the FuncGen params.
func (g *generator) audioStart() {
	if g.running {
		return
	}
	// We're inside a user-gesture handler, so the acquire's resume is allowed
	// under the browser autoplay policy.
	g.ctx = acquireAudioCtx("gen")
	if !g.ctx.Truthy() {
		return
	}
	// Pans feed the Envelope module's shaper gain, then the speakers.
	g.envGain = g.ctx.Call("createGain")
	g.envGain.Call("connect", g.ctx.Get("destination"))
	for i := range 3 {
		gain := g.ctx.Call("createGain")
		pan := g.ctx.Call("createStereoPanner")
		gain.Call("connect", pan)
		pan.Call("connect", g.envGain)
		g.gain[i], g.pan[i] = gain, pan
		g.kind[i] = ""
		g.ensureNode(i, aud.fg().Wave(i) == 4)
	}
	g.running = true
	for i := range 3 {
		g.audioUpdate(i)
	}
}

func (g *generator) audioStop() {
	if !g.running {
		return
	}
	for i := range 3 {
		if g.osc[i].Truthy() {
			g.osc[i].Call("stop")
			g.osc[i].Call("disconnect")
		}
		if g.pan[i].Truthy() {
			g.pan[i].Call("disconnect")
		}
		g.osc[i], g.gain[i], g.pan[i] = js.Undefined(), js.Undefined(), js.Undefined()
		g.kind[i] = ""
	}
	if g.envGain.Truthy() {
		g.envGain.Call("disconnect")
		g.envGain = js.Undefined()
	}
	g.ctx = js.Undefined()
	g.running = false
	releaseAudioCtx("gen")
}

// audioUpdate pushes oscillator i's waveform / frequency / channel routing to
// its Web Audio nodes.
func (g *generator) audioUpdate(i int) {
	if !g.running || !g.osc[i].Truthy() {
		return
	}
	noise := aud.fg().Wave(i) == 4
	g.ensureNode(i, noise)
	if noise {
		// The LFSR loop's clock tracks the freq knob via playback rate, the
		// same 32× mapping the analysis path uses.
		rate := aud.fg().Freq(i) * 32 / g.ctx.Get("sampleRate").Float()
		g.osc[i].Get("playbackRate").Set("value", rate)
	} else {
		g.osc[i].Set("type", waveTypeName(aud.fg().Wave(i)))
		g.osc[i].Get("frequency").Set("value", aud.fg().Freq(i))
	}
	// Channel routing from the dropdown: off / L / R / both.
	route := "off"
	if o := dom.Doc.Call("getElementById", genOscs[i].id+"-out"); o.Truthy() {
		route = o.Get("value").String()
	}
	gain, pan := 0.0, 0.0
	switch route {
	case "l":
		gain, pan = aud.fg().Amp(i), -1
	case "r":
		gain, pan = aud.fg().Amp(i), 1
	case "both":
		gain, pan = aud.fg().Amp(i), 0
	}
	g.gain[i].Get("gain").Set("value", gain*0.3) // headroom
	g.pan[i].Get("pan").Set("value", pan)
}
