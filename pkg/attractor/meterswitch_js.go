//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"html"
	"strconv"

	"github.com/0magnet/chaosrack/pkg/meters"
)

// The meters' own clocks, on the front panel.
//
// Every analyzer here had its period written into the source — 200 ms for
// loudness, 400 for distortion, 500 for wow and flutter — and its measurement
// window with it. Those are not implementation details. A meter's response
// time is a control on every real instrument that has one: VU against PPM,
// FAST and SLOW on a sound level meter, and the rack's own TIME/DIV is the
// same idea. The operator owns the trade and the panel should say so.
//
// TWO CONTROLS AND NOT ONE, because the period was doing two jobs. RATE is
// how often the reading is taken and shown, which is a human-factors number:
// a readout that changes sixty times a second cannot be read, and a real
// meter damps deliberately. WINDOW is how much audio each reading is taken
// over, which is a measurement number: it sets the frequency resolution, the
// noise floor and the slowest wobble that can be seen.
//
// They pull opposite ways on the frame, and that is the point of separating
// them. RATE does not make an analysis cheaper, it makes it rarer — the same
// lump of work lands on one frame in thirty instead of one in fifteen, so it
// trades how OFTEN the rack hesitates. WINDOW makes the lump itself smaller
// and trades what the reading can resolve. Neither knob is the other's
// substitute, and with one control you could only ever have had one of them.
//
// What is NOT here: the loudness integration's own windows. Momentary is
// 400 ms and short-term is 3 s because BS.1770 says so, and a knob that moved
// them would not give a different view of the program, it would give a
// number that is not LUFS. Nor the true-peak filter's taps. The line is that
// a control belongs on the panel when the operator owns the trade-off, and
// stays in the source when there is one right answer.

// meterDetent is one position on a RATE or WINDOW switch.
type meterDetent struct {
	V     int    // the value it sets
	Label string // what is printed around the dial
	Desc  string // the tooltip on that label
}

func detentLabels(d []meterDetent) []string {
	out := make([]string, len(d))
	for i, x := range d {
		out[i] = x.Label
	}
	return out
}

// addMeterSwitch appends a switch to a meter's panel row and wires it.
//
// The cell is built here rather than written into panelhtml_js.go five times:
// the ids of the select, its knob stack and its reset have to agree with each
// other, and a typo in one of them shows up as a knob that silently does
// nothing rather than as an error. One call, one switch, ids derived from one
// name.
//
// apply is called when the switch moves, and once at wiring with the default
// so the Go side starts out agreeing with the panel. It is never called on a
// frame — that is the whole difference from the controls this pattern
// replaces, where the counter reads its gate time out of the DOM sixty times
// a second. scopeState says why: the listeners write in, the draw reads.
func addMeterSwitch(moduleID, name, label, title, permaKey string, detents []meterDetent, def int, apply func(int)) {
	row := dom.Doc.Call("querySelector", "#"+moduleID+" .vmrow")
	if !row.Truthy() {
		return
	}
	selID, stackID, resetID := name, name+"-stack", "rst-"+name
	row.Call("insertAdjacentHTML", "beforeend",
		`<span class="pcell axcol vmcell gen-cell">`+
			`<span class="punit-top"><span class="plabel">`+label+`</span></span>`+
			`<span class="grp vmbay"><span id="`+stackID+`"></span></span>`+
			`<button class="rst" id="`+resetID+`" title="`+html.EscapeString(docf("reset", "label", label))+`">&#8634;</button>`+
			`<select id="`+selID+`" title="`+title+`" style="display:none"></select></span>`)

	sel := dom.Doc.Call("getElementById", selID)
	stack := dom.Doc.Call("getElementById", stackID)
	if !sel.Truthy() || !stack.Truthy() {
		return
	}
	for _, d := range detents {
		opt := dom.Doc.Call("createElement", "option")
		opt.Set("value", strconv.Itoa(d.V))
		opt.Set("textContent", d.Label)
		opt.Set("title", d.Desc)
		sel.Call("appendChild", opt)
	}
	defStr := strconv.Itoa(def)
	sel.Set("value", defStr)
	stack.Call("appendChild", singleSelectorKnob(sel, detentLabels(detents)))
	adoptDescControl(ControlDesc{
		ID: selID, Label: label, IsSelect: true, SelectDef: defStr,
		PermaKey: permaKey, ResetID: resetID,
		SelectApply: func(v string) {
			if n, err := strconv.Atoi(v); err == nil {
				apply(n)
			}
		},
	})
	apply(def)
}

// rateDetents and the rest are the positions on each meter's switches. Named
// in the units the panel prints, which for a rate is how often rather than
// how long: a meter is specified by its response, not by its period.
var (
	lufsRateDetents = []meterDetent{
		{100, "10/s", doc("lufs-rate=100")},
		{200, "5/s", doc("lufs-rate=200")},
		{500, "2/s", doc("lufs-rate=500")},
		{1000, "1/s", doc("lufs-rate=1000")},
	}
	thdRateDetents = []meterDetent{
		{200, "5/s", doc("thd-rate=200")},
		{400, "2.5/s", doc("thd-rate=400")},
		{1000, "1/s", doc("thd-rate=1000")},
		{2000, "0.5/s", doc("thd-rate=2000")},
	}
	wfRateDetents = []meterDetent{
		{250, "4/s", doc("wf-rate=250")},
		{500, "2/s", doc("wf-rate=500")},
		{1000, "1/s", doc("wf-rate=1000")},
		{2000, "0.5/s", doc("wf-rate=2000")},
	}
	wfWindowDetents = []meterDetent{
		{2, "2s", doc("wf-win=2")},
		{5, "5s", doc("wf-win=5")},
		{10, "10s", doc("wf-win=10")},
		{20, "20s", doc("wf-win=20")},
	}
)

// wireMeterClocks puts RATE on each analyzer and WINDOW where the window is
// the operator's to choose. Called once from Run, after the modules are wired.
func wireMeterClocks() {
	addMeterSwitch("lufs-module", "lufs-rate", "rate",
		doc("lufs-rate"),
		"lr", lufsRateDetents, 200, func(v int) { lufs.periodMs = float64(v) })

	addMeterSwitch("thd-module", "thd-rate", "rate",
		doc("thd-rate"),
		"tr", thdRateDetents, 400, func(v int) { thd.periodMs = float64(v) })

	addMeterSwitch("wf-module", "wf-rate", "rate",
		doc("wf-rate"),
		"wr", wfRateDetents, 500, func(v int) { wow.periodMs = float64(v) })

	addMeterSwitch("wf-module", "wf-win", "window",
		doc("wf-win"),
		"ww", wfWindowDetents, 10, func(v int) {
			wow.windowSec = v
			// The window it was measuring no longer describes what is being
			// asked about, the same way a change of channel does on the
			// distortion module.
			wow.win.Reset()
			wow.res = meters.WowFlutterResult{}
			wow.showWowFlutter()
		})
}
