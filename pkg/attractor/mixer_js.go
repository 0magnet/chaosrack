//go:build js && wasm

package attractor

// The Mixer: every sound on the rack, and where each one goes. A pin matrix,
// the Mod module's twin: a column for each source, a row for each place a
// source can go, and a pin where they cross puts the one into the other at
// the pin's gain. Several pins on a row are mixed; a negative gain inverts.
//
// The rows are the two places sound goes. SPK L and R are the speakers.
// RACK L and R are the rack's own signal, the one thing every meter, every
// audio-driven model, the Mod matrix and the scopes' RACK inputs read
// (audiosrc.Bus). They are separate on purpose: the capture is analyzed but
// never played, since it is the system's own audio and playing it back
// would feed itself — so its column has no speaker pins at all — and an
// instrument can be heard without disturbing what the meters read, or read
// without being heard.
//
// The speakers' half is built in Web Audio (mixOut): each source has an
// input, and each pin is a gain from it to a speaker. The rack's half of the
// sources Go makes itself — the capture, the generators and the model — is
// the bus's Mix; the rest, the instruments only Web Audio has, are sent on a
// pair of gains like the speakers' and come back to the bus through a tap
// (audiosrc.NewTap), the way a send comes back on a return.

import (
	"strconv"
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/led"
)

// mixSource is a column: a sound the rack makes or takes in.
type mixSource struct {
	key   string      // in a link and in the manual: mix-src=key
	group string      // the heading over it, with the columns beside it
	label string      // over its column
	in    audiosrc.In // the bus's, for a source Go makes; -1 for one only Web Audio has
	noSpk bool        // never on the speakers
}

// mixSources are the columns, in order.
var mixSources = []mixSource{
	{"cl", "cap", "L", audiosrc.InCaptureL, true},
	{"cr", "cap", "R", audiosrc.InCaptureR, true},
	{"g1", "gen", "1", audiosrc.InGen, false},
	{"g2", "gen", "2", audiosrc.InGen + 1, false},
	{"g3", "gen", "3", audiosrc.InGen + 2, false},
	{"g4", "gen", "4", audiosrc.InGen + 3, false},
	{"mx", "model", "X", audiosrc.InModelX, false},
	{"my", "model", "Y", audiosrc.InModelY, false},
	{"mz", "model", "Z", audiosrc.InModelZ, false},
	{"ky", "inst", "KY", -1, false},
	{"dr", "inst", "DR", -1, false},
	{"tm", "inst", "TM", -1, false},
	{"fx", "inst", "FX", -1, false},
}

// mixSrcIndex is the column of source key, or -1.
func mixSrcIndex(key string) int {
	for i, s := range mixSources {
		if s.key == key {
			return i
		}
	}
	return -1
}

// The rows.
const (
	mixSpkL = iota
	mixSpkR
	mixRackL
	mixRackR
	mixModA
	mixModB
	mixRows
)

// mixRowKeys and mixRowNames are each row's key, in a link and the manual,
// and its name on its display.
var (
	mixRowKeys  = [mixRows]string{"sl", "sr", "rl", "rr", "ma", "mb"}
	mixRowNames = [mixRows]string{"spk L", "spk R", "rack L", "rack R", "mod A", "mod B"}
)

// mixRackRow reports whether row is one Go reads rather than the speakers
// play: the rack's two, or the two sends to modulation.
func mixRackRow(row int) bool { return row >= mixRackL }

// mixModRow reports whether row is one of the sends to modulation.
func mixModRow(row int) bool { return row >= mixModA }

// mixAllowed reports whether source s can be pinned to row.
func mixAllowed(row, s int) bool { return mixRackRow(row) || !mixSources[s].noSpk }

// mixPinMax is the most a pin's gain can be either way.
const mixPinMax = 1

// mixDefaults is the Mixer as the rack starts: the capture on the rack's
// two sides, which is what a rack listening to the system always read, and
// every instrument on both speakers, which is where each one played before
// it had a column.
func mixDefaults() (m [mixRows][]float32) {
	for r := range m {
		m[r] = make([]float32, len(mixSources))
	}
	m[mixRackL][mixSrcIndex("cl")] = 1
	m[mixRackR][mixSrcIndex("cr")] = 1
	for _, k := range []string{"ky", "dr", "tm", "fx"} {
		s := mixSrcIndex(k)
		m[mixSpkL][s], m[mixSpkR][s] = 1, 1
	}
	return m
}

// mixer is the Mixer's state and its parts on the panel.
var mixer = struct {
	pin    [mixRows][]float32 // each pin's gain, 0 for none
	selRow int
	selSrc int // the selected pin, -1 for none
	pins   [mixRows][]js.Value
	lvl    js.Value // the hidden range behind LVL
	lvlRO  js.Value // its readout
	name   js.Value // the selected pin's name
	funcs  []js.Func
}{pin: mixDefaults(), selSrc: -1}

// mixHasSource reports whether source s is pinned anywhere.
func mixHasSource(s int) bool {
	for r := range mixer.pin {
		if mixer.pin[r][s] != 0 {
			return true
		}
	}
	return false
}

// mixOnSpeakers reports whether source s is pinned to a speaker.
func mixOnSpeakers(s int) bool { return mixer.pin[mixSpkL][s] != 0 || mixer.pin[mixSpkR][s] != 0 }

// mixCarriesCapture reports whether the capture is in the rack's signal or
// on a send, which is when it has to be made.
func mixCarriesCapture() bool {
	for _, k := range []string{"cl", "cr"} {
		s := mixSrcIndex(k)
		for r := mixRackL; r < mixRows; r++ {
			if mixer.pin[r][s] != 0 {
				return true
			}
		}
	}
	return false
}

// mixApply puts the pins where they act: the bus's mix, the speakers' and
// the return's gains, and whatever is heard starting or stopping with them.
func mixApply() {
	mixToBus(aud.rackBus())
	mixOutSync()
	gen.audioSync()
	son.sync()
	for r := range mixer.pins {
		mixLightRow(r)
	}
	scheduleDesignate() // the pins' tooltips were rewritten without their address
}

// mixToBus puts the pins of the sources Go makes into b's mixes, the rack's
// and the sends', and says whether anything is sent on either return.
func mixToBus(b *audiosrc.Bus) {
	b.ReturnOn, b.ModReturnOn = false, false
	for s, src := range mixSources {
		if src.in >= 0 {
			b.Mix[0][src.in] = mixer.pin[mixRackL][s]
			b.Mix[1][src.in] = mixer.pin[mixRackR][s]
			b.ModMix[0][src.in] = mixer.pin[mixModA][s]
			b.ModMix[1][src.in] = mixer.pin[mixModB][s]
			continue
		}
		b.ReturnOn = b.ReturnOn || mixer.pin[mixRackL][s] != 0 || mixer.pin[mixRackR][s] != 0
		b.ModReturnOn = b.ModReturnOn || mixer.pin[mixModA][s] != 0 || mixer.pin[mixModB][s] != 0
	}
}

// mixSetPin sets one pin and puts it to work.
func mixSetPin(row, s int, g float32) {
	if !mixAllowed(row, s) {
		return
	}
	g = max(-mixPinMax, min(mixPinMax, g))
	if g > -0.005 && g < 0.005 {
		g = 0
	}
	mixer.pin[row][s] = g
	mixApply()
	perma.syncPermalinkNow()
}

// mixAutoPin pins generator g, switched on by hand while it is pinned
// nowhere, to one side of the speakers and the rack: Gen 1 and 3 on the
// left, 2 and 4 on the right, so the first two switched on are a stereo
// pair and the xy scope draws them against each other. Switching one on is
// asking to hear it and see it.
func mixAutoPin(g int) bool {
	s := mixSrcIndex("g" + strconv.Itoa(g+1))
	if mixHasSource(s) {
		return false
	}
	spk, rack := mixSpkL, mixRackL
	if g%2 == 1 {
		spk, rack = mixSpkR, mixRackR
	}
	mixer.pin[spk][s], mixer.pin[rack][s] = 1, 1
	mixApply()
	perma.syncPermalinkNow()
	return true
}

// ── the link ─────────────────────────────────────────────────────────────

// mixLinkKey is the Mixer's key in a link: every pin that differs from the
// rack as it starts, as row.source, with :gain when the gain is not 1 —
// rr.g2:-0.5 — and a pin taken away as gain 0.
const mixLinkKey = "mx"

// mixLinkValue is the Mixer as a link carries it, "" when it is as it
// starts.
func mixLinkValue() string {
	def := mixDefaults()
	var parts []string
	for r := range mixer.pin {
		for s, g := range mixer.pin[r] {
			if g == def[r][s] {
				continue
			}
			p := mixRowKeys[r] + "." + mixSources[s].key
			if g != 1 {
				p += ":" + strconv.FormatFloat(float64(g), 'g', 3, 32)
			}
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ",")
}

// applyMixLink sets the Mixer from a link's value: the rack as it starts,
// and the pins the link changes.
func applyMixLink(v string) {
	mixer.pin = mixDefaults()
	for p := range strings.SplitSeq(v, ",") {
		rs, gs, hasGain := strings.Cut(p, ":")
		rk, sk, ok := strings.Cut(rs, ".")
		if !ok {
			continue
		}
		row := -1
		for r, k := range mixRowKeys {
			if k == rk {
				row = r
			}
		}
		s := mixSrcIndex(sk)
		if row < 0 || s < 0 || !mixAllowed(row, s) {
			continue
		}
		g := float32(1)
		if hasGain {
			f, err := strconv.ParseFloat(gs, 32)
			if err != nil {
				continue
			}
			g = float32(max(-mixPinMax, min(mixPinMax, f)))
		}
		mixer.pin[row][s] = g
	}
	mixApply()
	mixShowSelected()
}

// mixReset puts the Mixer back as the rack starts (Reset All).
func mixReset() {
	mixer.pin = mixDefaults()
	mixer.selSrc = -1
	mixApply()
	mixShowSelected()
}

// ── the speakers' half ───────────────────────────────────────────────────

// mixOut is the Mixer in Web Audio: an input for each source; from each, a
// gain to each speaker it may be pinned to and, for a source only Web Audio
// has, a gain to each side of the rack's return and of the sends'; the
// speakers' pair into the output, and each return's into a tap the bus reads. Built on first use
// and kept: the shared context only ever suspends, so a graph made on it
// stays good (audioctx_js.go). users is how many are heard through it, the
// lease it holds on the context.
var mixOut struct {
	ctx   js.Value
	in    []js.Value
	send  [mixRows][]js.Value
	spk   js.Value
	ret   js.Value // the rack's return: RACK L and R
	mret  js.Value // the sends' return: MOD A and B
	tap   audiosrc.Source
	mtap  audiosrc.Source
	users int
}

// mixAcquire is the shared context with the Mixer built on it, for a source
// about to be heard through it, or an undefined value when there is no
// context to be had. Each acquire is one user until its mixRelease.
func mixAcquire() js.Value {
	ctx := acquireAudioCtx("mixer")
	if !ctx.Truthy() {
		return js.Undefined()
	}
	if !mixOut.ctx.Truthy() {
		mixBuild(ctx)
	}
	mixOut.users++
	return ctx
}

// mixRelease is a source no longer heard; the last lets go of the context.
func mixRelease() {
	if mixOut.users == 0 {
		return
	}
	mixOut.users--
	if mixOut.users == 0 {
		releaseAudioCtx("mixer")
	}
}

// mixEnsure builds the Mixer on ctx, a context the caller already holds a
// lease on, for an instrument that keeps its own: the keys, the drums and
// the tone matrix for the life of the page, a model's sounds while it runs.
func mixEnsure(ctx js.Value) {
	if ctx.Truthy() && !mixOut.ctx.Truthy() {
		mixBuild(ctx)
	}
}

// mixIn is source key's input, for a source to connect to while it holds
// an acquire or after a mixEnsure.
func mixIn(key string) js.Value {
	s := mixSrcIndex(key)
	if s < 0 || s >= len(mixOut.in) {
		return js.Undefined()
	}
	return mixOut.in[s]
}

// mixBuild makes the graph on ctx.
func mixBuild(ctx js.Value) {
	mixOut.ctx = ctx
	mixOut.spk = ctx.Call("createChannelMerger", 2)
	mixOut.spk.Call("connect", ctx.Get("destination"))
	mixOut.ret = ctx.Call("createChannelMerger", 2)
	mixOut.mret = ctx.Call("createChannelMerger", 2)
	mixOut.in = make([]js.Value, len(mixSources))
	for r := range mixOut.send {
		mixOut.send[r] = make([]js.Value, len(mixSources))
	}
	for s, src := range mixSources {
		if src.noSpk && src.in >= 0 {
			continue // the capture: Go has it, and the speakers never do
		}
		in := ctx.Call("createGain")
		// One signal a column: a stereo instrument is summed into it, and
		// its pins put it where it goes.
		in.Set("channelCount", 1)
		in.Set("channelCountMode", "explicit")
		mixOut.in[s] = in
		for r := range mixRows {
			if !mixAllowed(r, s) || (mixRackRow(r) && src.in >= 0) {
				continue // the bus mixes Go's own sources for the rack
			}
			g := ctx.Call("createGain")
			g.Get("gain").Set("value", 0)
			in.Call("connect", g)
			dst := mixOut.spk
			switch {
			case mixModRow(r):
				dst = mixOut.mret
			case mixRackRow(r):
				dst = mixOut.ret
			}
			g.Call("connect", dst, 0, r%2)
			mixOut.send[r][s] = g
		}
	}
	mixOutSync()
}

// mixOutSync sets the graph's gains from the pins, and makes a return's tap
// when something is first sent through it.
func mixOutSync() {
	if !mixOut.ctx.Truthy() {
		return
	}
	for r := range mixOut.send {
		for s, g := range mixOut.send[r] {
			if g.Truthy() {
				g.Get("gain").Set("value", mixer.pin[r][s])
			}
		}
	}
	if aud.rackBus().ReturnOn && mixOut.tap == nil {
		mixOut.tap = audiosrc.NewTap(mixOut.ctx, mixOut.ret)
		aud.rackBus().Return = func() audiosrc.Source { return mixOut.tap }
	}
	if aud.rackBus().ModReturnOn && mixOut.mtap == nil {
		mixOut.mtap = audiosrc.NewTap(mixOut.ctx, mixOut.mret)
		aud.rackBus().ModReturn = func() audiosrc.Source { return mixOut.mtap }
	}
}

// ── the module ───────────────────────────────────────────────────────────

// buildMixer builds the Mixer module beside the Mod matrix (the packer puts
// it in the modulation bay; racksection.go).
func buildMixer() {
	if dom.Doc.Call("getElementById", "mixer-module").Truthy() {
		return
	}
	at := dom.Doc.Call("getElementById", "params-module")
	if !at.Truthy() {
		return
	}
	dom.RebuildInto(&mixer.funcs, func() {
		at.Get("parentNode").Call("insertBefore", mixModule(), at.Get("nextSibling"))
	})
	// For the tools that drive the rack (uitool): the Mixer set from a link's
	// value, "" for as it starts, and what it is now. For the life of the page.
	js.Global().Set("rackmix", js.FuncOf(func(_ js.Value, a []js.Value) any {
		if len(a) > 0 && a[0].Type() == js.TypeString {
			applyMixLink(a[0].String())
			perma.syncPermalinkNow()
		}
		return mixLinkValue()
	}))
	mixApply()
	mixShowSelected()
}

// mixModule is the module: the matrix, and the selected pin's cells under it.
func mixModule() js.Value {
	mod := dom.Doc.Call("createElement", "div")
	mod.Set("className", "sect")
	mod.Set("id", "mixer-module")
	h := dom.Doc.Call("createElement", "div")
	h.Set("className", "sect-hdr")
	h.Set("textContent", "Mixer")
	h.Call("setAttribute", "data-doc", "mixer")
	h.Set("title", doc("mixer"))
	mod.Call("appendChild", h)
	body := dom.Doc.Call("createElement", "div")
	body.Set("className", "row modmx")
	body.Call("appendChild", mixGrid())
	body.Call("appendChild", mixEditRow())
	mod.Call("appendChild", body)
	return mod
}

// mixGrid is the pin matrix: the sources' groups over their columns, each
// column's name, then a row of pins under each row's name.
func mixGrid() js.Value {
	grid := dom.Doc.Call("createElement", "div")
	grid.Set("className", "mxgrid")
	grid.Call("setAttribute", "data-doc", "mixer-matrix")
	grid.Set("title", doc("mixer-matrix"))
	grid.Get("style").Set("gridTemplateColumns", "auto repeat("+strconv.Itoa(len(mixSources))+",auto)")
	// The groups, one heading over each run of columns.
	grid.Call("appendChild", mxAxis("from ▸", "mx-from"))
	for s := 0; s < len(mixSources); {
		e := s
		for e < len(mixSources) && mixSources[e].group == mixSources[s].group {
			e++
		}
		g := dom.Doc.Call("createElement", "span")
		g.Set("className", "mxlbl mixgrp")
		g.Set("textContent", mixSources[s].group)
		g.Call("setAttribute", "data-doc", "mix-group="+mixSources[s].group)
		g.Set("title", doc("mix-group="+mixSources[s].group))
		g.Get("style").Set("gridColumn", "span "+strconv.Itoa(e-s))
		grid.Call("appendChild", g)
		s = e
	}
	grid.Call("appendChild", mxAxis("to ▾", "mx-to"))
	for _, src := range mixSources {
		l := dom.Doc.Call("createElement", "span")
		l.Set("className", "mxlbl")
		l.Set("textContent", src.label)
		l.Call("setAttribute", "data-doc", "mix-src="+src.key)
		l.Set("title", doc("mix-src="+src.key))
		grid.Call("appendChild", l)
	}
	for r := range mixRows {
		lg := dotDisplayN(mixRowNames[r], false, dispFullChars)
		lg.Get("classList").Call("add", "mxleg")
		lg.Call("setAttribute", "data-doc", "mix-row="+mixRowKeys[r])
		lg.Set("title", doc("mix-row="+mixRowKeys[r]))
		grid.Call("appendChild", lg)
		mixer.pins[r] = make([]js.Value, len(mixSources))
		for s := range mixSources {
			pin := dom.Doc.Call("createElement", "span")
			pin.Set("className", "mxpin")
			pin.Call("setAttribute", "data-pin", mixRowKeys[r]+"."+mixSources[s].key)
			grid.Call("appendChild", pin)
			mixer.pins[r][s] = pin
			if !mixAllowed(r, s) {
				pin.Get("classList").Call("add", "mxpin-none")
				pin.Set("title", docf("mixer-pin.none", "src", mixSrcName(s), "row", mixRowName(r)))
				continue
			}
			row, src := r, s
			pin.Call("addEventListener", "click", dom.FuncOf(func(js.Value, []js.Value) any {
				g := float32(1)
				if mixer.pin[row][src] != 0 {
					g = 0
				}
				mixer.selRow, mixer.selSrc = row, src
				mixSetPin(row, src, g)
				mixShowSelected()
				return nil
			}))
			pin.Call("addEventListener", "wheel", dom.FuncOf(func(_ js.Value, a []js.Value) any {
				e := a[0]
				e.Call("preventDefault")
				step := float32(0.05)
				if e.Get("deltaY").Float() > 0 {
					step = -step
				}
				mixer.selRow, mixer.selSrc = row, src
				mixSetPin(row, src, mixer.pin[row][src]+step)
				mixShowSelected()
				return nil
			}), map[string]any{"passive": false})
		}
	}
	return grid
}

// mixSrcName is source s as the manual and a tooltip name it: CAP L, GEN 2,
// MODEL X, KEYS.
func mixSrcName(s int) string {
	src := mixSources[s]
	switch src.group {
	case "inst":
		return map[string]string{"ky": "KEYS", "dr": "DRUMS", "tm": "MATRIX", "fx": "SOUNDS"}[src.key]
	}
	return strings.ToUpper(src.group) + " " + src.label
}

// mixRowName is row r as a tooltip names it.
func mixRowName(r int) string { return strings.ToUpper(mixRowNames[r]) }

// mixLightRow lights row r's pins: each as bright as its gain, and an
// inverting one in the color of a minus.
func mixLightRow(r int) {
	for s, pin := range mixer.pins[r] {
		if !pin.Truthy() || !mixAllowed(r, s) {
			continue
		}
		g := mixer.pin[r][s]
		cl := pin.Get("classList")
		cl.Call("toggle", "on", g != 0)
		cl.Call("toggle", "neg", g < 0)
		cl.Call("toggle", "sel", r == mixer.selRow && s == mixer.selSrc)
		op := ""
		if g != 0 {
			a := float64(g)
			if a < 0 {
				a = -a
			}
			op = strconv.FormatFloat(min(1, 0.45+0.55*a), 'f', 2, 64)
		}
		pin.Get("style").Set("opacity", op)
		pin.Set("title", docf("mixer-pin", "src", mixSrcName(s), "row", mixRowName(r),
			"gain", led.Format(float64(g), 1, 2, true)))
	}
}

// mixEditRow is the selected pin under the matrix: its name, and LVL, its
// gain.
func mixEditRow() js.Value {
	row := dom.Doc.Call("createElement", "div")
	row.Set("className", "modmx-edit")
	cell := func(label, key string) (js.Value, js.Value) {
		c := dom.Doc.Call("createElement", "span")
		c.Set("className", "pcell axcol vmcell")
		c.Call("setAttribute", "data-doc", key)
		c.Set("title", doc(key))
		top := dom.Doc.Call("createElement", "span")
		top.Set("className", "punit-top")
		l := dom.Doc.Call("createElement", "span")
		l.Set("className", "plabel")
		l.Set("textContent", label)
		top.Call("appendChild", l)
		c.Call("appendChild", top)
		row.Call("appendChild", c)
		return c, top
	}
	_, ntop := cell("pin", "mixer-sel")
	mixer.name = dotDisplayN("", false, dispFullChars)
	mixer.name.Get("classList").Call("add", "dmdval")
	ntop.Call("appendChild", mixer.name)

	pc, ptop := cell("lvl", "mixer-lvl")
	rng := dom.Doc.Call("createElement", "input")
	rng.Set("type", "range")
	rng.Set("id", "mixer-lvl")
	rng.Set("min", "-1")
	rng.Set("max", "1")
	rng.Set("step", "0.01")
	rng.Set("value", "0")
	rng.Set("style", "display:none")
	rng.Set("title", doc("mixer-lvl"))
	ro := dom.Doc.Call("createElement", "input")
	ro.Set("type", "text")
	ro.Set("inputmode", "decimal")
	ro.Set("className", "numin")
	ro.Set("title", doc("mixer-lvl"))
	ptop.Call("appendChild", ro)
	sizeLEDField(ro, -1, 1, 2, true)
	pc.Call("appendChild", rng)
	pc.Call("appendChild", makeKnob(rng, ro, true, false, true))
	rng.Call("addEventListener", "input", dom.FuncOf(func(js.Value, []js.Value) any {
		v, err := strconv.ParseFloat(rng.Get("value").String(), 64)
		if err != nil || mixer.selSrc < 0 {
			return nil
		}
		ro.Set("value", led.Format(v, 1, 2, true))
		mixSetPin(mixer.selRow, mixer.selSrc, float32(v))
		return nil
	}))
	ro.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) any {
		if v, err := led.Parse(ro.Get("value").String()); err == nil {
			rng.Set("value", strconv.FormatFloat(v, 'g', -1, 64))
			dom.Fire(rng, "input")
		}
		return nil
	}))
	mixer.lvl, mixer.lvlRO = rng, ro
	return row
}

// mixRowShort is each row as the selected pin's display names it: eight
// characters hold a source, an arrow and one of these.
var mixRowShort = [mixRows]string{"spk L", "spk R", "rck L", "rck R", "mod A", "mod B"}

// mixShowSelected shows the selected pin under the matrix: its name, as
// G2>SPK L, and its gain on LVL.
func mixShowSelected() {
	if !mixer.name.Truthy() {
		return
	}
	name, g := "", float32(0)
	if s := mixer.selSrc; s >= 0 {
		name = mixSources[s].group[:1] + mixSources[s].label + ">" + mixRowShort[mixer.selRow]
		if mixSources[s].group == "inst" {
			name = mixSources[s].label + ">" + mixRowShort[mixer.selRow]
		}
		g = mixer.pin[mixer.selRow][s]
	}
	setDotText(mixer.name, strings.ToUpper(name))
	mixer.lvl.Set("value", strconv.FormatFloat(float64(g), 'g', -1, 32))
	mixer.lvlRO.Set("value", led.Format(float64(g), 1, 2, true))
	for r := range mixer.pins {
		mixLightRow(r)
	}
	scheduleDesignate()
}

// mxAxis is a pin matrix's corner, saying which way its axes run: FROM over
// the sources' columns, TO over the rows they go to. The Mixer and the Mod
// matrix both have one.
func mxAxis(text, key string) js.Value {
	s := dom.Doc.Call("createElement", "span")
	s.Set("className", "mxlbl mxaxis")
	s.Set("textContent", text)
	s.Call("setAttribute", "data-doc", key)
	s.Set("title", doc(key))
	return s
}
