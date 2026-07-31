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

import (
	"strconv"
	"syscall/js"
)

const (
	bifCols   = 360 // sweep resolution (columns)
	bifPerCol = 40  // maxima kept per column
)

var (
	lastFlowMode = "lorenz" // most recent mode with a registered flow
	bifParamIdx  = -1       // index into attractorParams[lastFlowMode]; -1 = auto
	bifSig       string     // source|param sig the data was computed for
	bifNextCol   int
	bifColOf     []int32   // column index per stored point
	bifValOf     []float64 // z-maximum per stored point
	bifMin       = 1e30
	bifMax       = -1e30
	bifFitDone   bool
)

func bifInvalidate() { bifSig = "" }

// bifParams returns the sweepable parameter list of the source mode.
func bifParams() []paramDef { return attractorParams[lastFlowMode] }

// bifParam picks the swept parameter: the selector's choice, or the first
// non-dt parameter (dt sweeps are integrator artifacts, not bifurcations).
func bifParam() (paramDef, int, bool) {
	ps := bifParams()
	if len(ps) == 0 {
		return paramDef{}, -1, false
	}
	if bifParamIdx >= 0 && bifParamIdx < len(ps) {
		return ps[bifParamIdx], bifParamIdx, true
	}
	for i := range ps {
		if ps[i].Label != "dt" {
			return ps[i], i, true
		}
	}
	return ps[0], 0, true
}

func generateBifurcation() {
	sys, ok := flowFor4(lastFlowMode)
	if !ok {
		return
	}
	p, pidx, ok := bifParam()
	if !ok {
		return
	}
	sig := lastFlowMode + "|" + strconv.Itoa(pidx)
	if bifSig != sig {
		bifSig = sig
		bifNextCol = 0
		bifColOf = bifColOf[:0]
		bifValOf = bifValOf[:0]
		bifMin, bifMax = 1e30, -1e30
		bifFitDone = false
	}

	// Progressive sweep: a few columns per frame.
	cols := 3
	if sys.interpreted {
		cols = 1
	}
	dt := sys.dt()
	ic := initCondFor(lastFlowMode)
	for c := 0; c < cols && bifNextCol < bifCols; c++ {
		j := bifNextCol
		bifNextCol++
		pv := p.Min + (p.Max-p.Min)*float32(j)/float32(bifCols-1)
		saved := *p.Value
		*p.Value = pv
		s := [4]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), sys.w()}
		const transient = 1500
		for i := 0; i < transient; i++ {
			twinStep(sys, &s, dt)
			if twinDiverged(s) {
				break
			}
		}
		z2, z1 := s[2], s[2]
		got := 0
		for i := 0; i < 6000 && got < bifPerCol; i++ {
			twinStep(sys, &s, dt)
			if twinDiverged(s) {
				break
			}
			z0 := s[2]
			if z1 > z2 && z1 >= z0 { // local maximum at z1
				bifColOf = append(bifColOf, int32(j))
				bifValOf = append(bifValOf, z1)
				if z1 < bifMin {
					bifMin = z1
				}
				if z1 > bifMax {
					bifMax = z1
				}
				got++
			}
			z2, z1 = z1, z0
		}
		*p.Value = saved
	}

	// Render the whole diagram from the raw (col, value) pairs each frame —
	// the y normalization tracks the running min/max, so early columns stay
	// correctly placed as later ones widen the range.
	n := len(bifColOf)
	if n < 2 || bifMax <= bifMin {
		uploadVerticesOnly(vertBuf[:0], glTypes.Points, 0)
		return
	}
	if cap(vertBuf) < n*4 {
		n = cap(vertBuf) / 4
	}
	span := bifMax - bifMin
	vertices := vertBuf[:n*4]
	for i := 0; i < n; i++ {
		fx := float32(bifColOf[i]) / float32(bifCols-1)
		fy := float32((bifValOf[i] - bifMin) / span)
		k := i * 4
		vertices[k] = (fx*2 - 1) * 20
		vertices[k+1] = (fy*2 - 1) * 12
		vertices[k+2] = 0
		vertices[k+3] = fx // gradient follows the sweep
	}
	uploadVerticesOnly(vertices, glTypes.Points, n)
	if !bifFitDone && bifNextCol >= bifCols/4 {
		bifFitDone = true
		autoFitCamera()
	}
}

// buildBifPanel fills the Parameters module for bifurcation mode: the source
// system, the swept-parameter selector, and progress.
func buildBifPanel(paramsDiv js.Value) {
	col := doc.Call("createElement", "span")
	col.Set("className", "pcell")

	src := doc.Call("createElement", "span")
	src.Set("className", "plabel")
	src.Set("textContent", "SWEEP "+modeInfo[lastFlowMode].Label)
	src.Set("title", "The system being swept — the most recent flow mode. Switch to an attractor, tune it, then come back.")
	col.Call("appendChild", src)

	sel := doc.Call("createElement", "select")
	sel.Set("title", "Swept parameter — the x axis of the diagram; each column integrates the system fresh at that value and plots the maxima of z")
	sel.Set("style", "background:#222;color:#ccc;border:1px solid #555;font-family:monospace;font-size:12px;padding:2px 4px;")
	_, cur, _ := bifParam()
	for i, pd := range bifParams() {
		opt := doc.Call("createElement", "option")
		opt.Set("value", strconv.Itoa(i))
		opt.Set("textContent", pd.Label)
		if i == cur {
			opt.Set("selected", true)
		}
		sel.Call("appendChild", opt)
	}
	sel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if v, err := strconv.Atoi(sel.Get("value").String()); err == nil {
			bifParamIdx = v
			bifInvalidate()
		}
		return nil
	}))
	grp := doc.Call("createElement", "span")
	grp.Set("className", "grp")
	grp.Call("appendChild", sel)
	col.Call("appendChild", grp)
	paramsDiv.Call("appendChild", col)
}

func init() {
	registerGenerate("bifurcation", generateBifurcation)
	attractorDescriptions["bifurcation"] = "Bifurcation Explorer — the fig-tree diagram, computed " +
		"live. One parameter of the most recent flow mode sweeps its whole knob range across the x " +
		"axis; each column integrates the system fresh at that value and plots the local maxima of " +
		"z. Thin branches are periodic orbits, fan-outs are period-doubling cascades, filled bands " +
		"are chaos — the route between them is the route to chaos. Pick the swept parameter in the " +
		"Parameters module; visit an attractor and tune it to change the source system."
}
