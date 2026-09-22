//go:build js && wasm

package attractor

import (
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
	row := doc.Call("querySelector", "#"+moduleID+" .vmrow")
	if !row.Truthy() {
		return
	}
	selID, stackID, resetID := name, name+"-stack", "rst-"+name
	row.Call("insertAdjacentHTML", "beforeend",
		`<span class="pcell axcol vmcell gen-cell">`+
			`<span class="punit-top"><span class="plabel">`+label+`</span></span>`+
			`<span class="grp vmbay"><span id="`+stackID+`"></span></span>`+
			`<button class="rst" id="`+resetID+`" title="Reset `+label+`">&#8634;</button>`+
			`<select id="`+selID+`" title="`+title+`" style="display:none"></select></span>`)

	sel := doc.Call("getElementById", selID)
	stack := doc.Call("getElementById", stackID)
	if !sel.Truthy() || !stack.Truthy() {
		return
	}
	for _, d := range detents {
		opt := doc.Call("createElement", "option")
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
		{100, "10/s", "Ten readings a second — as fast as a readout can be followed"},
		{200, "5/s", "Five a second: the default, and about the rate a needle settles at"},
		{500, "2/s", "Twice a second — steadier to read, and a fifth of the writes"},
		{1000, "1/s", "Once a second, for a number being watched rather than chased"},
	}
	thdRateDetents = []meterDetent{
		{200, "5/s", "Five a second — the window is 341 ms, so this is as often as there is new audio to measure"},
		{400, "2.5/s", "The default: a fresh window every time, and no audio measured twice"},
		{1000, "1/s", "Once a second — the same reading, a fifth of the work"},
		{2000, "0.5/s", "Every two seconds, for a distortion figure being logged rather than tuned"},
	}
	wfRateDetents = []meterDetent{
		{250, "4/s", "Four a second — the most responsive, and the most expensive: this analysis walks the whole window each time"},
		{500, "2/s", "The default"},
		{1000, "1/s", "Once a second — halves the largest single lump of work in the rack"},
		{2000, "0.5/s", "Every two seconds. A reading that settles over ten seconds does not need remaking faster than this."},
	}
	wfWindowDetents = []meterDetent{
		{2, "2s", "Two seconds — flutter only. Too short to see wow at all, and the cheapest by five times."},
		{5, "5s", "Five seconds — two cycles of the slowest wow, and half the work of ten"},
		{10, "10s", "Ten seconds: the default, five cycles of the slowest wow"},
		{20, "20s", "Twenty seconds — the steadiest reading and twice the work"},
	}
)

// wireMeterClocks puts RATE on each analyzer and WINDOW where the window is
// the operator's to choose. Called once from Run, after the modules are wired.
func wireMeterClocks() {
	addMeterSwitch("lufs-module", "lufs-rate", "rate",
		"How often the loudness readouts latch. This is a DISPLAY rate only — the meter integrates every sample that arrives whatever this says, because an integrated loudness with a block missing is a block missing from the answer. Slower is steadier to read and costs the panel less.",
		"lr", lufsRateDetents, 200, func(v int) { lufsPeriodMs = float64(v) })

	addMeterSwitch("thd-module", "thd-rate", "rate",
		"How often the distortion measurement is made and shown. The analysis window is 341 ms, so measuring faster than about three times a second measures the same audio twice; slower is the same reading for less work.",
		"tr", thdRateDetents, 400, func(v int) { thdPeriodMs = float64(v) })

	addMeterSwitch("wf-module", "wf-rate", "rate",
		"How often the wow-and-flutter measurement is remade. This does not make the analysis cheaper — it makes it rarer. The same lump of work lands on one frame in sixty instead of one in thirty, so this is the control for how OFTEN the rack hesitates, not for how much.",
		"wr", wfRateDetents, 500, func(v int) { wfPeriodMs = float64(v) })

	addMeterSwitch("wf-module", "wf-win", "window",
		"How much audio each wow-and-flutter reading is made over. This is the measurement: ten seconds holds five cycles of the slowest wow, and two seconds cannot see wow at all, only flutter. It is also the cost — the analysis walks the whole window — so unlike RATE, this is the control that makes the work itself smaller.",
		"ww", wfWindowDetents, 10, func(v int) {
			wfWindowSec = v
			// The window it was measuring no longer describes what is being
			// asked about, the same way a change of channel does on the
			// distortion module.
			wfWin.Reset()
			wfRes = meters.WowFlutterResult{}
			showWowFlutter()
		})
}
