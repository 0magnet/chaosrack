//go:build js && wasm

package attractor

// Bifurcation explorer — the fig-tree view. Sweeps one parameter of the most
// recent flow mode across its knob range, integrating a fresh trajectory per
// column and plotting the local maxima of z against the parameter: periodic
// windows show as thin branches, period-doubling cascades fan out, chaos
// fills bands. The diagram computes progressively (a few columns per frame,
// left to right) so it grows across the screen live. The Parameters module
// holds the swept-parameter selector; editing the source system's parameters
// elsewhere re-sweeps.
//
// DRIVE picks what puts the system at a parameter value. On "sweep" that is
// the sweep itself and the view is the diagram alone. On "audio" the sweep
// still computes the diagram — it has to, there would be nothing to point at
// otherwise — and the live audio envelope moves a cursor along it, so the
// branch structure under the music is visible while the music is playing.
// bifdrive.go carries the argument for why the cursor moves and the diagram
// does not.

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"strconv"
	"syscall/js"
)

// bifurcation is the bifurcation explorer: the sweep, the columns drawn so
// far, and the readout.
type bifurcation struct {
	BifSweep            // the sweep and its columns (bifsweep.go)
	lastFlowMode string // most recent mode with a registered flow
	paramIdx     int    // index into attractorParams[lastFlowMode]; -1 = auto
	fitDone      bool

	// DRIVE: false sweeps the parameter, true points a cursor at it from the
	// live audio envelope. Not a paramDef because it is not a quantity — it
	// is a choice between two things, which is what the swept-parameter
	// control beside it is too, and both are selects for that reason.
	driveAudio bool
	curEl      js.Value // the cursor readout in the panel
	curText    string   // last text written to it

	// sweepFilling is set while SWEEP's options are refilled, so the change
	// event that refill sends its displays is not taken for a new choice.
	sweepFilling bool

	// depth is the fraction of the swept parameter's full range the envelope is
	// mapped across. A third by default rather than the whole axis: at full depth
	// a quiet passage parks the cursor at one end and a loud one throws it to the
	// other, which is a level meter drawn sideways. A third of the range around
	// the knob is enough to cross a bifurcation and not so much that every drum
	// hit crosses all of them.
	depth  float32
	curBuf []float32 // cursor vertex scratch (vertBuf holds the diagram)
}

var bif = bifurcation{
	lastFlowMode: "lorenz",
	paramIdx:     -1,
	depth:        0.33,
}

// DEPTH is bifurcation's one parameter: a knob in the Visual bank like any
// model's constant, which is what a parameter gets.
func init() {
	attractorParams["bifurcation"] = []paramDef{
		{"bif-depth", "depth", &bif.depth, 0.33, 0, 1, 0.01},
	}
}

func (b *bifurcation) invalidate() { b.sig = "" }

// params returns the sweepable parameter list of the source mode.
func (b *bifurcation) params() []paramDef { return attractorParams[b.lastFlowMode] }

// param picks the swept parameter: the selector's choice, or the first
// non-dt parameter (dt sweeps are integrator artifacts, not bifurcations).
func (b *bifurcation) param() (paramDef, int, bool) {
	ps := b.params()
	if len(ps) == 0 {
		return paramDef{}, -1, false
	}
	if b.paramIdx >= 0 && b.paramIdx < len(ps) {
		return ps[b.paramIdx], b.paramIdx, true
	}
	for i := range ps {
		if ps[i].Label != "dt" {
			return ps[i], i, true
		}
	}
	return ps[0], 0, true
}

func (b *bifurcation) generateBifurcation() {
	p, pidx, ok := b.param()
	if !ok {
		return
	}
	if b.sig != b.lastFlowMode+"|"+strconv.Itoa(pidx) {
		b.fitDone = false // a new diagram is framed anew
	}
	if !b.Fill(b.lastFlowMode, pidx, p.Value, p.Min, p.Max, 3) {
		return
	}
	vertices := sim.vertBuf[:min(len(b.colOf), cap(sim.vertBuf)/4)*4]
	n := b.Points(len(vertices)/4, func(i int, x, y, frac float32) {
		k := i * 4
		vertices[k], vertices[k+1], vertices[k+2] = x, y, 0
		vertices[k+3] = frac // gradient follows the sweep
	})
	if n == 0 {
		gpu.uploadVerticesOnly(sim.vertBuf[:0], glctx.Types.Points, 0)
		// The readout still tells the truth about the cursor here. There is
		// nothing to point AT for the first frames of a sweep, but "mod off"
		// with Audio mod on would be a lie about the audio rather than a
		// statement about the diagram.
		cv, cok := b.cursorValue(p)
		b.showCursor(b.cursorReadout(p, cv, cok))
		return
	}
	gpu.uploadVerticesOnly(vertices[:n*4], glctx.Types.Points, n)
	b.drawCursor(p, b.max-b.min)
	if !b.fitDone && b.Started() {
		b.fitDone = true
		view.autoFitCamera()
	}
}

// ── The audio cursor ─────────────────────────────────────────────────────
//
// The diagram above is untouched by any of this. What follows only points at
// it — see bifdrive.go for why that is the whole design and not a compromise.

// bifCursorRulePts is how many points the vertical rule is drawn with. Points
// and not a line strip because the diagram is a single POINTS draw and this
// rides the same pipeline; a strip would be a second draw call and a second
// pass through uploadVerticesOnly's in-place center subtraction, which is
// the reasoning the Poincaré return map's y=x diagonal already settled.
const bifCursorRulePts = 96

// bifCursorBand is how many columns either side of the cursor's own are lit
// up with it. One: at 360 columns across the screen a single column is a
// hairline, and the point of the highlight is to show the attractor's slice AT
// this parameter — widening it further would start mixing in structure from
// parameter values the music is not at.
const bifCursorBand = 1

// bifEnvelope is the audio envelope driving the cursor, and whether there is
// one to read.
//
// af.feat["amp"] is the existing mono envelope: the RMS of the current window,
// adaptively normalized and smoothed with a fast attack and a slow release. It
// is deliberately reused rather than measured again here — a second envelope
// with its own normalizer would drift away from the one every other
// audio-reactive control in the rack follows, and the cursor would disagree
// with the meters about how loud the music is.
//
// It is only computed while Audio mod is on, which is what the second return
// reports. Saying "mod off" in the readout is better than a cursor parked at
// zero, which is indistinguishable from silence.
func bifEnvelope() (float32, bool) {
	if !audioMod {
		return 0, false
	}
	return clamp01(af.feat["amp"]), true
}

// cursorValue is where the audio currently puts the swept parameter.
func (b *bifurcation) cursorValue(p paramDef) (float32, bool) {
	if !b.driveAudio {
		return 0, false
	}
	env, ok := bifEnvelope()
	if !ok {
		return 0, false
	}
	return bifAudioValue(env, b.depth, *p.Value, p.Min, p.Max), true
}

// drawCursor lights the diagram's own points at the cursor's parameter and
// draws a rule through them, in gold via the monochrome override.
//
// span is the diagram's current y range, passed in rather than recomputed so
// the rule spans exactly what the scatter spans.
func (b *bifurcation) drawCursor(p paramDef, span float64) {
	v, ok := b.cursorValue(p)
	b.showCursor(b.cursorReadout(p, v, ok))
	if !ok {
		return
	}
	need := (bifCursorRulePts + bifPerCol*(2*bifCursorBand+1)) * 4
	if cap(b.curBuf) < need {
		b.curBuf = make([]float32, need)
	}
	buf := b.curBuf[:0]
	fx := bifFrac(v, p.Min, p.Max)
	x := (fx*2 - 1) * 20
	for i := range bifCursorRulePts {
		fy := float32(i) / float32(bifCursorRulePts-1)
		buf = append(buf, x, (fy*2-1)*12, 0, fy)
	}
	// The diagram's own points in the cursor's column band. These are the
	// reading: the set of values the attractor settles onto at the parameter
	// the music is currently at.
	col := int32(bifColumnFor(v, p.Min, p.Max, bifCols)) //nolint:gosec // a column index into the bifurcation image
	for i := range b.colOf {
		if b.colOf[i] < col-bifCursorBand || b.colOf[i] > col+bifCursorBand {
			continue
		}
		if len(buf)+4 > cap(b.curBuf) {
			break
		}
		cx := float32(b.colOf[i]) / float32(bifCols-1)
		cy := float32((b.valOf[i] - b.min) / span)
		buf = append(buf, (cx*2-1)*20, (cy*2-1)*12, 0, cy)
	}
	// Gold, and put BOTH uniforms back afterwards. renderFrame re-uploads the
	// gradient's source every frame but not uBaseColor, so an override left
	// set here would tint the next mode's trail until something touched a
	// color knob — the bug the Poincaré overlay had and the reason its restore
	// looks like this one.
	glctx.GL.Call("uniform1i", gpu.u.gradientColors, 1)
	glctx.GL.Call("uniform3f", gpu.u.baseColor, 1.0, 0.8, 0.15)
	gpu.uploadVerticesOnly(buf, glctx.Types.Points, len(buf)/4)
	restoreTraceColor()
}

// cursorReadout is the LED text: the parameter value the cursor is at, or
// why there is no cursor. Naming the reason matters here — "audio" selected
// with Audio mod off looks exactly like a broken feature otherwise.
func (b *bifurcation) cursorReadout(p paramDef, v float32, ok bool) string {
	if !b.driveAudio {
		return "sweep"
	}
	if !ok {
		return "mod off"
	}
	return p.Label + " " + strconv.FormatFloat(float64(v), 'f', 2, 32)
}

// showCursor writes the readout, and only when it changes — the value moves
// with every beat and the DOM does not need sixty writes a second of it. The
// rule stereoInst.showReadout keeps, for the same reason.
func (b *bifurcation) showCursor(s string) {
	if s == b.curText {
		return
	}
	b.curText = s
	if b.curEl.Truthy() {
		setDotText(b.curEl, s)
	}
}

// wireBifCells wires SWEEP and DRIVE, bifurcation's two positions in the
// Visual bank (the cells in the #model-parts holder, marked data-bank-of).
// DEPTH is a parameter of the model (the init below), so the bank builds its
// knob as it does any other; it is dark while DRIVE is on sweep, where it
// moves nothing.
func (b *bifurcation) wireBifCells() {
	if sel := dom.Doc.Call("getElementById", "bif-sweep"); sel.Truthy() {
		// A control of the rack, so a rack without the page can choose what is
		// swept: its options are the last flow's parameters, by index.
		registerControl(ControlDesc{ID: "bif-sweep", Label: "sweep", IsSelect: true, SelectDef: "1"}, moduleOfControl("bif-sweep"))
		dom.On(sel, "change", func(js.Value, []js.Value) any {
			if b.sweepFilling {
				return nil
			}
			if v, err := strconv.Atoi(sel.Get("value").String()); err == nil {
				b.paramIdx = v
				b.invalidate()
			}
			return nil
		})
	}
	adoptDescControl(ControlDesc{
		ID: "bif-drive", Label: "drive", IsSelect: true, SelectDef: "0", ResetID: "rst-bif-drive",
		SelectApply: func(v string) {
			b.driveAudio = v == "1"
			syncAudioMod()
			b.syncDepthCell()
		},
	})
	b.syncDepthCell()
}

// syncSweepCell fills SWEEP with the parameters of the system being swept —
// the most recent flow mode, which is why its options are not fixed and its
// ring prints none of them (ledSelector). Called as the mode comes up.
func (b *bifurcation) syncSweepCell() {
	sel := dom.Doc.Call("getElementById", "bif-sweep")
	if !sel.Truthy() {
		return
	}
	_, cur, _ := b.param()
	b.sweepFilling = true
	sel.Set("innerHTML", "")
	for i, pd := range b.params() {
		opt := dom.Doc.Call("createElement", "option")
		opt.Set("value", strconv.Itoa(i))
		opt.Set("textContent", pd.Label)
		sel.Call("appendChild", opt)
	}
	sel.Set("value", strconv.Itoa(cur))
	// Its ring and its display follow the change event; this handler skips
	// it, because nothing about the sweep has changed.
	dom.Fire(sel, "change")
	b.sweepFilling = false
	if cell := sel.Call("closest", ".punit"); cell.Truthy() {
		cell.Set("title", docf("bif-sweep-cell.live", "mode", modeInfo[b.lastFlowMode].Label))
	}
}

// syncDepthCell blanks DEPTH's readout while the audio drive is off.
func (b *bifurcation) syncDepthCell() {
	cs := dom.Doc.Call("querySelectorAll", `[data-param="bif-depth"]`)
	for i := range cs.Length() {
		setCellBlank(cs.Index(i), !b.driveAudio)
	}
}

// appendCursorReadout puts where the audio drive has the swept parameter on
// the model's readout line (liveReadoutHost). It also says why there is no
// cursor when there is not, in words, so it is a character display.
func (b *bifurcation) appendCursorReadout(host js.Value) {
	// Cleared so the next frame writes into the NEW element: the panel is
	// rebuilt on every mode change and this guard would otherwise skip the
	// fresh cell as unchanged and leave it blank.
	b.curText = ""
	b.curEl = liveReadout(host, "cur", dispFullChars, "sweep",
		doc("ro.cur"))
}

func init() {
	registerGenerate("bifurcation", bif.generateBifurcation)
}
