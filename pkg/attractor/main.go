//go:build js && wasm

package attractor

import (
	"fmt"
	"math"
	"runtime"
	"strconv"
	"syscall/js"
	"time"

	"github.com/0magnet/chaosrack/pkg/colorspace"
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"

	"github.com/0magnet/rack-go"
	"github.com/go-gl/mathgl/mgl32"
)

// Run boots the panel: build it, wire every control, start the loop.
//
// It was sixteen hundred lines at a cyclomatic complexity of 146 — the
// largest piece of debt in the Go here, grown honestly by wiring each
// control inline next to the last. Twenty blocks have come out and it is
// 487 lines at 56.
//
// They were found rather than chosen. Track brace depth through the body,
// then take the runs where every mention of one of Run's locals is also a
// declaration of it inside that run, bounded by statement boundaries and
// containing no return that would exit Run rather than the new function.
// Thirteen used no locals at all; the rest own theirs. buildPanelKnobs is
// the one that mattered — six closures declared once and used by every axis
// and every slider, private to that block all along, with Run for a scope.
//
// That is as far as cutting goes: the analysis now finds exactly one run
// left, and it is the guard at the top that returns out of Run. The 56 that
// remains is spread thin across statements sharing locals that are read
// hundreds of lines apart — panel, shell, hash, footers — and those come
// down by giving the boot a struct to hang its state on, which is a design
// and not a cut.
//
//nolint:gocyclo // 56, down from 146; see above for what the rest needs
func Run() {
	// Lazy WebGL init — see initWebGL doc. Must run after the host
	// DOM is ready (caller's responsibility); otherwise gocanvas
	// won't exist yet and canvasEl.Call("getContext", ...) panics.
	gpu.initWebGL()
	if dom.Body.IsUndefined() || dom.Body.IsNull() {
		js.Global().Call("alert", "cannot get html body, exiting")
		return
	}
	if glctx.Canvas.IsUndefined() || glctx.Canvas.IsNull() {
		js.Global().Call("alert", "cannot find #gocanvas, exiting")
		return
	}
	installErrorNet()

	// Build controls panel. If the host page already has a <footer>
	// (e.g. m2/magnetosphere.net puts cart + shipping nav there), append
	// the controls inline into it so the existing footer nav stays
	// accessible. Otherwise fall back to a fixed-bottom overlay for
	// standalone use.
	// The panel lives inside a shell, and the shell is what gets positioned.
	//
	// The panel scrolls, so anything placed inside it is clipped by that
	// scrolling — which is why the dock buttons and the resize bar used to be
	// position:fixed siblings with their coordinates recomputed in Go every
	// time the panel moved. The shell is a positioning box that does not
	// scroll, so those two are ordinary absolutely-positioned children of it
	// and CSS puts them where the dock edge says. Nothing has to follow
	// anything.
	//
	// It also gives the panel one thing to move rather than several: whatever
	// ends up owning the geometry — an edge dock, a floating window, a host
	// page's footer — moves the shell, and the furniture goes with it.
	shell := dom.Doc.Call("createElement", "div")
	shell.Set("id", "panel-shell")
	panel := dom.Doc.Call("createElement", "div")
	panel.Set("id", "controls-panel")
	// The panel's stylesheet goes in the HEAD, not in the panel.
	//
	// It used to be a <style> prepended to the panel's own innerHTML, which
	// works in every browser and is invalid: style is metadata content, and
	// metadata content belongs in the head. It rendered, so nothing said so
	// until the page was put through a validator.
	//
	// Where it sits never scoped it — scoped was dropped from the spec, and
	// these rules always applied to the whole document — but WHEN it is
	// added decides the ties. rack-go's sheet and this one both carry
	// single-class rules for the same element, .rack against .runit-open,
	// and equal specificity is settled by source order alone. Sitting in
	// the body made this sheet last by accident; moving it to the head made
	// it first, and a bay went from nowrap to wrap and doubled in height.
	// So rack-go's goes in explicitly, ahead of it. It still has to come
	// last, and now it says so rather than happening to.
	rack.InjectCSS()
	css := dom.Doc.Call("createElement", "style")
	css.Set("id", "chaosrack-panel-css")
	css.Set("textContent", panelCSS)
	dom.Doc.Get("head").Call("appendChild", css)
	panel.Set("innerHTML", withRackScopes(withDocs(controlsBody)))
	buildModeSelect(panel) // the mode <select> derives from the mode registry
	footers := dom.Doc.Call("getElementsByTagName", "footer")
	var existingFooter js.Value
	if footers.Get("length").Int() > 0 {
		existingFooter = footers.Index(0)
	}
	// ForceStandalonePanel lets a host page with a <footer> opt into the
	// full standalone chrome (dock/resize/float) instead of the inline
	// row that a host <footer> otherwise triggers. See its doc comment.
	// The full dock/resize/float chrome is wired in EVERY mode; a host page
	// with a <footer> just gets one extra dock target — "footer", which
	// appends the panel inline below the footer's own content (cart links
	// stay clickable) — and boots docked there. The ▣ button returns to it
	// after re-docking or floating.
	layout.hostFooter = existingFooter
	layout.standalone = true
	shell.Call("appendChild", panel)
	// Parsed into a scratch element and moved across, NOT assigned to the
	// shell's innerHTML: the panel is already built and wired by this point,
	// and re-serializing the shell would hand back a fresh copy of it with
	// every listener and every knob dropped.
	{
		tmp := dom.Doc.Call("createElement", "div")
		tmp.Set("innerHTML", withDocs(shellFurniture))
		for tmp.Get("firstChild").Truthy() {
			shell.Call("appendChild", tmp.Get("firstChild"))
		}
	}
	dom.Body.Call("appendChild", shell)
	layout.wireDockButtons()
	layout.initDockResize()
	if layout.hostFooter.Truthy() && !ForceStandalonePanel {
		layout.applyDock("footer")
	} else {
		layout.applyDock(layout.readDockPref())
	}
	// Initial panel visibility: ?panel= query param wins over the Go var
	// so a shareable URL can invite users to open (or close) the panel.
	// Hidden state leaves only the ▤ toggle button in the bottom-left;
	// clicking it restores the panel via the normal show/hide path.
	switch queryParam("panel") {
	case "hidden":
		panel.Get("style").Set("display", "none")
	case "shown", "visible":
		// explicit show — leave alone
	default:
		if PanelStartHidden {
			panel.Get("style").Set("display", "none")
		}
	}

	dom.Init() // refresh the handles; the host's document may have changed
	bootMark("fonts")
	injectFonts() // embedded @font-face rules for the panel / LED / header fonts
	cacheElementRefs()
	// obeys the speed". Faithful to Glen's 3D projective unit, whose panel
	bootMark("knobs-start")
	buildPanelKnobs()
	// Floating show/hide button for the whole control panel (it can block the
	// view). Lives outside the panel so it can bring it back.
	panelToggle := dom.Doc.Call("createElement", "button")
	panelToggle.Set("id", "panel-toggle")
	panelToggle.Set("textContent", "▤")
	panelToggle.Set("title", doc("panel-toggle"))
	panelToggle.Set("style", "position:fixed;bottom:6px;left:6px;z-index:var(--z-toggle);background:#222;color:#ccc;border:1px solid #555;border-radius:3px;font-family:'B612 Mono',monospace;font-size:14px;cursor:pointer;padding:2px 8px;opacity:0.55;")
	dom.On(panelToggle, "click", func(this js.Value, args []js.Value) any {
		p := dom.Doc.Call("getElementById", "controls-panel")
		if !p.Truthy() {
			return nil
		}
		st := p.Get("style")
		hidden := st.Get("display").String() == "none"
		// The SHELL counts as hidden too, and it is a different element. Closing
		// the rack under Contain leaves the panel's own display alone and hides
		// the shell around it, so a button that asked only the panel thought the
		// controls were on screen, hid them again, and had to be pressed twice
		// to give back something that was never visible in between.
		if sh := dom.Doc.Call("getElementById", "panel-shell"); sh.Truthy() &&
			sh.Get("style").Get("display").String() == "none" {
			hidden = true
		}
		// The recovery-raise is ONE bit: body.panel-raised. CSS lifts the panel
		// AND its chrome (resize strip, float grip, dock buttons) together, so
		// the whole recovery unit surfaces above the Front canvas as a piece.
		cl := dom.Body.Get("classList")
		raised := cl.Call("contains", "panel-raised").Bool()
		// Is any of the model drawn over the panel? It used to be a switch, so
		// this was a checkbox read; with the Fore knob it is anything but the
		// far end, since even a sliver in front can bury the controls.
		frontOn := splitFrac > -1+splitEpsilon
		// Recover whenever the panel is hidden OR the "Front" canvas is drawn over
		// it — otherwise an opaque backdrop with Front on buries the panel and
		// toggling display alone can never bring it back. Raise it above the Front
		// canvas so one press always restores it; otherwise hide it so the model
		// can be viewed unobstructed.
		if hidden || (frontOn && !raised) {
			st.Set("display", "")
			cl.Call("add", "panel-raised")
			// With Contain on, the desk IS the environment, so the button
			// brings the environment back with the controls — see
			// deskShell_js.go. Without it the desk is unaffected.
			if deskContainHidesEverything() {
				deskLayerVisible(true)
			}
			// A rack that was CLOSED has no window to show, and showing the
			// shell alone shows nothing: the close handler leaves the shell
			// hidden in the body. Rebuild the window instead, so this button
			// means "give me the controls back" in every state rather than in
			// most of them — found by a monkey run that closed the rack under
			// Contain and then could not get it back, which is the next thing
			// a person would try.
			if deskContain && panelWindow == nil {
				relaunchRack()
			} else if sh := dom.Doc.Call("getElementById", "panel-shell"); sh.Truthy() {
				sh.Get("style").Set("display", "")
			}
			// First show after a hidden boot: module widths were never
			// measurable, so quantize now and refit the panel chrome.
			quantizeModuleWidths()
			// positionDockControls is gone: the cluster is placed by CSS off the
			// shell edge and needs no refitting.
			layout.positionResizeHandle()
		} else {
			st.Set("display", "none")
			cl.Call("remove", "panel-raised")
			// Hiding "the controls" while the desk is the environment means
			// hiding the environment: the panel and the windows, leaving the
			// model. A desktop with a panel and no application is not what
			// anybody means by that button.
			if deskContainHidesEverything() {
				deskLayerVisible(false)
			}
		}
		return nil
	})
	dom.Body.Call("appendChild", panelToggle)

	// The Style module's knobs: the knob face, the LED color and the
	// phosphor. The interface size has no knob of its own: dragging the
	// panel's edge scales it continuously (layout.setKScale), which a
	// four-step ring only repeated, and the rack's metalwork is the one style.
	{
		// The knob face, on a lone labeled ring.
		if st := dom.Doc.Call("getElementById", "knob-style"); st.Truthy() {
			if kh := dom.Doc.Call("getElementById", "knobstyle-stack"); kh.Truthy() {
				kh.Call("appendChild", singleSelectorKnob(st, []string{"std", "flat", "vint", "chrm", "gold", "carb"}))
			}
			applyStyle := func() {
				cl := dom.Doc.Call("getElementById", "controls-panel").Get("classList")
				for _, s := range []string{"ks-std", "ks-flat", "ks-vint", "ks-chrome", "ks-gold", "ks-carbon"} {
					cl.Call("remove", s)
				}
				cl.Call("add", "ks-"+st.Get("value").String())
			}
			dom.On(st, "change", func(this js.Value, args []js.Value) any {
				applyStyle()
				return nil
			})
			applyStyle()
			st.Get("style").Set("display", "none")
			// No PermaKey: neither the face nor the LED color is carried by a
			// link today, and adding hash keys is a separate decision.
			adoptDescControl(ControlDesc{
				ID: "knob-style", Label: "Knob", IsSelect: true, SelectDef: "std", ResetID: "rst-knob-style",
			})
		}
		// The LED color, in a cell of its own: a ring of colored dots, one per
		// option in its own color, and the color's name on a character display
		// under the knob, the way Phosphor names its setting.
		if lc := dom.Doc.Call("getElementById", "led-color"); lc.Truthy() {
			// Populate the LED-color options from the single ordered source.
			var dotCols []string
			ledCols := map[string][4]string{}
			for _, d := range ledColorDefs {
				o := dom.Doc.Call("createElement", "option")
				o.Set("value", d.name)
				o.Set("textContent", d.name)
				o.Set("title", d.desc)
				lc.Call("appendChild", o)
				dotCols = append(dotCols, d.col)
				ledCols[d.name] = [4]string{d.col, d.glow, d.bg, d.bd}
			}
			if lh := dom.Doc.Call("getElementById", "ledcolor-stack"); lh.Truthy() {
				ro := selectorKnobReadout(lc)
				addSelectorDotLabels(ro.Call("querySelector", ".knobstack"), dotCols, lc, 44)
				lh.Call("appendChild", ro)
			}
			// On the page, not the panel: the windows outside it (the Info
			// window's addresses) read in the LED color too.
			applyLED := func() {
				c := ledCols[lc.Get("value").String()]
				s := dom.Doc.Get("documentElement").Get("style")
				s.Call("setProperty", "--led-col", c[0])
				s.Call("setProperty", "--led-glow", c[1])
				s.Call("setProperty", "--led-bg", c[2])
				s.Call("setProperty", "--led-bd", c[3])
			}
			dom.On(lc, "change", func(this js.Value, args []js.Value) any { applyLED(); repaintTurning(); return nil })
			applyLED()
			lc.Get("style").Set("display", "none")
			// The default is the first entry of the one ordered table the
			// options were built from, rather than a color name written out
			// again here — the list is allowed to be reordered.
			adoptDescControl(ControlDesc{
				ID: "led-color", Label: "LED", IsSelect: true, SelectDef: ledColorDefs[0].name,
				ResetID: "rst-led-color",
			})
		}
		// Restore a saved (continuous) interface scale from a prior
		// resize-drag; otherwise the standard size.
		kscale := 1.0
		if s, ok := lsGet("wasmstuff-kscale"); ok {
			if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
				kscale = v
			}
		}
		layout.setKScale(kscale)
	}
	bootMark("knobs-done")
	// From here to the permalink, every handler that would re-quantize the
	// rack or rebuild the panel asks once and is answered once, at the end:
	// each quantize writes the rack and reads it back, which is a full layout
	// of the page, and the boot asked for sixteen of them in a row. With no
	// mode, because the hash has not been read yet: the flush uses the one
	// the boot ends on.
	owed.withDeferredLayout("", func() {
		wireModeAndResetInputs()
		if dst := dom.Doc.Call("getElementById", "desk-style"); dst.Truthy() {
			// A position in the Visual bank (modelparts_js.go), where the display
			// over its knob names the style. Through the registry, which is what
			// gives the cell its reset button.
			// deskFlat is the default the package variable already holds, so the
			// reset target is that one constant rather than a second copy of it.
			adoptDescControl(ControlDesc{
				ID: "desk-style", Label: "style", IsSelect: true, SelectDef: deskFlat,
				ResetID:     "rst-desk-style",
				SelectApply: desks.setDeskStyle,
			})
		}
		// Bifurcation's and the wobbulator's bank positions, from the same holder.
		bif.wireBifCells()
		fvf.wireFVFCells()
		syncFVFRoute()
		wireColorAndViewControls()
		// Event: physics switch — the weight is a switch on the Motion panel, not a
		wirePhysSwitch()
		// canvas, which is what the GPU costs, but keeps the control panel — so
		wireFullscreenSwitch()
		// Initial mode — read from URL hash if present. The hash may carry
		// permalink state after the mode ("#aizawa&p.a=1.19&..."); take only
		// the leading mode token here (the rest is applied post-setup).
		run.selectedMode = "globe"
		// A link to a model folded into another (#cube, #sphere) is rewritten as
		// what it is now, before the hash is read by anything.
		migrateLocationHash()
		hash := js.Global().Get("location").Get("hash").String()
		if len(hash) > 1 {
			hashMode := hashModeToken()
			// Validate against the mode registry. (The old params-map + hardcoded
			// list pair silently rejected several real modes, e.g. sphere and fvf.)
			if knownMode(hashMode) {
				run.selectedMode = hashMode
			}
		}
		// Select the matching dropdown option
		sel := dom.Doc.Call("getElementById", "mode-select")
		if !sel.IsNull() && !sel.IsUndefined() {
			sel.Set("value", run.selectedMode)
		}
		bootMark("params")
		// Built now, though the boot is deferring layout: the rest of the boot is
		// wired against it.
		buildParamPanelNow(run.selectedMode)
		updateTrailVisibility()

		// One-time drag listeners for every selector knob in the panel, the
		wireGradientKnobs()
		// first pass can run before the panel's own font has been applied.
		bootMark("quantize")
		requantizeAfterFonts()

		// Initialize persistent JS typed arrays for zero-alloc frame uploads
		gpu.vertU8 = js.Global().Get("Uint8Array").New(sim.steps * 4 * 4)
		buf := gpu.vertU8.Get("buffer")
		gpu.vertF32 = js.Global().Get("Float32Array").New(buf, 0, sim.steps*4)
		initDrawState()
		debugVal := js.Global().Get("__WASM_DEBUG__")
		if !debugVal.IsUndefined() && debugVal.Bool() {
			debugEnabled = true
		}

		// Registry-owned fixed controls (registry refactor). One ControlDesc per
		// control owns the LED format, slider→state plumbing, typed entry,
		// wheel-step, reset button, AND Reset All (via the builtControls loop in
		// onResetAll) — previously each of those was a separate wiring site and
		// Reset All silently missed pan-x/pan-y/period. Registered BEFORE the
		// permalink restore below so a restored hash value drives Apply like any
		registerViewControls()
		adoptDescControl(ControlDesc{ID: "trail-slider", Label: "Trail", Min: 1000, Max: 500000, Step: 1000, Def: 20000,
			PermaKey: "tr", LEDID: "slider-value-trail", ResetID: "rst-trail",
			Apply: func(v float64) {
				newSteps := int(v)
				if newSteps != sim.steps {
					sim.steps = newSteps
					sim.vertBuf = make([]float32, sim.steps*4)
					gpu.vertU8 = js.Global().Get("Uint8Array").New(sim.steps * 4 * 4)
					buf := gpu.vertU8.Get("buffer")
					gpu.vertF32 = js.Global().Get("Float32Array").New(buf, 0, sim.steps*4)
					resetAttractorState()
					refreshGradient()
				}
			},
			// Persist is a switch of its own: resetting the trail's length
			// leaves it as it is.
		})
		registerOutputControls()
		// value so engine state matches the panel by construction (the old code
		bootMark("commit")
		commitBuiltControls()
		// encoded in the URL hash, then keep the hash in sync with the live
		bootMark("permalink")
		capturePermalinkAndRestore()
	})
	done := make(chan struct{})
	renderFrame = dom.FuncOf(renderLoop)
	js.Global().Call("requestAnimationFrame", renderFrame)

	// Set initial trail-controls visibility for the starting mode.
	updateTrailVisibility()

	// And the Physics switch, for the same reason and with the same timing.
	// The earlier call (beside the switch's own listener) runs before the URL
	// hash has chosen the mode, so it decided against the DEFAULT mode: booting
	wireExtraNav()
	// Kill the vertical scroll the controls panel adds by growing the body.
	applyHostPageTweaks()
	// had to skip doing so when the hash pinned a Y rate — because the
	bootMark("bg-start")
	startBackgroundTasks()
	bootMark("run-end")
	<-done
}

func postDebugStats() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	avgMs := float32(0)
	fps := float32(0)
	if fstats.count > 0 {
		avgMs = fstats.totalMs / float32(fstats.count)
		fps = 1000.0 / avgMs
	}

	payload := fmt.Sprintf(
		`{"mode":%q,"paused":%t,"fps":%.1f,"frame_avg_ms":%.2f,"frame_min_ms":%.2f,"frame_max_ms":%.2f,"frame_count":%d,"speed_steps":%d,"speed_scale":%.4f,"trail_steps":%d,"heap_alloc_mb":%.2f,"heap_sys_mb":%.2f,"heap_objects":%d,"gc_runs":%d,"goroutines":%d}`,
		run.selectedMode, run.paused, fps, avgMs, fstats.minMs, fstats.maxMs, fstats.count,
		sim.speedSteps, sim.speedScale, sim.steps,
		float64(ms.HeapAlloc)/1048576, float64(ms.HeapSys)/1048576,
		ms.HeapObjects, gcRunsCount(&ms), runtime.NumGoroutine(),
	)

	// Reset frame stats for next interval
	fstats.count = 0
	fstats.totalMs = 0
	fstats.minMs = 999
	fstats.maxMs = 0

	// Post via fetch
	headers := js.Global().Get("Headers").New()
	headers.Call("set", "Content-Type", "application/json")
	opts := js.Global().Get("Object").New()
	opts.Set("method", "POST")
	opts.Set("headers", headers)
	opts.Set("body", payload)
	js.Global().Call("fetch", "/debug/stats", opts)
}

// Per-attractor initial conditions — defaults to (0.1, 0.5, -0.6) for most.
func installErrorNet() {
	dom.On(js.Global(), "unhandledrejection", func(this js.Value, a []js.Value) any {
		if len(a) > 0 {
			js.Global().Get("console").Call("warn", "[async rejection contained]", a[0].Get("reason"))
			a[0].Call("preventDefault")
		}
		return nil
	})
}

func onResetAll(this js.Value, args []js.Value) any {
	// Reset camera
	view.defaultDist = view.initDist

	// Static geometry may need re-upload (params reset to defaults).
	gpu.staticDirty = true

	// Reset attractor position
	resetAttractorState()

	// Every modulation route, with no switch to silence them any more: a
	// recall goes through here, and a recalled patch must not keep the routes
	// it replaced.
	clear(pmod.params)
	syncAudioMod()
	// And the Mixer, for the same reason: a recalled patch's pins replace
	// the ones before, they do not add to them.
	mixReset()

	// Registry-owned controls (zoom, pan X/Y, rainbow period, …): each resets
	// itself — value, LED format, and any reset hook — so Reset All can never
	// silently miss one again. (Rotation sliders + view.modelMat are re-randomized
	// below so the model never lands on the same view twice.)
	for _, c := range builtControls {
		if c.skipResetAll {
			continue
		}
		c.resetToDefault()
	}

	// Reset all parameters to defaults
	for _, params := range attractorParams {
		for _, p := range params {
			*p.Value = p.Def
		}
	}
	buildParamPanel(run.selectedMode)

	// Reset auto-rotate, draw mode. (Speed / line width / trail — values,
	// LEDs, buffer realloc, persist drop — are registry-owned above.)
	run.paused = false
	if ps := dom.Doc.Call("getElementById", "pause-sw"); ps.Truthy() {
		ps.Set("checked", false)
	}
	style.usePoints = false
	gpu.drawMode = glctx.Types.LineStrip
	view.ball.orient = mgl32.Ident4() // clear trackball drag orientation
	dom.Doc.Call("getElementById", "auto-rotate").Set("checked", true)
	dom.Doc.Call("getElementById", "use-points").Set("checked", false)
	dom.Doc.Call("getElementById", "show-info").Set("checked", false)
	info.hideInfoWindow()
	style.persistTrail = false
	dom.Doc.Call("getElementById", "persist-trail").Set("checked", false)
	// The source and map rings are registry-owned, so the loop above has already
	// put them back — including gradientSource / gradientColors and the dimming,
	// because resetting a Control dispatches the change its own handler listens
	// for. Only the Reverse switch, which is a checkbox and not a Control, is
	// still this function's to set.
	style.gradientReverse = false
	dom.Doc.Call("getElementById", "gradient-reverse").Set("checked", false)
	updateGradientUI()

	// Reset colors
	style.baseColor = [3]float32{1.0, 0.0, 0.0}
	style.midColor = [3]float32{0.0, 1.0, 0.0}
	style.topColor = [3]float32{0.0, 0.0, 1.0}
	style.bgColor = [3]float32{0, 0, 0}
	dom.Doc.Call("getElementById", "color-base").Set("value", "#ff0000")
	dom.Doc.Call("getElementById", "color-mid").Set("value", "#00ff00")
	dom.Doc.Call("getElementById", "color-top").Set("value", "#0000ff")
	dom.Doc.Call("getElementById", "color-bg").Set("value", "#000000")
	glctx.GL.Call("uniform3f", gpu.u.baseColor, style.baseColor[0], style.baseColor[1], style.baseColor[2])
	glctx.GL.Call("uniform3f", gpu.u.midColor, style.midColor[0], style.midColor[1], style.midColor[2])
	glctx.GL.Call("uniform3f", gpu.u.topColor, style.topColor[0], style.topColor[1], style.topColor[2])
	// Alpha=0: don't paint over the host page's bg (SVG logo etc).
	glctx.GL.Call("clearColor", 0, 0, 0, 0)

	// Reset the remaining effect switches to their defaults — dispatch 'change'
	// so each effect's own handler applies it (single source of truth). Layout
	// prefs (dock edge, interface size) and the Fullscreen toggle
	// are intentionally left alone.
	//
	// So is "preset-on", for the same reason and one more: recalling a preset
	// runs this function first, so listing the Presets module here would make
	// every recall close the drawer the recall was made from — unless the
	// preset happened to have been saved with it open.
	swDefaults := []struct {
		id  string
		def bool
	}{
		{"spect-fill", false},
		{"spectro-skin", false},
		{"handles-on", false}, {"desk-pass", false}, {"desk-contain", false},
		{"jam-sw", false}, {"show-meters", true},
		{"ring-sw", false}, {"twin-sw", false}, {"sect-sw", false},
		{"link-sw", true},
		{"scope-grat", true}, // the graticule is what makes the trace measurable
		// Back to recording the full canvas. This one is here because of what
		// it LEAVES BEHIND: choosing a region draws a dashed outline that dims
		// everything outside it, and the outline stays after the selection is
		// made, on purpose, so the chosen area is not something to remember.
		// Switching back to "full" is what clears it — and someone who does not
		// know that reaches for Reset All, which is exactly what that button is
		// for. Without this it was the one piece of screen furniture the button
		// could not remove.
		{"rec-region-sw", false},
	}
	for _, s := range swDefaults {
		if sw := dom.Doc.Call("getElementById", s.id); sw.Truthy() && sw.Get("checked").Bool() != s.def {
			sw.Set("checked", s.def)
			dom.Fire(sw, "change")
		}
	}
	// Every selector in the panel is a registry Control now, so the loop at the
	// top of this function has already put them all back — the backdrop, the
	// skin, the phosphor and LED color, Step and Fine, the Model Out rings, the
	// oscillators' routing and waveform, the envelope mode.
	//
	// What stood here was a resetSel closure and fourteen calls to it: a second,
	// hand-maintained list of every selector and its default, beside the one the
	// descriptors already state. Two lists of the same thing is how the Backdrop
	// came to drive a select option that did not exist, and nothing had checked
	// that this one still agreed with the panel either. The Size ring is the one
	// deliberate exclusion, and it says so on its own descriptor (SkipResetAll)
	// rather than by being absent from a list.

	// Randomized starting pose + low-rate rotation. Replaces the old
	// identity-matrix reset so each click of Reset All produces a
	// fresh viewing angle. randomizeOrientation zeroes the spin rates;
	// re-enable the gentle auto-spin afterward (so its Y-rate shows).
	// Flat scope modes stay face-on and still — screens, not models.
	if isFlatScope(run.selectedMode) {
		normalizeOrientation()
	} else {
		randomizeOrientation()
		view.ctl.autoRotate = false
		setAutoRotate(true)
	}

	// Reset view
	generateForMode(run.selectedMode)
	view.updateViewMatrix()
	view.updateModelMatrix()

	return nil
}

// startBackgroundTasks wires the input bindings that are not any one
// control's, and starts the two tickers that run for the life of the page:
// the wall clock in the header, and the debug stats when they are asked for.
func startBackgroundTasks() {
	// serialized ry already contained the contribution, and re-adding it crept
	// the rate +0.1 on every reload (0.1 → 0.2 → 0.3 …, found live on
	// magnetosphere.net). The contribution is added in the render loop now and
	// never written to the slider, so there is nothing to put back, nothing to
	// double, and no special case for a pinned rate.

	wireWheelBindings()
	wireKnobArrowKeys()

	// Last of the wiring: the reveal chord hides the whole control surface, so
	// it must run after every piece of that surface exists and has been placed.
	initPanelRevealChord()
	initHostPage() // centers the backdrop on a host element, if one was named

	// Window resize: keep canvas pixel dimensions in sync with the
	// viewport so the model doesn't get stretched when devtools opens
	// or closes (or on phone orientation change).
	dom.On(js.Global(), "resize", func(this js.Value, args []js.Value) any {
		if gpu.sizeCanvasToViewport() {
			glctx.GL.Call("viewport", 0, 0, gpu.width, gpu.height)
			setupMatrices()
		}
		return nil
	})

	// Clock goroutine
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if !rtc.IsUndefined() {
				rtc.Set("innerHTML", time.Now().Format("2006-01-02 15:04:05"))
			}
		}
	}()

	// Debug stats reporter goroutine
	if debugEnabled {
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				postDebugStats()
			}
		}()
	}

}

// applyHostPageTweaks is what a page EMBEDDING the rack needs, rather than
// anything the rack itself has: the scroll lock, and the footer handling that
// goes with it.
func applyHostPageTweaks() {
	//
	// This used to be unconditional, on the stated grounds that "Run() is only
	// invoked on the animation page" so nothing that legitimately scrolls could
	// be affected. That stopped being true: magnetosphere.net's front page is
	// both the animation page AND its catalog, because the category listings are
	// :target views of the same document. The rule locked a 7908px document to
	// the viewport, and the page could be scrolled for exactly as long as it took
	// the wasm to boot and then never again.
	if LockHostScroll {
		noScrollStyle := dom.Doc.Call("createElement", "style")
		noScrollStyle.Set("textContent", "html,body{overflow:hidden!important;margin:0;padding:0;}")
		dom.Doc.Get("head").Call("appendChild", noScrollStyle)
	}

	// Random initial orientation + low-rate rotation so the model doesn't start
	// in the same pose every load — UNLESS the permalink pinned an explicit pose
	// (&rot / &drag), in which case that must win so a shared still-view link is
	// restored faithfully. Must run AFTER the rotation-controls-x/y/z elements
	// are created and queried.
	if !perma.hashPinnedPose {
		// Flat scope modes (Pong, Fourier Text) boot face-on (their mode-entry
		// sync normalized the pose) rather than in a random pose.
		if !isFlatScope(run.selectedMode) {
			randomizeOrientation()
		}
		// randomizeOrientation zeroed the rate sliders — put back any spin
		// rates the permalink explicitly pinned (&rx/&ry/&rz).
		for ax, v := range perma.hashPinnedSpin {
			if sl := dom.Doc.Call("getElementById", "rotation-controls-"+ax); sl.Truthy() {
				sl.Set("value", v)
				dom.Fire(sl, "input")
			}
		}
		if len(perma.hashPinnedSpin) > 0 {
			syncKnobs()
		}
	} else {
		// A pinned pose (&rot) is by construction a STILL view — rot is only
		// serialized when auto-rotate is off and all spin rates are zero — so
		// clear any residual spin rate so the restored view doesn't drift away
		// from the shared pose.
		//
		// This used to be the place that swept up the negative Y rate a
		// restored ar=0 left behind, which fixed the shared-still-view case and
		// left every other one broken. The subtraction no longer happens, so
		// this is back to being what it says: a guard on the pose.
		for _, ax := range []string{"x", "y", "z"} {
			if sl := dom.Doc.Call("getElementById", "rotation-controls-"+ax); sl.Truthy() && sl.Get("value").String() != "0" {
				sl.Set("value", "0")
				dom.Fire(sl, "input")
			}
		}
	}
	// The spectrogram wants a static, face-on default instead — undo the
	// randomized pose/spin for an initial #spectrogram load (mode switches
	// go through onModeChange, which already handles this).
	if isTexturePlane(run.selectedMode) {
		spect.setSpectrogramCamera()
	}

	// Nothing to re-apply here any more. This used to put auto-rotate's Y-rate
	// contribution back after randomizeOrientation zeroed the spin sliders, and
}

// registerOutputControls declares the Model Out cells through the descriptor
// path, which is what gives each of them its LED formatting, typed entry,
// wheel nudge, reset and the Control that Reset All drives.
func registerOutputControls() {
	// View spin rates: the last controls on the legacy wiring path. Reset
	// also zeroes the axis ANGLE state and rebuilds the matrices (parity
	// with the old bespoke rst-rx/ry/rz handlers).
	adoptDescControl(ControlDesc{ID: "rotation-controls-x", Label: "X rate", Min: -1, Max: 1, Step: 0.1, Def: 0,
		Signed: true, PermaKey: "rx", LEDID: "slider-value-x", ResetID: "rst-rx",
		Apply: func(v float64) { view.ctl.spinX = float32(v) },
		ResetExtra: func() {
			view.angleX = 0
			view.rebuildModelMatrix()
			view.updateModelMatrix()
			rotKnobs.update()
			syncKnobs()
		}})
	adoptDescControl(ControlDesc{ID: "rotation-controls-y", Label: "Y rate", Min: -1, Max: 1, Step: 0.1, Def: 0,
		Signed: true, PermaKey: "ry", LEDID: "slider-value-y", ResetID: "rst-ry",
		Apply: func(v float64) { view.ctl.spinY = float32(v) },
		ResetExtra: func() {
			view.angleY = 0
			clearAutoRotateFlag() // Y spin (incl. auto) just zeroed
			view.rebuildModelMatrix()
			view.updateModelMatrix()
			rotKnobs.update()
		}})
	adoptDescControl(ControlDesc{ID: "rotation-controls-z", Label: "Z rate", Min: -1, Max: 1, Step: 0.1, Def: 0,
		Signed: true, PermaKey: "rz", LEDID: "slider-value-z", ResetID: "rst-rz",
		Apply: func(v float64) { view.ctl.spinZ = float32(v) },
		ResetExtra: func() {
			view.angleZ = 0
			view.rebuildModelMatrix()
			view.updateModelMatrix()
			rotKnobs.update()
		}})

	// Prime every registry control once: run its Apply from the DOM's current
}

// The position knobs turn once round (oneTurn) and read in degrees, like the
// angle knobs beside them: half a turn either way is the end of the range.
// The range itself is unchanged, so a permalink's px, py and z mean what they
// always meant; only the readout speaks degrees.

// degreesOf maps a slider in ±half to the ±180° its knob has turned,
// rounded to a millionth: the slider's fine step does not divide its range
// exactly, so the browser parks a centered knob at -8e-15, which read "-0".
func degreesOf(half float64) func(float64) float64 {
	return func(s float64) float64 { return math.Round(s*180/half*1e6) / 1e6 }
}

// fromDegrees is degreesOf backwards, for a typed readout.
func fromDegrees(half float64) func(float64) float64 {
	return func(d float64) float64 { return d * half / 180 }
}

// panStep is one wheel notch of a position knob: 5° of its turn.
const panStep = 8.0 / 36

// registerViewControls declares the camera and view cells the same way:
// zoom, the pans, the spins, and the trail.
func registerViewControls() {
	// other input.
	// Step a quarter, not a whole. Distance is 100 − zoom, so a step of one is
	// a hundredth of the way in at the far end and a TENTH of it at the near
	// end: the knob gets about twenty times twitchier exactly where a close look
	// is being taken. The mapping is left alone — it is what every saved
	// permalink's z means — and the step is made small enough that the near end
	// is usable. The knob's inner disc trims finer still.
	adoptDescControl(ControlDesc{ID: "camera-zoom", Label: "Zoom", Min: -95, Max: 95, Step: 0.25, Def: 0,
		Signed: true, PermaKey: "z", LEDID: "slider-value-zoom", ResetID: "rst-zoom",
		SliderToVal: degreesOf(95), ValToSlider: fromDegrees(95), LEDMin: -180, LEDMax: 180, LEDStep: 1,
		Apply: func(v float64) { view.ctl.zoom = float32(v) },
		ResetExtra: func() {
			view.defaultDist = view.initDist
			view.updateViewMatrix()
			syncKnobs()
		}})
	// Fore: where the model sits relative to the rack. The ends are the old
	// Front switch — all behind, all in front — and everything between is a
	// plane cutting the model, which is what the switch could never express.
	adoptDescControl(ControlDesc{ID: "model-fore", Label: "fore", Min: -1, Max: 1, Step: 0.05, Def: -1,
		Signed: true, PermaKey: "fo", LEDID: "slider-value-fore", ResetID: "rst-zoom",
		Apply: func(v float64) {
			splitFrac = float32(v)
			syncSplitCanvas()
		}})
	adoptDescControl(ControlDesc{ID: "pan-x", Label: "X", Min: -8, Max: 8, Step: panStep, Def: 0,
		Signed: true, PermaKey: "px", LEDID: "slider-value-panx", ResetID: "rst-panx",
		SliderToVal: degreesOf(8), ValToSlider: fromDegrees(8), LEDMin: -180, LEDMax: 180, LEDStep: 1,
		Apply: func(v float64) { view.ctl.panX = float32(v) }})
	adoptDescControl(ControlDesc{ID: "pan-y", Label: "Y", Min: -8, Max: 8, Step: panStep, Def: 0,
		Signed: true, PermaKey: "py", LEDID: "slider-value-pany", ResetID: "rst-pany",
		SliderToVal: degreesOf(8), ValToSlider: fromDegrees(8), LEDMin: -180, LEDMax: 180, LEDStep: 1,
		Apply: func(v float64) { view.ctl.panY = float32(v) }})
	// The sweep's own ends, as a fraction of whatever parameter it is
	// pointed at — which is what lets one pair of knobs bound a sweep of
	// any target. to below from runs the contact sheet backwards, which is
	// deliberate and is why neither clamps against the other.
	adoptDescControl(ControlDesc{ID: "sweep-lo", Label: "from", Min: 0, Max: 1, Step: 0.01, Def: 0,
		PermaKey: "wl", LEDID: "slider-value-swlo", ResetID: "rst-swlo",
		Apply: func(v float64) { grid.sweepLo = float32(v) }})
	adoptDescControl(ControlDesc{ID: "sweep-hi", Label: "to", Min: 0, Max: 1, Step: 0.01, Def: 1,
		PermaKey: "wh", LEDID: "slider-value-swhi", ResetID: "rst-swhi",
		Apply: func(v float64) { grid.sweepHi = float32(v) }})
	adoptDescControl(ControlDesc{ID: "rainbow-freq", Label: "period", Min: 0.05, Max: 20, Step: 0.05, Def: 1,
		PermaKey: "rf", LEDID: "slider-value-rfreq", ResetID: "rst-rfreq",
		Apply: func(v float64) { style.gradientFreq = float32(v) }})
	// The colormap window's position, paired with the period above. Def 0 is
	// the coordinate the colormaps already sampled, so a shared link opens on
	// the picture it was made from until this is turned. See palettemod_js.go
	// for why the range is exactly ±1 and why the ends are the reversed map
	// rather than a stop.
	adoptDescControl(ControlDesc{ID: "palette-shift", Label: "shift", Min: -1, Max: 1, Step: 0.01, Def: 0,
		Signed: true, PermaKey: "gh", LEDID: "slider-value-pshift", ResetID: "rst-pshift",
		Apply: func(v float64) { gradientShift = float32(v) }})
	// Speed: the slider runs log10 (-2..2) while the LED shows the effective
	// multiplier (0.01..100), whole sub-step counts at ≥1 — the one mapping
	// pair below is the SSOT both directions.
	adoptDescControl(ControlDesc{ID: "speed-slider", Label: "Speed", Min: -2, Max: 2, Step: 0.1, Def: 0,
		PermaKey: "sp", LEDID: "slider-value-speed", ResetID: "rst-speed",
		Apply:       sim.applySpeedLog,
		SliderToVal: speedDisplayVal,
		ValToSlider: func(v float64) float64 {
			if v <= 0 {
				return -2
			}
			lg := math.Log10(v)
			if lg < -2 {
				lg = -2
			}
			if lg > 2 {
				lg = 2
			}
			return lg
		},
		LEDMin: 0.01, LEDMax: 100, LEDStep: 0.1,
		ResetExtra: syncKnobs})
	// Line width: WebGL's gl.lineWidth() is capped at 1.0 on most modern
	// browsers/drivers (Chrome enforces it; many ANGLE / Mesa stacks too) —
	// the call still runs, but the visual effect is implementation-dependent.
	adoptDescControl(ControlDesc{ID: "line-width", Label: "Line", Min: 1, Max: 10, Step: 1, Def: 1,
		PermaKey: "lw", LEDID: "slider-value-line", ResetID: "rst-line",
		Apply: func(v float64) {
			if v < 1 {
				v = 1
			}
			glctx.GL.Call("lineWidth", v)
		}})
	// The points/line continuum. Def 1 is the solid trace this has always
	// drawn, so an existing view is unchanged until the knob is turned.
	adoptDescControl(ControlDesc{ID: "dash-duty", Label: "Points", Min: 0, Max: 4000, Step: 10, Def: 0,
		PermaKey: "pts", LEDID: "slider-value-dash", ResetID: "rst-dash",
		Apply: func(v float64) { pointCount = float32(v) }})
}

// wireColorAndViewControls wires the gradient and color cells, and the view
// switches that are not descriptor-owned — the twin canvas, the Poincare
// section, the sweep and grid dials, and the link switches.
func wireColorAndViewControls() {

	// Event: twin-trajectory switch + λ readout.
	twin.wireTwinSwitch()
	// Event: Poincaré-section switch.
	sect.wireSectSwitch()
	grid.wireViewGridDial()
	grid.wireViewLinkSwitches()
	wireBackLayer() // the Back switch: the panel on the model behind (backlayer_js.go)
	grid.wireSweepDial()

	// Event: persist trail checkbox
	dom.On(dom.Doc.Call("getElementById", "persist-trail"), "change", func(this js.Value, args []js.Value) any {
		style.persistTrail = dom.Doc.Call("getElementById", "persist-trail").Get("checked").Bool()
		return nil
	})

	// Event: show info checkbox. The text lives in a window now (see
	// infowindow_js.go) rather than in a caption pinned over the canvas, so
	// there is no element to create here and nothing to position: a description
	// taller than the screen scrolls, and one in the way can be moved.
	dom.On(dom.Doc.Call("getElementById", "show-info"), "change", func(this js.Value, args []js.Value) any {
		if dom.Doc.Call("getElementById", "show-info").Get("checked").Bool() {
			info.showInfoWindow()
		} else {
			info.hideInfoWindow()
		}
		return nil
	})

	// Event: background color picker. Alpha=0 keeps the canvas
	// transparent so the host page's background (e.g. m2's SVG logo)
	// shows through — picking a non-black bg here only tints what's
	// drawn, it doesn't paint over the host.
	dom.On(dom.Doc.Call("getElementById", "color-bg"), "input", func(this js.Value, args []js.Value) any {
		hex := dom.Doc.Call("getElementById", "color-bg").Get("value").String()
		style.bgColor = colorspace.ParseHex(hex)
		glctx.GL.Call("clearColor", style.bgColor[0], style.bgColor[1], style.bgColor[2], 0)
		return nil
	})
	dom.On(dom.Doc.Call("getElementById", "rst-color-bg"), "click", func(this js.Value, args []js.Value) any {
		style.bgColor = [3]float32{0, 0, 0}
		dom.Doc.Call("getElementById", "color-bg").Set("value", "#000000")
		glctx.GL.Call("clearColor", 0, 0, 0, 0)
		return nil
	})

	// Event: gradient source + colors selectors (each driven by a rotary knob).
	//
	// Through the registry, so each ring gets the reset button its cell now
	// carries and Reset All reaches both by driving the same Control the button
	// does — rather than by setting the two elements and calling updateGradientUI
	// itself, which is what it used to do and which could drift from these
	// handlers. No PermaKey: "gs" and "gc" already carry them in permaCtls.
	adoptDescControl(ControlDesc{
		ID: "gradient-source", Label: "src", IsSelect: true, SelectDef: "2",
		ResetID: "rst-gradient-source",
		SelectApply: func(v string) {
			if n, err := strconv.Atoi(v); err == nil {
				style.gradientSource = n
				// The focused view keeps it, so the two halves can be
				// colored differently. See views_js.go.
				grid.noteGradientSource(n)
			}
			// The source decides whether the map ring and the swatches apply at
			// all — OFF leaves nothing to map — so this has to refresh the dimming
			// the way the map ring's own handler does.
			updateGradientUI()
		},
	})
	// Whether the color ramp refits itself. A setting of the MAP rather than
	// of any one mode, which is why it sits beside src and map and not in a
	// mode row: every audio-fed source shares the one auto-range.
	adoptDescControl(ControlDesc{
		ID: "color-lock", Label: "rng", IsSelect: true, SelectDef: "0",
		ResetID:     "rst-color-lock",
		SelectApply: func(v string) { colorRangeLock = v == "1" },
	})
	adoptDescControl(ControlDesc{
		ID: "gradient-colors", Label: "map", IsSelect: true, SelectDef: "2",
		ResetID: "rst-gradient-colors",
		SelectApply: func(v string) {
			if n, err := strconv.Atoi(v); err == nil {
				style.gradientColors = n
				grid.noteGradientColors(n)
			}
			updateGradientUI()
		},
	})
	dom.On(dom.Doc.Call("getElementById", "gradient-reverse"), "change", func(this js.Value, args []js.Value) any {
		style.gradientReverse = dom.Doc.Call("getElementById", "gradient-reverse").Get("checked").Bool()
		return nil
	})

	// Event: pause button
	dom.On(dom.Doc.Call("getElementById", "pause-sw"), "change", func(this js.Value, args []js.Value) any {
		run.paused = dom.Doc.Call("getElementById", "pause-sw").Get("checked").Bool()
		return nil
	})

}

// wirePanelSwitches wires every switch and selector knob on the panel that
// no ControlDesc owns: the Console's own buttons and switches, and the three
// visual selectors (backdrop, skin, phosphor) that pick what is drawn behind,
// on and through the model.
func wirePanelSwitches() {

	// Event: normalize — reorient the current model to the default
	// (identity) pose and stop any slider-driven spin.
	dom.On(dom.Doc.Call("getElementById", "normalize-btn"), "click", func(this js.Value, args []js.Value) any {
		normalizeOrientation()
		return nil
	})

	// Event: speed slider
	// (Speed's input/reset wiring is owned by its ControlDesc registration.)

	// Event: auto-rotate switch — fold the auto-spin into the Y rate knob.
	dom.On(dom.Doc.Call("getElementById", "auto-rotate"), "change", func(this js.Value, args []js.Value) any {
		setAutoRotate(dom.Doc.Call("getElementById", "auto-rotate").Get("checked").Bool())
		return nil
	})

	// Event: Skin — WHICH picture is painted on the model's surface.
	//
	// A selector rather than the switch it was, for the reason the Backdrop is
	// one: the skin is one of three places a picture can go, and the other two
	// already take any source. The mesh does not care what is on it.
	if sk := dom.Doc.Call("getElementById", "skin-visual"); sk.Truthy() {
		dom.On(sk, "change", func(js.Value, []js.Value) any {
			skin.source = sk.Get("value").String()
			skin.dirty = true
			generateForMode(run.selectedMode)
			buildParamPanel(run.selectedMode) // the Spectro module arrives and leaves with it
			return nil
		})
		if holder := dom.Doc.Call("getElementById", "skin-stack"); holder.Truthy() {
			holder.Call("appendChild", selectorKnobReadout(sk))
		}
		// No PermaKey: "sk" already carries it in permaCtls.
		adoptDescControl(ControlDesc{
			ID: "skin-visual", Label: "skin", IsSelect: true, SelectDef: "", ResetID: "rst-skin-visual",
		})
	}
	// Event: Backdrop — ONE selector, because the state is one-of-N.
	//
	// This was four checkboxes (Spectro bg / XY bg / Term bg / Desk bg) kept
	// mutually exclusive by hand, over a hidden select. Two things were wrong
	// with that and one of them was fatal. A row of independent switches says
	// you may have two backdrops at once, which was never true; and the hand
	// exclusivity had drifted from the select it drove — the select had no
	// "desk" option, so Desk bg set it to a value that does not exist, the
	// value stayed empty, and the switch unchecked itself. That control had
	// never once worked.
	//
	// The select is now the only source of truth and the knob is a view of it,
	// so a backdrop that is not in the list cannot be selected by anything.
	if bv := dom.Doc.Call("getElementById", "bg-visual"); bv.Truthy() {
		dom.On(bv, "change", func(this js.Value, args []js.Value) any {
			setBackgroundVisual(bv.Get("value").String())
			onBackdropChoice(bgVisual)
			buildParamPanel(run.selectedMode) // the spectrogram's module follows it here
			return nil
		})
		if holder := dom.Doc.Call("getElementById", "bg-stack"); holder.Truthy() {
			holder.Call("appendChild", selectorKnobReadout(bv))
		}
		// No PermaKey: "bd" already carries it in permaCtls.
		adoptDescControl(ControlDesc{
			ID: "bg-visual", Label: "Behind", IsSelect: true, SelectDef: "", ResetID: "rst-bg-visual",
		})
	}

	// Phosphor selector — populate from the phosphor table + set phos.index.
	// Lives in the Style module as a rotary knob with a name readout (too many
	// options / too-long names for a label ring).
	if ph := dom.Doc.Call("getElementById", "phosphor"); ph.Truthy() {
		for i, p := range phosphors {
			opt := dom.Doc.Call("createElement", "option")
			opt.Set("value", strconv.Itoa(i))
			opt.Set("textContent", p.name)
			opt.Set("title", p.desc)
			ph.Call("appendChild", opt)
		}
		dom.On(ph, "change", func(this js.Value, args []js.Value) any {
			if v, err := strconv.Atoi(ph.Get("value").String()); err == nil {
				phos.index = v
			}
			// Selecting a phosphor IS how you enter CRT mode now: the trace is
			// drawn monochrome on that phosphor, so the gradient source + palette
			// colors are overridden (and dimmed).
			phos.crtMode = phos.index > 0
			phos.updateCRTOverlay()
			phos.updateCRTDim()
			return nil
		})
		// Index 0 is the "off" phosphor, which is what a reset means here: out
		// of CRT mode. The options are built from the phosphors table just above,
		// so the default is that table's first entry by construction.
		adoptDescControl(ControlDesc{
			ID: "phosphor", Label: "Phosphor", IsSelect: true, SelectDef: "0",
			ResetID: "rst-phosphor",
		})
		if holder := dom.Doc.Call("getElementById", "phosphor-stack"); holder.Truthy() {
			pk := selectorKnobReadout(ph)
			holder.Call("appendChild", pk)
			addPhosphorTraces(pk.Call("querySelector", ".knobstack"), ph)
		}
	}

	// Modulation runs when something needs it (syncAudioMod): a server feed
	// from the start, otherwise the first route.
	syncAudioMod()
	// The Info window's manual follows the bay last worked at, from the start.
	watchInfoBay()

	// Event: Meters switch — show/hide the top-left audio feature meters.
	dom.On(dom.Doc.Call("getElementById", "show-meters"), "change", func(this js.Value, args []js.Value) any {
		metersEnabled = dom.Doc.Call("getElementById", "show-meters").Get("checked").Bool()
		af.updateMetersVisibility()
		return nil
	})

	// The Mixer, then the generators it takes.
	buildMixer()
	buildGeneratorModule()
	// The readouts remember what they are showing (see led.Readouts), and
	// these calls hand them fresh elements, so the memory has to go with the
	// old ones or a new LED stays blank until its reading happens to move.
	owed.readouts.Forget()
	// Same reason, for the same elements: the visibility observer holds the
	// ones it was given, and a rebuilt panel's are not those.
	onScreen.invalidate()
	thd.wireDistortionModule()
	lufs.wireLoudnessModule()
	wow.wireWowFlutterModule()
	counter.wireCounterModule()
	tpanel.wireTimingModule()
	wireMeterClocks()
	fillReadoutColumns() // spare positions to the foot of each readout column
	dotReadoutLabels()   // the meters' readout names, on character displays
	// The analyzers move off this thread if the browser will have them; see
	// metersclient_js.go. Nothing downstream depends on whether it worked.
	mc.startMetersWorker()
	// The control surface, reachable from outside the page; see rackctl_js.go.
	exposeRackControl()
	enablePartsCatalog() // /parts: every distinct part, once; see parts_js.go
	enableManualMode()   // /manual: the rack writing its own manual; see manualmode_js.go
	lyap.wireAnalysisModule()
	keys.wireKeysModule()
	tm.wireTonematrixModule()
	rhy.wireRhythmModule()
	wirePresetModule()
	buildDemoModules()

	if sw := dom.Doc.Call("getElementById", "handles-on"); sw.Truthy() {
		dom.On(sw, "change", func(this js.Value, args []js.Value) any {
			setRackBay(sw.Get("checked").Bool())
			return nil
		})
	}

	// Event: points/line toggle
	dom.On(dom.Doc.Call("getElementById", "use-points"), "change", func(this js.Value, args []js.Value) any {
		style.usePoints = dom.Doc.Call("getElementById", "use-points").Get("checked").Bool()
		if style.usePoints {
			gpu.drawMode = glctx.Types.Points
		} else {
			gpu.drawMode = glctx.Types.LineStrip
		}
		return nil
	})

	// Event: trail length slider
	// (Trail's input/reset wiring is owned by its ControlDesc registration.)

	// Event: ring-trail switch — beam model on/off (re-primes on enable).
	if rs := dom.Doc.Call("getElementById", "ring-sw"); rs.Truthy() {
		dom.On(rs, "change", func(this js.Value, args []js.Value) any {
			ring.on = rs.Get("checked").Bool()
			ring.invalidate()
			return nil
		})
	}

	// Events: in-app recorder, jam mode, WebMIDI.
	rec.wireRecordSwitch()
	wireRegionSwitch()
	recmod.wireRecordModule()
	wireJamSwitch()
	wireMIDISwitch()

	// Event: the desk as the environment, with this app inside it.
	if dc := dom.Doc.Call("getElementById", "desk-contain"); dc.Truthy() {
		dom.On(dc, "change", func(js.Value, []js.Value) any {
			setDeskContain(dc.Get("checked").Bool())
			return nil
		})
	}

	// Event: where the mouse goes while the desk is a model.
	if dp := dom.Doc.Call("getElementById", "desk-pass"); dp.Truthy() {
		dom.On(dp, "change", func(js.Value, []js.Value) any {
			deskPassOn = dp.Get("checked").Bool()
			return nil
		})
	}

	// Event: which of the four 3-D desktops the desk wears.
	desks.buildDeskStyleSelect()
}

// wireExtraNav puts the host page's own navigation into the Console, and
// settles the Physics switch once the URL hash has chosen the mode.
func wireExtraNav() {
	// straight into #turtle left the switch hidden, and it only appeared after
	// changing modes to somewhere else and back. Every shared link to the
	// turtle — the README's included — came up with its physics unreachable.
	updatePhysVisibility()

	// Wire input listeners for the fixed sliders so the cached vars
	// + visible text output stay in sync with user interaction. The
	// renderLoop reads view.ctl.zoom/RotX/Y/Z instead of polling
	// parseFloat per frame.

	// (Speed / Line / Trail LED treatment is owned by their ControlDesc
	// registrations below.)

	// Numeric input → slider: typing a value (committed on Enter/blur)
	// drives the paired slider, reusing its input handler for all the
	// downstream work. Uses "change" (not "input") so the slider handler
	// writing back the formatted value doesn't fight per-keystroke typing.
	// toSlider maps the typed number to the slider's raw value (identity
	// for most; log10 for the speed slider, whose display is the effective
	// multiplier).
	// (line / trail / speed typed entry is owned by their ControlDesc
	// registrations — speed's log10 mapping lives in its ValToSlider.)

	// Line-width slider. WebGL's gl.lineWidth() is capped at 1.0
	// on most modern browsers/drivers (Chrome enforces it; many
	// platforms' ANGLE / Mesa drivers do too) — the call still
	// runs but visual effect is implementation-dependent. If the
	// stack honors it, the slider gives thicker line strokes; if
	// not, this is a harmless no-op for values >1. Wheel-binding
	// for "line-width" is registered in the bindWheelToInput loop
	// further down.
	// (Line width's input/reset wiring is owned by its ControlDesc registration.)

	// Optional host-injected nav snippet (e.g. m2 links to /attractors).
	if ExtraNavHTML != "" {
		nav := dom.Doc.Call("getElementById", "extra-nav")
		if nav.Truthy() {
			nav.Set("innerHTML", ExtraNavHTML)
		}
	}

}

// initDrawState sets the GL enum table and the draw mode the first frame
// will use.
func initDrawState() {
	// The enum table is built by glctx.Init, with the context it belongs to.
	gpu.drawMode = glctx.Types.LineStrip
	// Bind buffers before setting up attrib pointers in setupShaders
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, gpu.vbuf)
	glctx.GL.Call("bindBuffer", glctx.Types.ElementArrayBuffer, gpu.ibuf)
	gpu.setupShaders()
	texp.setupTexShaders()
	setupMatrices()
	generateForMode(run.selectedMode)
	if isTexturePlane(run.selectedMode) {
		spect.setSpectrogramCamera()
	} else {
		view.autoFitCamera()
	}
	refreshGradient()

	// Check if debug mode is enabled via JS global
}

// wireViewGridStack builds the Grid module and fills its dials.
func wireViewGridStack() {
	// The Grid's P-units, before its dials are filled (gridbank_js.go).
	buildGridBank()
	// The sweep dial: what varies across the grid. Its options are the
	// current mode's own parameters, so building it is a function the
	// mode change calls too rather than a block written out here.
	grid.setSweepTargets(run.selectedMode)
	grid.buildSweepDial()
	buildRackScopes() // the rack scopes' own dials, independent of the model
	wireColorLockSwitch()
	updateGradientUI()
	// And again when the fonts land: the widths are measured from text, and the
}

// buildRackAndRestore assembles the rack — the category rows, their
// monitors, the row switches — and puts back the arrangement this browser
// last left, in the order the restore requires.
func buildRackAndRestore() {
	// category rows' rotaries included.
	selk.initSelKnobDrag()
	// The saved arrangement goes back BEFORE the switches are built: the rack
	// builds each one checked or not from its own hidden set, so restoring
	// afterward gives a switch that says a module is in while it is out.
	restoreRackLayout()
	restoreRackBay()
	// One row per model category, each with the rotary that selects within
	// it. Before the first layout pass, so the rows are packed with
	// everything else rather than appearing after it.
	buildCategoryModules()
	wireRowMonitors() // each row's screen, after the rows that hold them exist
	// And measure the rack once it exists. The category rows are built here,
	// after the boot pass that sized every other module, and a module whose
	// width was never measured against its real content keeps whatever the
	// last pass guessed — measured, the two widest rows came up a slot short
	// and clipped 35px of their last column until the window was resized.
	afterTwoFrames(func() {
		// Twice: the first pass measures modules that the pass before it has
		// just re-parented into their bays, and a module measured in the
		// wrong opening is measured against the wrong available width.
		quantizeModuleWidths()
		quantizeModuleWidths()
	})
	wireScreenPower() // the scope's intensity and the Monitor switches: a screen you are not watching costs nothing
	wireModuleDrag()
	// Source and map: two knobs in two cells, each with its own ring and its own
	// label.
	//
	// Concentric on one dial first, under a single cell labeled "src" — which
	// named the outer ring and left the inner one unnamed, so the cell's tooltip
	// had to open by correcting its own label. A cell holds one control; these
	// are two, and they answer different questions (what the color follows, and
	// how a value becomes a color).
}

// wirePowerSwitch is the rack's own power: off stops the render loop and
// clears the canvas, keeping the panel.
func wirePowerSwitch() {
	// every setting is still there to read and to change. A switch again: it
	// had been folded into the model category knob's first detent, and when
	// that knob moved to the rows it would have gone with it.
	if sw := dom.Doc.Call("getElementById", "power-sw"); sw.Truthy() {
		dom.On(sw, "change", func(this js.Value, args []js.Value) any {
			setPowerState(sw.Get("checked").Bool())
			// The rotaries say the same thing the switch does: powered down,
			// every row reads off, because no model is being drawn.
			syncCategoryRotaries()
			return nil
		})
	}

	// Fullscreen switch — on requests fullscreen, off exits. Kept in sync with
	// the actual fullscreen state (fullscreenchange fires on Esc etc.).
	// requestFullscreen/exitFullscreen return a promise that REJECTS when the
	// browser refuses (no user gesture, a permissions-policy block, or some
	// mobile/embedded contexts → "Permissions check failed"). Swallow that with
	// a .catch and re-sync the switch to reality, so it never surfaces as an
	// unhandled promise rejection.
}

// wirePhysSwitch is the Physics switch and the module it reveals.
func wirePhysSwitch() {
	// knob among the figure's parameters, because it is not one of them: the
	// figure is the same figure whatever it weighs. Throwing it reveals the
	// Physics module and hands the placing over to the body.
	if ps := dom.Doc.Call("getElementById", "phys-sw"); ps.Truthy() {
		dom.On(ps, "change", func(this js.Value, args []js.Value) any {
			turtlePhysOn = dom.Doc.Call("getElementById", "phys-sw").Get("checked").Bool()
			if turtlePhysOn {
				// Face the room. The body is simulated in the PLANE OF THE
				// SCREEN — the floor is the bottom of the picture and the walls
				// are its edges — which is only true if the model pose is
				// identity. The turtle boots at a random orientation like every
				// other model, so gravity pulled along world −y while the screen
				// showed that direction rotated by (340°, 201°, 359°): the
				// figure appeared to fall sideways, came to rest against a floor
				// that was not the bottom of the picture, and settled at a tilt
				// instead of flat. Picking it up had the same problem, since the
				// grab hit-tests world positions against a screen pointer.
				normalizeOrientation()
			}
			updatePhysVisibility()
			return nil
		})
		updatePhysVisibility() // a definite state before any mode change
	}

	// Power switch (default on): off stops the render loop and clears the
}

// wireColorControls wires the color swatches, their Hue/Sat/Val knobs and
// the resets that go with them.
func wireColorControls() {
	attachColorKnobs() // Hue/Sat/Val knob under each color swatch

	// Event: per-control reset buttons for colors
	dom.On(dom.Doc.Call("getElementById", "rst-color-base"), "click", func(this js.Value, args []js.Value) any {
		style.baseColor = [3]float32{1.0, 0.0, 0.0}
		dom.Doc.Call("getElementById", "color-base").Set("value", "#ff0000")
		glctx.GL.Call("uniform3f", gpu.u.baseColor, style.baseColor[0], style.baseColor[1], style.baseColor[2])
		return nil
	})
	dom.On(dom.Doc.Call("getElementById", "rst-color-mid"), "click", func(this js.Value, args []js.Value) any {
		style.midColor = [3]float32{0.0, 1.0, 0.0}
		dom.Doc.Call("getElementById", "color-mid").Set("value", "#00ff00")
		glctx.GL.Call("uniform3f", gpu.u.midColor, style.midColor[0], style.midColor[1], style.midColor[2])
		return nil
	})
	dom.On(dom.Doc.Call("getElementById", "rst-color-top"), "click", func(this js.Value, args []js.Value) any {
		style.topColor = [3]float32{0.0, 0.0, 1.0}
		dom.Doc.Call("getElementById", "color-top").Set("value", "#0000ff")
		glctx.GL.Call("uniform3f", gpu.u.topColor, style.topColor[0], style.topColor[1], style.topColor[2])
		return nil
	})

	// Event: reset all button
	dom.On(dom.Doc.Call("getElementById", "reset-all-btn"), "click", onResetAll)

	// Any reset button (↺) — re-sync every knob pointer to its freshly-reset
	// value on the next frame (after the button's own handler has set values),
	// so knobs whose handler sets the slider without dispatching 'input' still
	// snap their pointer back.
}

// buildPanelKnobs turns the panel's fixed sliders into knobs and wires the
// three rotation knobs that drag against each other.
//
// This is the block that kept Run above its complexity budget after the
// thirteen straight cuts, and it is a unit rather than a sequence: knobAxis,
// knobAngleAt, attachKnob, knobRelease, knobifyFixed and stackAxis are
// declared once here and used by every axis and every slider below them.
// Nothing outside mentions any of them, which is what makes it a function
// and not a cut — the state was always private to this, it just had Run for
// a scope.
func buildPanelKnobs() {
	// pots set absolute X/Y angles on 7-seg displays.
	rotKnobs.ptr[0] = dom.Doc.Call("getElementById", "knobptr-x")
	rotKnobs.ptr[1] = dom.Doc.Call("getElementById", "knobptr-y")
	rotKnobs.ptr[2] = dom.Doc.Call("getElementById", "knobptr-z")
	rotKnobs.led[0] = dom.Doc.Call("getElementById", "led-x")
	rotKnobs.led[1] = dom.Doc.Call("getElementById", "led-y")
	rotKnobs.led[2] = dom.Doc.Call("getElementById", "led-z")
	rotKnobs.ready = true
	// Shared drag state (only one knob turns at a time). Document-level
	// move/up listeners (below) let the drag continue when the cursor
	// leaves the small knob, without setPointerCapture (a JS throw there
	// would panic the Go callback).
	knobAxis := -1 // which axis is being turned (-1 = none)
	var knobCX, knobCY, knobPrevAng float64
	knobAngleAt := func(e js.Value) float64 {
		return math.Atan2(e.Get("clientY").Float()-knobCY, e.Get("clientX").Float()-knobCX)
	}
	attachKnob := func(knobID string, axis int, spin, spinNum js.Value) {
		kn := dom.Doc.Call("getElementById", knobID)
		if !kn.Truthy() {
			return
		}
		dom.On(kn, "pointerdown", func(this js.Value, args []js.Value) any {
			e := args[0]
			e.Call("preventDefault")
			r := kn.Call("getBoundingClientRect")
			knobCX = r.Get("left").Float() + r.Get("width").Float()/2
			knobCY = r.Get("top").Float() + r.Get("height").Float()/2
			knobPrevAng = knobAngleAt(e)
			knobAxis = axis
			// Grabbing the knob holds the pose: stop this axis's spin
			// ("the speed obeys the knob").
			spin.Set("value", "0")
			if spinNum.Truthy() {
				spinNum.Set("value", "0")
			}
			setSpinAxis(axis, 0)
			if axis == 1 { // Y spin just zeroed → reflect auto-rotate off
				clearAutoRotateFlag()
			}
			return nil
		})
		// Scroll over the angle ring nudges the pose by 5° per notch.
		dom.On(kn, "wheel", func(this js.Value, args []js.Value) any {
			e := args[0]
			e.Call("preventDefault")
			e.Call("stopPropagation")
			step := float32(math.Pi / 36) // 5°
			if e.Get("deltaY").Float() > 0 {
				step = -step
			}
			addAngleAxis(axis, step)
			return nil
		})
	}
	attachKnob("knob-x", 0, camPanel.rotationControlsX, camPanel.sliderX)
	attachKnob("knob-y", 1, camPanel.rotationControlsY, camPanel.sliderY)
	attachKnob("knob-z", 2, camPanel.rotationControlsZ, camPanel.sliderZ)
	onPointerMove(func(e js.Value) {
		if knobAxis < 0 {
			return
		}
		cur := knobAngleAt(e)
		d := cur - knobPrevAng
		for d > math.Pi { // shortest-arc delta so it turns endlessly
			d -= 2 * math.Pi
		}
		for d < -math.Pi {
			d += 2 * math.Pi
		}
		knobPrevAng = cur
		addAngleAxis(knobAxis, float32(d))
	})
	knobRelease := dom.FuncOf(func(this js.Value, args []js.Value) any {
		knobAxis = -1
		return nil
	})
	dom.Doc.Call("addEventListener", "pointerup", knobRelease)
	dom.Doc.Call("addEventListener", "pointercancel", knobRelease)
	rotKnobs.update()
	initPointerMove() // the one shared pointermove listener the drags share
	initKnobDrag()    // document listeners for the bounded param/camera knobs

	// Knobify the fixed sliders too (zoom, speed, line, trail, spin rates):
	// hide each range input and insert a bounded knob that drives it. Reuses
	// each slider's existing 'input' handler, so behavior is unchanged.
	knobifyFixed := func(sliderID, numID string, dial bool) js.Value {
		sl := dom.Doc.Call("getElementById", sliderID)
		if !sl.Truthy() {
			return js.Undefined()
		}
		num := dom.Doc.Call("getElementById", numID)
		sl.Get("style").Set("display", "none")
		// The knob is inserted bare; the cell's reset button stays a cell child and
		// is pinned to the header's top-right by CSS (standard cell template).
		k := makeKnob(sl, num, true, true, dial)
		parent := sl.Get("parentNode")
		// Insert the knob right after the (hidden) slider. Only insertBefore the
		// numeric when it's actually a sibling — with the label-column layout the
		// numeric lives in a separate span, so we just append to the column.
		if num.Truthy() && num.Get("parentNode").Equal(parent) {
			parent.Call("insertBefore", k, num)
		} else {
			parent.Call("appendChild", k)
		}
		return k
	}
	// Value knobs get a numeric scale dial; the rotation-rate knobs don't (they
	// nest as the inner disc of the View angle knobs, which already carry a
	// degree dial — a second scale would collide). Position and zoom are built
	// below, beside the angle knobs and in their image (stackPos).
	knobifyFixed("speed-slider", "slider-value-speed", true)
	knobifyFixed("line-width", "slider-value-line", true)
	knobifyFixed("dash-duty", "slider-value-dash", true)
	knobifyFixed("trail-slider", "slider-value-trail", true)
	knobifyFixed("rainbow-freq", "slider-value-rfreq", true)
	knobifyFixed("sweep-lo", "slider-value-swlo", true)
	knobifyFixed("sweep-hi", "slider-value-swhi", true)
	knobifyFixed("palette-shift", "slider-value-pshift", true)
	rkx := knobifyFixed("rotation-controls-x", "slider-value-x", false)
	rky := knobifyFixed("rotation-controls-y", "slider-value-y", false)
	rkz := knobifyFixed("rotation-controls-z", "slider-value-z", false)

	// Stack each axis's angle knob (outer ring) around its spin-rate knob
	// (inner), oscilloscope-style, with an analog degree dial around the ring.
	// Each axis is a NARROW vertical strip: label, degrees LED and reset on
	// one line · the angle knob · ω and the spin-rate numeric. This sets the
	// minimum module slot width.
	stackAxis := func(axisLbl, angleID, ledID, rateNumID, rateRstID string, rateKnob js.Value) {
		ak := dom.Doc.Call("getElementById", angleID)
		if !ak.Truthy() || !rateKnob.Truthy() {
			return
		}
		led := dom.Doc.Call("getElementById", ledID)
		rateNum := dom.Doc.Call("getElementById", rateNumID)
		rst := dom.Doc.Call("getElementById", rateRstID)
		cell := ak.Call("closest", ".pcell")
		angleGrp := ak.Get("parentNode")
		var rateGrp js.Value
		if rateNum.Truthy() {
			rateGrp = rateNum.Get("parentNode")
		}

		stack := stackKnobs(ak, rateKnob)
		addAngleDial(stack)

		knobBox := dom.Doc.Call("createElement", "span")
		knobBox.Set("className", "axknob-box")
		knobBox.Call("appendChild", stack)

		col := dom.Doc.Call("createElement", "span")
		col.Set("className", "grp axstack")
		// Top line: axis label ("X") to the LEFT of the degrees LED readout.
		topRow := dom.Doc.Call("createElement", "span")
		topRow.Set("className", "grp axrow toprow")
		if cell.Truthy() {
			if lbl := cell.Call("querySelector", ".plabel"); lbl.Truthy() {
				lbl.Set("textContent", axisLbl)
				topRow.Call("appendChild", lbl)
			}
		}
		if led.Truthy() {
			topRow.Call("appendChild", led)
		}
		// The reset ends the readout's line, as it does in every other cell.
		if rst.Truthy() {
			topRow.Call("appendChild", rst)
		}
		col.Call("appendChild", topRow)
		col.Call("appendChild", knobBox)
		// Bottom line: "Rate" label to the LEFT of the spin-rate numeric.
		botRow := dom.Doc.Call("createElement", "span")
		botRow.Set("className", "grp axrow botrow")
		rlbl := dom.Doc.Call("createElement", "span")
		rlbl.Set("className", "plabel sym") // ω = angular rate; .sym keeps it lowercase (not Ω)
		rlbl.Set("textContent", "ω")
		botRow.Call("appendChild", rlbl)
		if rateNum.Truthy() {
			botRow.Call("appendChild", rateNum)
		}
		col.Call("appendChild", botRow)

		// swap the column in for the old [knob][led] grp, hide the rate row
		if angleGrp.Truthy() {
			p := angleGrp.Get("parentNode")
			p.Call("insertBefore", col, angleGrp)
			p.Call("removeChild", angleGrp)
		}
		if rateGrp.Truthy() {
			rateGrp.Get("style").Set("display", "none")
		}
	}
	stackAxis("X", "knob-x", "led-x", "slider-value-x", "rst-rx", rkx)
	stackAxis("Y", "knob-y", "led-y", "slider-value-y", "rst-ry", rky)
	stackAxis("Z", "knob-z", "led-z", "slider-value-z", "rst-rz", rkz)

	// Each axis's position stands beside its angle, in the same shape: label,
	// readout and reset on the top line, the knob in a degree dial, and a
	// second readout under it. Position turns once round (oneTurn), so the
	// dial is the angle knobs' own; zoom carries Fore nested on top, the way
	// the angle carries ω, and X and Y have nothing underneath.
	stackPos := func(sliderID, numID, innerID, innerNumID, innerLbl string) {
		sl := dom.Doc.Call("getElementById", sliderID)
		if !sl.Truthy() {
			return
		}
		cell := sl.Call("closest", ".pcell")
		num := dom.Doc.Call("getElementById", numID)
		sl.Get("style").Set("display", "none")
		// The fine disc is the knob's center, which an inner knob covers.
		nested := innerID != ""
		kn := makeKnob(sl, num, !nested, true, false).Call("querySelector", ".knob")
		var stack js.Value
		if nested {
			isl := dom.Doc.Call("getElementById", innerID)
			isl.Get("style").Set("display", "none")
			stack = stackKnobs(kn, makeKnob(isl, dom.Doc.Call("getElementById", innerNumID), true, true, false))
		} else {
			stack = dom.Doc.Call("createElement", "span")
			stack.Set("className", "knobstack")
			stack.Call("setAttribute", "data-no-drag", "")
			kn.Get("classList").Call("add", "knob-ring")
			stack.Call("appendChild", kn)
		}
		addAngleDial(stack)
		knobBox := dom.Doc.Call("createElement", "span")
		knobBox.Set("className", "axknob-box")
		knobBox.Call("appendChild", stack)

		col := dom.Doc.Call("createElement", "span")
		col.Set("className", "grp axstack")
		topRow := dom.Doc.Call("createElement", "span")
		topRow.Set("className", "grp axrow toprow")
		for _, sel := range []string{".plabel", ".numin", ".rst"} {
			if e := cell.Call("querySelector", sel); e.Truthy() {
				topRow.Call("appendChild", e)
			}
		}
		botRow := dom.Doc.Call("createElement", "span")
		botRow.Set("className", "grp axrow botrow")
		if nested {
			lbl := dom.Doc.Call("createElement", "span")
			lbl.Set("className", "plabel")
			lbl.Set("textContent", innerLbl)
			botRow.Call("appendChild", lbl)
			botRow.Call("appendChild", dom.Doc.Call("getElementById", innerNumID))
		}
		if top := cell.Call("querySelector", ".punit-top"); top.Truthy() {
			cell.Call("removeChild", top)
		}
		col.Call("appendChild", topRow)
		col.Call("appendChild", knobBox)
		col.Call("appendChild", botRow)
		cell.Call("appendChild", col)
	}
	stackPos("pan-x", "slider-value-panx", "", "", "")
	stackPos("pan-y", "slider-value-pany", "", "", "")
	stackPos("camera-zoom", "slider-value-zoom", "model-fore", "slider-value-fore", "fore")

	if sf := dom.Doc.Call("getElementById", "spect-fill"); sf.Truthy() {
		dom.On(sf, "change", func(this js.Value, args []js.Value) any {
			spect.fill = sf.Get("checked").Bool()
			return nil
		})
	}

}

// capturePermalinkAndRestore takes the pristine defaults the permalink
// diffs against, then puts back whatever the URL and this browser remember,
// in the order those two have to happen in.
func capturePermalinkAndRestore() {
	// state so the current view is always shareable.
	perma.capturePermaDefaults()
	applyStateFromHash()
	// A link that opens on the spectrogram without naming a map gets the
	// spectrogram's own; one that names a map keeps it.
	spect.followMode(run.selectedMode)
	perma.startPermalinkSync()

	// Final tooltip pass now that every selector (gradient / model / style) is
	// built — some are created after the first buildParamPanel's annotate.
	annotateControlTooltips()

	// Start animation loop
}

// commitBuiltControls fires each registered control once so the value it
// was built with actually reaches the thing it drives. A selector-backed
// Control has no slider, and calling a method on an undefined js.Value is a
// panic rather than a no-op.
func commitBuiltControls() {
	// did this ad hoc — applyLineWidth() at wiring, readSliderCache, …).
	//
	// Under withDeferredLayout, because several of these handlers rebuild the
	// parameter panel or re-quantize the rack and none of them needs either
	// to have happened before the next control is committed. Measured on this
	// rack: twenty-three quantizes and three panel rebuilds, six seconds of a
	// thirteen-second boot. One of each at the end is the same answer.
	owed.withDeferredLayout(run.selectedMode, func() {
		for _, c := range builtControls {
			// Whichever element holds this control's value, and the event that
			// commits it. A selector-backed Control has no slider at all, and Call
			// on an undefined js.Value is a panic rather than a no-op — which took
			// the whole runtime down the first time a selector reached this loop.
			switch {
			case c.sel.Truthy():
				dom.Fire(c.sel, "change")
			case c.slider.Truthy():
				dom.Fire(c.slider, "input")
			}
		}
	})

	// Permalink: capture pristine control defaults, restore any state
}

// wireGradientKnobs builds the two color rings — what the gradient follows,
// and how a value becomes a color — as knobs over their hidden selects.
func wireGradientKnobs() {
	bootMark("rack-start")
	buildRackAndRestore()
	bootMark("rack-done")
	gsrc := dom.Doc.Call("getElementById", "gradient-source")
	gcol := dom.Doc.Call("getElementById", "gradient-colors")
	if gsrc.Truthy() && gcol.Truthy() {
		if sh := dom.Doc.Call("getElementById", "gradient-stack"); sh.Truthy() {
			sstack := soloKnob(gsrc)
			// OFF first, because it is the absence of a source rather than one more
			// of them. Its option value is 5 while the five that follow keep 0..4,
			// so a permalink written before this still names the same source: the
			// ring binds a label to an option by INDEX and the link by VALUE, and
			// those are free to disagree.
			//
			// ONE LABEL PER OPTION, IN OPTION ORDER. Binding by index is what
			// makes that a requirement rather than a nicety: seven audio sources
			// were added to the select and this list was left at six, so the dial
			// went on offering the original six and the new ones could not be
			// reached from the knob at all — only from a permalink. The order
			// here is the order in panelhtml_js.go, not numeric by value.
			addSelectorLabels(sstack, gradSrcRingLabels, gsrc).
				Set("id", "grad-src-ring")
			sh.Call("appendChild", sstack)
			gsrc.Get("style").Set("display", "none")
		}
		if mh := dom.Doc.Call("getElementById", "map-stack"); mh.Truthy() {
			mstack := soloKnob(gcol)
			// No "1" here any more: mono was never a map, it was the absence of a
			// source, and it lives on the src ring as OFF. Every position left is
			// a genuine mapping of a value to a color.
			//
			// NINE labels for nine options, and the count is load-bearing: a ring
			// that does not match its select is discarded whole and the dial falls
			// back to full names, which is how the spectrogram's old color dial
			// came to read "graysca…e" and "…idis" under the knob when turbo,
			// viridis and magma were added to a three-label ring. Add a map here
			// and add its label in the same commit.
			// 45 rather than the src ring's 43: "hue" is the one three-character
			// label and it lands where its width points straight at the knob, so at
			// the src ring's radius it touched the dial while every 2-character
			// label beside it cleared. Two more percent is as far as it can go —
			// past that the outermost labels clip the cell.
			addSelectorLabels(mstack, []string{"2", "3", "hue", "ht", "bl", "gy", "tb", "vr", "mg"}, gcol).
				Set("id", "grad-map-ring")
			mh.Call("appendChild", mstack)
			gcol.Get("style").Set("display", "none")
		}
	}
	wireViewGridStack()
}

// wireFullscreenSwitch drives browser fullscreen from the Console switch and
// keeps the switch in step with the real state, which Esc can change without
// asking. requestFullscreen rejects rather than throws when the browser
// refuses, so the rejection has to be caught or it surfaces as an unhandled
// promise.
func wireFullscreenSwitch() {
	wirePowerSwitch()
	fsReject := dom.FuncOf(func(this js.Value, a []js.Value) any {
		if sw := dom.Doc.Call("getElementById", "fullscreen-sw"); sw.Truthy() {
			sw.Set("checked", dom.Doc.Get("fullscreenElement").Truthy() || dom.Doc.Get("webkitFullscreenElement").Truthy())
		}
		return nil
	})
	catchFs := func(pr js.Value) {
		if pr.Truthy() && !pr.Get("then").IsUndefined() {
			pr.Call("catch", fsReject)
		}
	}
	dom.On(dom.Doc.Call("getElementById", "fullscreen-sw"), "change", func(this js.Value, args []js.Value) any {
		want := dom.Doc.Call("getElementById", "fullscreen-sw").Get("checked").Bool()
		if want {
			docEl := dom.Doc.Get("documentElement")
			if !docEl.Get("requestFullscreen").IsUndefined() {
				catchFs(docEl.Call("requestFullscreen"))
			} else if !docEl.Get("webkitRequestFullscreen").IsUndefined() {
				catchFs(docEl.Call("webkitRequestFullscreen"))
			}
		} else {
			if !dom.Doc.Get("exitFullscreen").IsUndefined() {
				catchFs(dom.Doc.Call("exitFullscreen"))
			} else if !dom.Doc.Get("webkitExitFullscreen").IsUndefined() {
				catchFs(dom.Doc.Call("webkitExitFullscreen"))
			}
		}
		return nil
	})
	syncFsSwitch := dom.FuncOf(func(this js.Value, args []js.Value) any {
		if sw := dom.Doc.Call("getElementById", "fullscreen-sw"); sw.Truthy() {
			sw.Set("checked", dom.Doc.Get("fullscreenElement").Truthy() || dom.Doc.Get("webkitFullscreenElement").Truthy())
		}
		return nil
	})
	dom.Doc.Call("addEventListener", "fullscreenchange", syncFsSwitch)
	dom.Doc.Call("addEventListener", "webkitfullscreenchange", syncFsSwitch)

	wireModelInput()

}

// wireModeAndResetInputs wires the model select, the color callback and the
// document-level click that resyncs a control after a reset.
func wireModeAndResetInputs() {

	// Event: mode change
	dom.On(dom.Doc.Call("getElementById", "mode-select"), "change", onModeChange)

	// Event: color pickers
	colorCallback := dom.FuncOf(onColorChange)
	dom.Doc.Call("getElementById", "color-base").Call("addEventListener", "input", colorCallback)
	dom.Doc.Call("getElementById", "color-mid").Call("addEventListener", "input", colorCallback)
	dom.Doc.Call("getElementById", "color-top").Call("addEventListener", "input", colorCallback)
	wireColorControls()
	resetSync := dom.FuncOf(func(this js.Value, args []js.Value) any { syncKnobs(); return nil })
	dom.On(dom.Doc, "click", func(this js.Value, a []js.Value) any {
		if t := a[0].Get("target"); t.Truthy() && t.Call("closest", ".rst").Truthy() {
			js.Global().Call("requestAnimationFrame", resetSync)
		}
		return nil
	})
	wirePanelSwitches()
}

// cacheElementRefs looks up the elements the render loop and the controls
// reach for every frame, so neither pays for getElementById at 60Hz.
func cacheElementRefs() {

	// Get control element references
	rtc = dom.Doc.Call("getElementById", "runtime")
	camPanel.cameraControl = dom.Doc.Call("getElementById", "camera-zoom")
	camPanel.rotationControlsX = dom.Doc.Call("getElementById", "rotation-controls-x")
	camPanel.rotationControlsY = dom.Doc.Call("getElementById", "rotation-controls-y")
	camPanel.rotationControlsZ = dom.Doc.Call("getElementById", "rotation-controls-z")
	camPanel.sliderZoom = dom.Doc.Call("getElementById", "slider-value-zoom")
	camPanel.sliderX = dom.Doc.Call("getElementById", "slider-value-x")
	camPanel.sliderY = dom.Doc.Call("getElementById", "slider-value-y")
	camPanel.sliderZ = dom.Doc.Call("getElementById", "slider-value-z")

	// ── Rotation knobs (digital-pot style, one per axis) ──────────────
	// Turning a knob sets that axis's absolute angle (position) and zeroes
	// its spin rate — "the speed obeys the knob". Moving the rate slider
	// spins the axis and the knob pointer/LED track it live — "the knob
}
