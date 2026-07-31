//go:build js && wasm

package attractor

import (
	_ "embed"
	"fmt"
	"math"
	"runtime"
	"strconv"
	"syscall/js"
	"time"

	"github.com/go-gl/mathgl/mgl32"
)

func Run() {
	// Lazy WebGL init — see initWebGL doc. Must run after the host
	// DOM is ready (caller's responsibility); otherwise gocanvas
	// won't exist yet and canvasEl.Call("getContext", ...) panics.
	initWebGL()
	if body.IsUndefined() || body.IsNull() {
		js.Global().Call("alert", "cannot get html body, exiting")
		return
	}
	if canvasEl.IsUndefined() || canvasEl.IsNull() {
		js.Global().Call("alert", "cannot find #gocanvas, exiting")
		return
	}
	installErrorNet()

	// Build controls panel. If the host page already has a <footer>
	// (e.g. m2/magnetosphere.net puts cart + shipping nav there), append
	// the controls inline into it so the existing footer nav stays
	// accessible. Otherwise fall back to a fixed-bottom overlay for
	// standalone use.
	panel := doc.Call("createElement", "div")
	panel.Set("id", "controls-panel")
	panel.Set("innerHTML", "<style>"+panelCSS+"</style>"+controlsBody)
	buildModeSelect(panel) // the mode <select> derives from the mode registry
	footers := doc.Call("getElementsByTagName", "footer")
	var existingFooter js.Value
	if footers.Get("length").Int() > 0 {
		existingFooter = footers.Index(0)
	}
	if existingFooter.Truthy() {
		panel.Set("style", "color:#aaa;font-family:'B612 Mono',monospace;font-size:12px;padding:8px 12px;background:rgba(0,0,0,0.85);border-top:1px solid #333;")
		existingFooter.Call("appendChild", panel)
	} else {
		standalonePanel = true
		body.Call("appendChild", panel)
		wireDockButtons()
		initDockResize()
		initFloatWindow()
		applyDock(readDockPref())
	}

	// Refresh DOM
	doc = js.Global().Get("document")
	body = doc.Get("body")
	injectFonts() // embedded @font-face rules for the panel / LED / header fonts

	// Get control element references
	rtc = doc.Call("getElementById", "runtime")
	cameraControl = doc.Call("getElementById", "camera-zoom")
	rotationControlsX = doc.Call("getElementById", "rotation-controls-x")
	rotationControlsY = doc.Call("getElementById", "rotation-controls-y")
	rotationControlsZ = doc.Call("getElementById", "rotation-controls-z")
	sliderZoom = doc.Call("getElementById", "slider-value-zoom")
	sliderX = doc.Call("getElementById", "slider-value-x")
	sliderY = doc.Call("getElementById", "slider-value-y")
	sliderZ = doc.Call("getElementById", "slider-value-z")

	// ── Rotation knobs (digital-pot style, one per axis) ──────────────
	// Turning a knob sets that axis's absolute angle (position) and zeroes
	// its spin rate — "the speed obeys the knob". Moving the rate slider
	// spins the axis and the knob pointer/LED track it live — "the knob
	// obeys the speed". Faithful to Glen's 3D projective unit, whose panel
	// pots set absolute X/Y angles on 7-seg displays.
	knobPtr[0] = doc.Call("getElementById", "knobptr-x")
	knobPtr[1] = doc.Call("getElementById", "knobptr-y")
	knobPtr[2] = doc.Call("getElementById", "knobptr-z")
	knobLED[0] = doc.Call("getElementById", "led-x")
	knobLED[1] = doc.Call("getElementById", "led-y")
	knobLED[2] = doc.Call("getElementById", "led-z")
	knobsReady = true
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
		kn := doc.Call("getElementById", knobID)
		if !kn.Truthy() {
			return
		}
		kn.Call("addEventListener", "pointerdown", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
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
		}))
		// Scroll over the angle ring nudges the pose by 5° per notch.
		kn.Call("addEventListener", "wheel", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			e := args[0]
			e.Call("preventDefault")
			e.Call("stopPropagation")
			step := float32(math.Pi / 36) // 5°
			if e.Get("deltaY").Float() > 0 {
				step = -step
			}
			addAngleAxis(axis, step)
			return nil
		}))
	}
	attachKnob("knob-x", 0, rotationControlsX, sliderX)
	attachKnob("knob-y", 1, rotationControlsY, sliderY)
	attachKnob("knob-z", 2, rotationControlsZ, sliderZ)
	doc.Call("addEventListener", "pointermove", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if knobAxis < 0 {
			return nil
		}
		cur := knobAngleAt(args[0])
		d := cur - knobPrevAng
		for d > math.Pi { // shortest-arc delta so it turns endlessly
			d -= 2 * math.Pi
		}
		for d < -math.Pi {
			d += 2 * math.Pi
		}
		knobPrevAng = cur
		addAngleAxis(knobAxis, float32(d))
		return nil
	}))
	knobRelease := trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		knobAxis = -1
		return nil
	})
	doc.Call("addEventListener", "pointerup", knobRelease)
	doc.Call("addEventListener", "pointercancel", knobRelease)
	updateRotKnobs()
	initKnobDrag() // document listeners for the bounded param/camera knobs

	// Knobify the fixed sliders too (zoom, speed, line, trail, spin rates):
	// hide each range input and insert a bounded knob that drives it. Reuses
	// each slider's existing 'input' handler, so behavior is unchanged.
	knobifyFixed := func(sliderID, numID string, dial bool) js.Value {
		sl := doc.Call("getElementById", sliderID)
		if !sl.Truthy() {
			return js.Undefined()
		}
		num := doc.Call("getElementById", numID)
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
	// degree dial — a second scale would collide).
	knobifyFixed("camera-zoom", "slider-value-zoom", true)
	knobifyFixed("pan-x", "slider-value-panx", true)
	knobifyFixed("pan-y", "slider-value-pany", true)
	knobifyFixed("speed-slider", "slider-value-speed", true)
	knobifyFixed("line-width", "slider-value-line", true)
	knobifyFixed("trail-slider", "slider-value-trail", true)
	knobifyFixed("rainbow-freq", "slider-value-rfreq", true)
	rkx := knobifyFixed("rotation-controls-x", "slider-value-x", false)
	rky := knobifyFixed("rotation-controls-y", "slider-value-y", false)
	rkz := knobifyFixed("rotation-controls-z", "slider-value-z", false)

	// Stack each axis's angle knob (outer ring) around its spin-rate knob
	// (inner), oscilloscope-style, with an analog degree dial around the ring.
	// Beside it, a digital readout column: the degrees LED, then the spin-rate
	// numeric directly below it, then reset. The cell label becomes "<axis> / Rate".
	// Each axis is a NARROW vertical strip: label ("X / Rate") · degrees LED ·
	// angle knob (in a square, with the reset tucked in its lower-right corner)
	// · spin-rate numeric. This sets the minimum module slot width.
	stackAxis := func(axisLbl, angleID, ledID, rateNumID, rateRstID string, rateKnob js.Value) {
		ak := doc.Call("getElementById", angleID)
		if !ak.Truthy() || !rateKnob.Truthy() {
			return
		}
		led := doc.Call("getElementById", ledID)
		rateNum := doc.Call("getElementById", rateNumID)
		rst := doc.Call("getElementById", rateRstID)
		cell := ak.Call("closest", ".pcell")
		angleGrp := ak.Get("parentNode")
		var rateGrp js.Value
		if rateNum.Truthy() {
			rateGrp = rateNum.Get("parentNode")
		}

		stack := stackKnobs(ak, rateKnob)
		addAngleDial(stack)

		// square wrapper so the reset can sit in the knob's corner
		knobBox := doc.Call("createElement", "span")
		knobBox.Set("className", "axknob-box")
		knobBox.Call("appendChild", stack)
		if rst.Truthy() {
			rst.Get("classList").Call("add", "axknob-rst")
			knobBox.Call("appendChild", rst)
		}

		col := doc.Call("createElement", "span")
		col.Set("className", "grp axstack")
		// Top line: axis label ("X") to the LEFT of the degrees LED readout.
		topRow := doc.Call("createElement", "span")
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
		col.Call("appendChild", topRow)
		col.Call("appendChild", knobBox)
		// Bottom line: "Rate" label to the LEFT of the spin-rate numeric.
		botRow := doc.Call("createElement", "span")
		botRow.Set("className", "grp axrow botrow")
		rlbl := doc.Call("createElement", "span")
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

	// "Front" switch: draw the model in front of the controls. Raising the
	// canvas above the panel and making it pointer-transparent lets the model
	// overlap the controls while clicks/drags still reach them (drag-rotate
	// is bound to the document, so it keeps working through the canvas).
	if mf := doc.Call("getElementById", "model-front"); mf.Truthy() {
		mf.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			cont := doc.Call("getElementById", "gocanvas-container")
			if !cont.Truthy() {
				return nil
			}
			st := cont.Get("style")
			if mf.Get("checked").Bool() {
				st.Set("zIndex", "var(--z-canvas-front)")
				st.Set("pointerEvents", "none")
			} else {
				st.Set("zIndex", "var(--z-canvas)")
				st.Set("pointerEvents", "auto")
				// Front off ⇒ normal layering; drop the recovery-raise so stacking
				// is predictable next time. The panel falls back to the CSS base
				// var(--z-panel)=10 (> canvas) — never to auto(0).
				body.Get("classList").Call("remove", "panel-raised")
			}
			return nil
		}))
	}

	if sf := doc.Call("getElementById", "spect-fill"); sf.Truthy() {
		sf.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			spectFill = sf.Get("checked").Bool()
			return nil
		}))
	}

	// Floating show/hide button for the whole control panel (it can block the
	// view). Lives outside the panel so it can bring it back.
	panelToggle := doc.Call("createElement", "button")
	panelToggle.Set("textContent", "▤")
	panelToggle.Set("title", "Show / hide controls (brings them back if the model's 'Front' overlay is hiding them)")
	panelToggle.Set("style", "position:fixed;bottom:6px;left:6px;z-index:var(--z-toggle);background:#222;color:#ccc;border:1px solid #555;border-radius:3px;font-family:'B612 Mono',monospace;font-size:14px;cursor:pointer;padding:2px 8px;opacity:0.55;")
	panelToggle.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		p := doc.Call("getElementById", "controls-panel")
		if !p.Truthy() {
			return nil
		}
		st := p.Get("style")
		hidden := st.Get("display").String() == "none"
		// The recovery-raise is ONE bit: body.panel-raised. CSS lifts the panel
		// AND its chrome (resize strip, float grip, dock buttons) together, so
		// the whole recovery unit surfaces above the Front canvas as a piece.
		cl := body.Get("classList")
		raised := cl.Call("contains", "panel-raised").Bool()
		frontOn := false
		if mf := doc.Call("getElementById", "model-front"); mf.Truthy() {
			frontOn = mf.Get("checked").Bool()
		}
		// Recover whenever the panel is hidden OR the "Front" canvas is drawn over
		// it — otherwise an opaque backdrop with Front on buries the panel and
		// toggling display alone can never bring it back. Raise it above the Front
		// canvas so one press always restores it; otherwise hide it so the model
		// can be viewed unobstructed.
		if hidden || (frontOn && !raised) {
			st.Set("display", "")
			cl.Call("add", "panel-raised")
		} else {
			st.Set("display", "none")
			cl.Call("remove", "panel-raised")
		}
		return nil
	}))
	body.Call("appendChild", panelToggle)

	// "Size / Step × / Fine ×" — one 3-layer concentric knob: the outermost
	// ring is Size (scales every knob via --kscale), the middle ring is Step×
	// (scales every param knob's coarse step), the inner knob is Fine× (the
	// inner disc's fraction of a coarse step). Step×/Fine× are read live by the
	// param knobs.
	sr := doc.Call("getElementById", "step-ratio")
	fr := doc.Call("getElementById", "fine-ratio")
	ks := doc.Call("getElementById", "knob-size")
	if sr.Truthy() && fr.Truthy() && ks.Truthy() {
		if holder := doc.Call("getElementById", "stepfine-stack"); holder.Truthy() {
			// Two concentric clickable label rings: Step× (outer) · Fine× (inner).
			// Size moved to its own Style module so this stays compact.
			sr.Set("title", "Step × — coarse step-size multiplier for every parameter knob")
			fr.Set("title", "Fine × — fine-trim step as a fraction of one coarse step")
			stepFine := stackKnobs(makeSelectorKnob(sr), makeSelectorKnob(fr))
			addSelectorLabels(stepFine, []string{".25", ".5", "1", "2", "5"}, sr, 43)
			addSelectorLabels(stepFine, []string{"1", ".1", ".01", ".001"}, fr, 31)
			holder.Call("appendChild", stepFine)
			sr.Get("style").Set("display", "none")
			fr.Get("style").Set("display", "none")
			// Separate LED readouts for Step× and Fine× (mirror the label rings).
			// Fixed decimals so the readouts are constant width (zero-padded) and
			// never resize as the ratios change.
			sled := doc.Call("getElementById", "step-led")
			fled := doc.Call("getElementById", "fine-led")
			fmtF := func(s string, dec int) string {
				v, _ := strconv.ParseFloat(s, 64)
				return strconv.FormatFloat(v, 'f', dec, 64)
			}
			upd := func() {
				if sled.Truthy() {
					sled.Set("textContent", fmtF(sr.Get("value").String(), 2)) // 0.25 … 5.00
				}
				if fled.Truthy() {
					fled.Set("textContent", fmtF(fr.Get("value").String(), 3)) // 1.000 … 0.001
				}
			}
			sr.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} { upd(); return nil }))
			fr.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} { upd(); return nil }))
			upd()
		}
		// Size knob lives in its own Style module (a lone labeled selector ring).
		if sh := doc.Call("getElementById", "size-stack"); sh.Truthy() {
			ks.Set("title", "Size — scales the whole control interface (S / M / L / XL)")
			sh.Call("appendChild", singleSelectorKnob(ks, []string{"S", "M", "L", "XL"}, 46))
			ks.Get("style").Set("display", "none")
		}
		// Knob-style selector (outer) with the LED-color selector stacked as its
		// inner ring — appearance + readout color on one shaft, saving a cell.
		if st := doc.Call("getElementById", "knob-style"); st.Truthy() {
			lc := doc.Call("getElementById", "led-color")
			st.Set("title", "Knob style — knob face appearance (std / flat / vint / chrome / gold / carbon)")
			if kh := doc.Call("getElementById", "knobstyle-stack"); kh.Truthy() {
				if lc.Truthy() {
					lc.Set("title", "LED color — readout color for every numeric LED readout")
					// Populate the LED-color options from the single ordered source.
					var dotCols []string
					for _, d := range ledColorDefs {
						o := doc.Call("createElement", "option")
						o.Set("value", d.name)
						o.Set("textContent", d.name)
						lc.Call("appendChild", o)
						dotCols = append(dotCols, d.col)
					}
					// Outer = knob style (labeled ring). Inner = LED color: a ring of
					// colored dots (one per option in its own LED color), the selected
					// one highlighted.
					stStack := stackKnobs(makeSelectorKnob(st, styleKnobRot), makeSelectorKnob(lc))
					addSelectorLabels(stStack, []string{"std", "flat", "vint", "chrm", "gold", "carb"}, st, 46, styleKnobRot) // labels staggered off the LED dots; pointer offset to match
					addSelectorDotLabels(stStack, dotCols, lc, 36)                                                            // ring just outside the style knob, inside its text labels
					kh.Call("appendChild", stStack)
				} else {
					kh.Call("appendChild", singleSelectorKnob(st, []string{"std", "flat", "vint", "chrm", "gold", "carb"}, 44))
				}
			}
			applyStyle := func() {
				cl := doc.Call("getElementById", "controls-panel").Get("classList")
				for _, s := range []string{"ks-std", "ks-flat", "ks-vint", "ks-chrome", "ks-gold", "ks-carbon"} {
					cl.Call("remove", s)
				}
				cl.Call("add", "ks-"+st.Get("value").String())
			}
			st.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
				applyStyle()
				return nil
			}))
			applyStyle()
			st.Get("style").Set("display", "none")
			// LED readout color presets → CSS vars on the panel.
			if lc.Truthy() {
				ledCols := map[string][4]string{}
				for _, d := range ledColorDefs {
					ledCols[d.name] = [4]string{d.col, d.glow, d.bg, d.bd}
				}
				applyLED := func() {
					c := ledCols[lc.Get("value").String()]
					s := doc.Call("getElementById", "controls-panel").Get("style")
					s.Call("setProperty", "--led-col", c[0])
					s.Call("setProperty", "--led-glow", c[1])
					s.Call("setProperty", "--led-bg", c[2])
					s.Call("setProperty", "--led-bd", c[3])
				}
				// Readout below the knob: an "LED" label to the left of a color-name
				// display (shown in the current LED color), like a labeled readout.
				var ledRO js.Value
				if kh := doc.Call("getElementById", "knobstyle-stack"); kh.Truthy() {
					row := doc.Call("createElement", "span")
					row.Set("className", "ledcolor-row")
					lab := doc.Call("createElement", "span")
					lab.Set("className", "plabel ledcolor-lbl")
					lab.Set("textContent", "LED")
					ledRO = doc.Call("createElement", "span")
					ledRO.Set("className", "selk-readout ledcolor-ro")
					row.Call("appendChild", lab)
					row.Call("appendChild", ledRO)
					kh.Get("parentNode").Call("appendChild", row)
				}
				updLEDRO := func() {
					if ledRO.Truthy() {
						idx := lc.Get("selectedIndex").Int()
						if idx < 0 {
							idx = 0
						}
						ledRO.Set("textContent", lc.Get("options").Index(idx).Get("text").String())
					}
				}
				lc.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} { applyLED(); updLEDRO(); return nil }))
				applyLED()
				updLEDRO()
				lc.Get("style").Set("display", "none")
			}
		}
		sr.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			if v, err := strconv.ParseFloat(sr.Get("value").String(), 64); err == nil && v > 0 {
				coarseRatio = v
			}
			return nil
		}))
		fr.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			if v, err := strconv.ParseFloat(fr.Get("value").String(), 64); err == nil && v > 0 {
				fineRatio = v
			}
			return nil
		}))
		applyKS := func() {
			if v, err := strconv.ParseFloat(ks.Get("value").String(), 64); err == nil {
				setKScale(v)
			}
		}
		ks.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			applyKS()
			return nil
		}))
		// Restore a saved (continuous) interface scale from a prior resize-drag;
		// otherwise honor the Size knob's default.
		if ls := js.Global().Get("localStorage"); ls.Truthy() {
			if s := ls.Call("getItem", "wasmstuff-kscale"); s.Truthy() {
				if v, err := strconv.ParseFloat(s.String(), 64); err == nil && v > 0 {
					setKScale(v)
				} else {
					applyKS()
				}
			} else {
				applyKS()
			}
		} else {
			applyKS()
		}
	}

	// Event: mode change
	doc.Call("getElementById", "mode-select").Call("addEventListener", "change", trackedFuncOf(onModeChange))

	// Event: color pickers
	colorCallback := trackedFuncOf(onColorChange)
	doc.Call("getElementById", "color-base").Call("addEventListener", "input", colorCallback)
	doc.Call("getElementById", "color-mid").Call("addEventListener", "input", colorCallback)
	doc.Call("getElementById", "color-top").Call("addEventListener", "input", colorCallback)
	attachColorKnobs() // Hue/Sat/Val knob under each color swatch

	// Event: per-control reset buttons for colors
	doc.Call("getElementById", "rst-color-base").Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		baseColor = [3]float32{1.0, 0.0, 0.0}
		doc.Call("getElementById", "color-base").Set("value", "#ff0000")
		gl.Call("uniform3f", uBaseColorLoc, baseColor[0], baseColor[1], baseColor[2])
		return nil
	}))
	doc.Call("getElementById", "rst-color-mid").Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		midColor = [3]float32{0.0, 1.0, 0.0}
		doc.Call("getElementById", "color-mid").Set("value", "#00ff00")
		gl.Call("uniform3f", uMidColorLoc, midColor[0], midColor[1], midColor[2])
		return nil
	}))
	doc.Call("getElementById", "rst-color-top").Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		topColor = [3]float32{0.0, 0.0, 1.0}
		doc.Call("getElementById", "color-top").Set("value", "#0000ff")
		gl.Call("uniform3f", uTopColorLoc, topColor[0], topColor[1], topColor[2])
		return nil
	}))

	// Event: reset all button
	doc.Call("getElementById", "reset-all-btn").Call("addEventListener", "click", trackedFuncOf(onResetAll))

	// Any reset button (↺) — re-sync every knob pointer to its freshly-reset
	// value on the next frame (after the button's own handler has set values),
	// so knobs whose handler sets the slider without dispatching 'input' still
	// snap their pointer back.
	resetSync := trackedFuncOf(func(this js.Value, args []js.Value) interface{} { syncKnobs(); return nil })
	doc.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if t := a[0].Get("target"); t.Truthy() && t.Call("closest", ".rst").Truthy() {
			js.Global().Call("requestAnimationFrame", resetSync)
		}
		return nil
	}))

	// Event: normalize — reorient the current model to the default
	// (identity) pose and stop any slider-driven spin.
	doc.Call("getElementById", "normalize-btn").Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		normalizeOrientation()
		return nil
	}))

	// Event: Edit eqn — load the current attractor's equations into the
	// editable Custom mode (if we have parseable forms for it) and switch.
	doc.Call("getElementById", "edit-eq-sw").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		sw := doc.Call("getElementById", "edit-eq-sw")
		s := doc.Call("getElementById", "mode-select")
		if sw.Get("checked").Bool() {
			if selectedMode != "custom" {
				preCustomMode = selectedMode // remember where to return
			}
			seedCustomFromMode(preCustomMode)
			s.Set("value", "custom")
		} else {
			back := preCustomMode
			if back == "" || back == "custom" {
				back = "lorenz"
			}
			s.Set("value", back)
		}
		s.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		return nil
	}))

	// Event: speed slider
	// (Speed's input/reset wiring is owned by its ControlDesc registration.)

	// Event: auto-rotate switch — fold the auto-spin into the Y rate knob.
	doc.Call("getElementById", "auto-rotate").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		setAutoRotate(doc.Call("getElementById", "auto-rotate").Get("checked").Bool())
		return nil
	}))

	// Event: spectrogram-skin checkbox — paint the live spectrogram onto
	// the current surface model (sphere/globe/torus).
	doc.Call("getElementById", "spectro-skin").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		spectroSkin = doc.Call("getElementById", "spectro-skin").Get("checked").Bool()
		skinDirty = true
		generateForMode(selectedMode)
		return nil
	}))

	// Event: Backdrop — two mutually-exclusive 2-position switches (Spectro bg /
	// XY bg) drive the hidden #bg-visual select (turning one on turns the other
	// off; turning the active one off clears the backdrop).
	if bv := doc.Call("getElementById", "bg-visual"); bv.Truthy() {
		spectro := doc.Call("getElementById", "bg-spectro")
		xy := doc.Call("getElementById", "bg-xy")
		syncBackdrop := func() {
			v := bv.Get("value").String()
			if spectro.Truthy() {
				spectro.Set("checked", v == "spectrogram")
			}
			if xy.Truthy() {
				xy.Set("checked", v == "xy")
			}
		}
		bv.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			setBackgroundVisual(bv.Get("value").String())
			syncBackdrop()
			return nil
		}))
		wireBg := func(sw js.Value, mode string) {
			if !sw.Truthy() {
				return
			}
			sw.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
				if sw.Get("checked").Bool() {
					bv.Set("value", mode)
				} else if bv.Get("value").String() == mode {
					bv.Set("value", "")
				}
				bv.Call("dispatchEvent", js.Global().Get("Event").New("change"))
				return nil
			}))
		}
		wireBg(spectro, "spectrogram")
		wireBg(xy, "xy")
		syncBackdrop()
	}

	// Phosphor selector — populate from the phosphor table + set phosphorIdx.
	// Lives in the Style module as a rotary knob with a name readout (too many
	// options / too-long names for a label ring).
	if ph := doc.Call("getElementById", "phosphor"); ph.Truthy() {
		for i, p := range phosphors {
			opt := doc.Call("createElement", "option")
			opt.Set("value", strconv.Itoa(i))
			opt.Set("textContent", p.name)
			ph.Call("appendChild", opt)
		}
		ph.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			if v, err := strconv.Atoi(ph.Get("value").String()); err == nil {
				phosphorIdx = v
			}
			// Selecting a phosphor IS how you enter CRT mode now: the trace is
			// drawn monochrome on that phosphor, so the gradient source + palette
			// colors are overridden (and dimmed).
			crtMode = phosphorIdx > 0
			updateCRTOverlay()
			updateCRTDim()
			return nil
		}))
		ph.Set("title", "Phosphor — CRT trace color + afterglow for scope modes (P31 crisp green … P7 blue→green … P33 long amber)")
		if holder := doc.Call("getElementById", "phosphor-stack"); holder.Truthy() {
			pk := selectorKnobReadout(ph)
			holder.Call("appendChild", pk)
			addPhosphorTraces(pk.Call("querySelector", ".knobstack"), ph)
		}
	}

	// Event: audio-mod checkbox — enable per-parameter audio modulation.
	// The per-parameter routing controls appear under each attractor
	// parameter (built by buildParamPanel) while this is checked.
	doc.Call("getElementById", "audio-mod").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		setAudioMod(doc.Call("getElementById", "audio-mod").Get("checked").Bool())
		return nil
	}))

	// Event: Meters switch — show/hide the top-left audio feature meters.
	doc.Call("getElementById", "show-meters").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		metersEnabled = doc.Call("getElementById", "show-meters").Get("checked").Bool()
		updateMetersVisibility()
		return nil
	}))

	// Event: function/signal generator — a client-side audio source that works
	// with no server or mic (so audio modulation / spectrogram / xy work on the
	// static site). Toggle + waveform + sweep-rate.
	doc.Call("getElementById", "fg-on").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		setFuncGen(doc.Call("getElementById", "fg-on").Get("checked").Bool())
		return nil
	}))
	buildGeneratorModule()
	buildSonifyModule()

	// Template legend module + its Window-group toggle.
	buildTemplateModule()
	doc.Call("getElementById", "tpl-on").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		setTemplate(doc.Call("getElementById", "tpl-on").Get("checked").Bool())
		return nil
	}))

	// Event: built-in test-tone generator (loops through the server's capture).
	doc.Call("getElementById", "test-tone").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		setTestTone(doc.Call("getElementById", "test-tone").Get("checked").Bool())
		return nil
	}))

	// Event: points/line toggle
	doc.Call("getElementById", "use-points").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		usePoints = doc.Call("getElementById", "use-points").Get("checked").Bool()
		if usePoints {
			attractorDrawMode = glTypes.Points
		} else {
			attractorDrawMode = glTypes.LineStrip
		}
		return nil
	}))

	// Event: trail length slider
	// (Trail's input/reset wiring is owned by its ControlDesc registration.)

	// Event: ring-trail switch — beam model on/off (re-primes on enable).
	if rs := doc.Call("getElementById", "ring-sw"); rs.Truthy() {
		rs.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			ringOn = rs.Get("checked").Bool()
			ringInvalidate()
			return nil
		}))
	}

	// Events: in-app recorder, jam mode, WebMIDI.
	wireRecordSwitch()
	wireJamSwitch()
	wireMIDISwitch()

	// Event: patchbay module visibility.
	if ps := doc.Call("getElementById", "patch-on"); ps.Truthy() {
		ps.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			patchOn = ps.Get("checked").Bool()
			buildParamPanel(selectedMode)
			return nil
		}))
	}

	// Event: twin-trajectory switch + λ readout.
	wireTwinSwitch()
	// Event: Poincaré-section switch.
	wireSectSwitch()

	// Event: persist trail checkbox
	doc.Call("getElementById", "persist-trail").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		persistTrail = doc.Call("getElementById", "persist-trail").Get("checked").Bool()
		return nil
	}))

	// Create info overlay div
	infoOverlay := doc.Call("createElement", "div")
	infoOverlay.Set("id", "info-overlay")
	infoOverlay.Set("style", "display:none;position:fixed;top:140px;left:20px;right:20px;z-index:var(--z-hud);"+
		"color:rgba(255,255,255,0.85);font-family:'B612 Mono',monospace;font-size:14px;line-height:1.6;"+
		"white-space:pre-wrap;pointer-events:none;text-shadow:0 0 10px #000,0 0 20px #000;"+
		"max-width:600px;")
	body.Call("appendChild", infoOverlay)

	// Event: show info checkbox
	doc.Call("getElementById", "show-info").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		checked := doc.Call("getElementById", "show-info").Get("checked").Bool()
		overlay := doc.Call("getElementById", "info-overlay")
		if checked {
			if desc, ok := attractorDescriptions[selectedMode]; ok {
				overlay.Set("textContent", desc)
			} else {
				overlay.Set("textContent", selectedMode)
			}
			overlay.Get("style").Set("display", "block")
			positionInfoOverlay()
		} else {
			overlay.Get("style").Set("display", "none")
		}
		return nil
	}))

	// Event: background color picker. Alpha=0 keeps the canvas
	// transparent so the host page's background (e.g. m2's SVG logo)
	// shows through — picking a non-black bg here only tints what's
	// drawn, it doesn't paint over the host.
	doc.Call("getElementById", "color-bg").Call("addEventListener", "input", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		hex := doc.Call("getElementById", "color-bg").Get("value").String()
		bgColor[0], bgColor[1], bgColor[2] = hexToRGB(hex)
		gl.Call("clearColor", bgColor[0], bgColor[1], bgColor[2], 0)
		return nil
	}))
	doc.Call("getElementById", "rst-color-bg").Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		bgColor = [3]float32{0, 0, 0}
		doc.Call("getElementById", "color-bg").Set("value", "#000000")
		gl.Call("clearColor", 0, 0, 0, 0)
		return nil
	}))

	// Event: gradient source + colors selectors (each driven by a rotary knob)
	doc.Call("getElementById", "gradient-source").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if v, err := strconv.Atoi(doc.Call("getElementById", "gradient-source").Get("value").String()); err == nil {
			gradientSource = v
		}
		return nil
	}))
	doc.Call("getElementById", "gradient-colors").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if v, err := strconv.Atoi(doc.Call("getElementById", "gradient-colors").Get("value").String()); err == nil {
			gradientColors = v
		}
		updateGradientUI()
		return nil
	}))
	doc.Call("getElementById", "gradient-reverse").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		gradientReverse = doc.Call("getElementById", "gradient-reverse").Get("checked").Bool()
		return nil
	}))

	// Event: pause button
	doc.Call("getElementById", "pause-sw").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		paused = doc.Call("getElementById", "pause-sw").Get("checked").Bool()
		return nil
	}))

	// Power switch (default on): OFF stops the render loop and clears the model
	// (canvas) to save GPU, but KEEPS the control panel; ON resumes rendering.
	// Power is now the model selector's "OFF" position (the outer category knob's
	// 6th detent) — see setPowerState, driven from buildNestedModelSelector.

	// Fullscreen switch — on requests fullscreen, off exits. Kept in sync with
	// the actual fullscreen state (fullscreenchange fires on Esc etc.).
	// requestFullscreen/exitFullscreen return a promise that REJECTS when the
	// browser refuses (no user gesture, a permissions-policy block, or some
	// mobile/embedded contexts → "Permissions check failed"). Swallow that with
	// a .catch and re-sync the switch to reality, so it never surfaces as an
	// unhandled promise rejection.
	fsReject := trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if sw := doc.Call("getElementById", "fullscreen-sw"); sw.Truthy() {
			sw.Set("checked", doc.Get("fullscreenElement").Truthy() || doc.Get("webkitFullscreenElement").Truthy())
		}
		return nil
	})
	catchFs := func(pr js.Value) {
		if pr.Truthy() && !pr.Get("then").IsUndefined() {
			pr.Call("catch", fsReject)
		}
	}
	doc.Call("getElementById", "fullscreen-sw").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		want := doc.Call("getElementById", "fullscreen-sw").Get("checked").Bool()
		if want {
			docEl := doc.Get("documentElement")
			if !docEl.Get("requestFullscreen").IsUndefined() {
				catchFs(docEl.Call("requestFullscreen"))
			} else if !docEl.Get("webkitRequestFullscreen").IsUndefined() {
				catchFs(docEl.Call("webkitRequestFullscreen"))
			}
		} else {
			if !doc.Get("exitFullscreen").IsUndefined() {
				catchFs(doc.Call("exitFullscreen"))
			} else if !doc.Get("webkitExitFullscreen").IsUndefined() {
				catchFs(doc.Call("webkitExitFullscreen"))
			}
		}
		return nil
	}))
	syncFsSwitch := trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if sw := doc.Call("getElementById", "fullscreen-sw"); sw.Truthy() {
			sw.Set("checked", doc.Get("fullscreenElement").Truthy() || doc.Get("webkitFullscreenElement").Truthy())
		}
		return nil
	})
	doc.Call("addEventListener", "fullscreenchange", syncFsSwitch)
	doc.Call("addEventListener", "webkitfullscreenchange", syncFsSwitch)

	// Event: screenshot button
	doc.Call("getElementById", "screenshot-btn").Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		dataURL := canvasEl.Call("toDataURL", "image/png")
		link := doc.Call("createElement", "a")
		link.Set("download", "attractor.png")
		link.Set("href", dataURL)
		link.Call("click")
		return nil
	}))

	// isInteractiveDragTarget returns true when the mousedown/touchstart
	// target is something the user is clicking deliberately (button,
	// link, form input, the controls panel itself) — in those cases we
	// must NOT hijack the click to start a drag.
	isInteractiveDragTarget := func(target js.Value) bool {
		if !target.Truthy() {
			return false
		}
		if closest := target.Get("closest"); closest.Type() == js.TypeFunction {
			match := target.Call("closest", "a, button, input, label, select, textarea, #controls-panel, [data-no-drag]")
			if match.Truthy() {
				return true
			}
		}
		return false
	}

	// Event: mouse drag rotation. Bound to document (not canvasEl) so
	// rotation still works when the host page paints other elements
	// (e.g. magnetosphere.net's SVG logo) above the canvas. The target
	// filter above lets clicks on links/buttons/inputs through.
	doc.Call("addEventListener", "mousedown", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		e := args[0]
		if isInteractiveDragTarget(e.Get("target")) {
			return nil
		}
		dragging = true
		beginDrag(e.Get("clientX").Float(), e.Get("clientY").Float())
		return nil
	}))
	js.Global().Call("addEventListener", "mousemove", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if !dragging {
			return nil
		}
		e := args[0]
		dragMove(e.Get("clientX").Float(), e.Get("clientY").Float())
		return nil
	}))
	js.Global().Call("addEventListener", "mouseup", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		dragging = false
		return nil
	}))

	// Event: touch drag rotation. Same doc-binding rationale as mouse.
	// Do NOT preventDefault here unconditionally — that breaks tap+drag
	// on host-page links/buttons. Only preventDefault when we're
	// actually starting a drag (target isn't interactive).
	doc.Call("addEventListener", "touchstart", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		e := args[0]
		if isInteractiveDragTarget(e.Get("target")) {
			return nil
		}
		e.Call("preventDefault")
		t := e.Get("touches").Index(0)
		dragging = true
		beginDrag(t.Get("clientX").Float(), t.Get("clientY").Float())
		return nil
	}))
	doc.Call("addEventListener", "touchmove", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if !dragging {
			return nil
		}
		e := args[0]
		e.Call("preventDefault")
		t := e.Get("touches").Index(0)
		dragMove(t.Get("clientX").Float(), t.Get("clientY").Float())
		return nil
	}))
	doc.Call("addEventListener", "touchend", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		dragging = false
		return nil
	}))

	// Event: scroll wheel zoom
	canvasEl.Call("addEventListener", "wheel", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		e := args[0]
		e.Call("preventDefault")
		// deltaY is ~100–130 per mouse notch; keep the per-notch zoom step
		// small (~2–3) while still scaling gently on fine trackpads.
		delta := float32(e.Get("deltaY").Float()) * 0.02
		zoomVal := float32(js.Global().Get("parseFloat").Invoke(cameraControl.Get("value")).Float())
		zoomVal -= delta
		if zoomVal < -95 {
			zoomVal = -95
		}
		if zoomVal > 95 {
			zoomVal = 95
		}
		cameraControl.Set("value", strconv.FormatFloat(float64(zoomVal), 'f', 0, 64))
		cachedZoom = zoomVal // render loop reads the cache, not the DOM
		// Fire 'input' so the zoom knob pointer + numeric box track the wheel.
		cameraControl.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		return nil
	}))

	// Initial mode — read from URL hash if present. The hash may carry
	// permalink state after the mode ("#aizawa&p.a=1.19&..."); take only
	// the leading mode token here (the rest is applied post-setup).
	selectedMode = "globe"
	hash := js.Global().Get("location").Get("hash").String()
	if len(hash) > 1 {
		hashMode := hashModeToken()
		// Validate against the mode registry. (The old params-map + hardcoded
		// list pair silently rejected several real modes, e.g. sphere and fvf.)
		if knownMode(hashMode) {
			selectedMode = hashMode
		}
	}
	// Select the matching dropdown option
	sel := doc.Call("getElementById", "mode-select")
	if !sel.IsNull() && !sel.IsUndefined() {
		sel.Set("value", selectedMode)
	}
	buildParamPanel(selectedMode)
	updateTrailVisibility()

	// Model selector: two concentric detented rotary encoders — the outer ring
	// picks the category (attractors / polyhedra / geometry / audio / custom),
	// the inner knob the model within it. Two dropdowns below mirror them. The
	// hidden #mode-select stays the single source of truth for all the rest of
	// the code (permalink, param panel, render loop).
	selWindow = doc.Call("getElementById", "sel-window")
	updateSelWindow()
	initSelKnobDrag()
	buildNestedModelSelector()
	// Gradient source + palette: one concentric knob (source ring outside,
	// number-of-colors inside), with both dropdowns in a column beside it.
	gsrc := doc.Call("getElementById", "gradient-source")
	gcol := doc.Call("getElementById", "gradient-colors")
	if gsrc.Truthy() && gcol.Truthy() {
		if holder := doc.Call("getElementById", "gradient-stack"); holder.Truthy() {
			gstack := stackKnobs(makeSelectorKnob(gsrc), makeSelectorKnob(gcol))
			// Concentric clickable labels replace the dropdowns: source (what the
			// color follows) on the OUTER ring, palette (color count) on the INNER.
			addSelectorLabels(gstack, []string{"X", "Y", "Z", "trl"}, gsrc, 43)
			addSelectorLabels(gstack, []string{"1", "2", "3", "∞"}, gcol, 31)
			holder.Call("appendChild", gstack)
			gsrc.Get("style").Set("display", "none")
			gcol.Get("style").Set("display", "none")
		}
	}
	updateGradientUI()

	// Initialize persistent JS typed arrays for zero-alloc frame uploads
	jsVertUint8 = js.Global().Get("Uint8Array").New(steps * 4 * 4)
	buf := jsVertUint8.Get("buffer")
	jsVertFloat = js.Global().Get("Float32Array").New(buf, 0, steps*4)

	// Initialize WebGL
	glTypes.New(gl)
	attractorDrawMode = glTypes.LineStrip
	// Bind buffers before setting up attrib pointers in setupShaders
	gl.Call("bindBuffer", glTypes.ArrayBuffer, attractorVertexBuffer)
	gl.Call("bindBuffer", glTypes.ElementArrayBuffer, attractorIndexBuffer)
	setupShaders()
	setupTexShaders()
	setupMatrices()
	generateForMode(selectedMode)
	if isSpectroSurface(selectedMode) {
		setSpectrogramCamera()
	} else {
		autoFitCamera()
	}
	refreshGradient()

	// Check if debug mode is enabled via JS global
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
	// other input.
	adoptDescControl(ControlDesc{ID: "camera-zoom", Label: "Zoom", Min: -95, Max: 95, Step: 1, Def: 0,
		Signed: true, PermaKey: "z", LEDID: "slider-value-zoom", ResetID: "rst-zoom",
		Apply: func(v float64) { cachedZoom = float32(v) },
		ResetExtra: func() {
			defaultCameraDist = initCameraDist
			updateViewMatrix()
			syncKnobs()
		}})
	adoptDescControl(ControlDesc{ID: "pan-x", Label: "X", Min: -8, Max: 8, Step: 1, Def: 0,
		Signed: true, PermaKey: "px", LEDID: "slider-value-panx", ResetID: "rst-panx",
		Apply: func(v float64) { cachedPanX = float32(v) }})
	adoptDescControl(ControlDesc{ID: "pan-y", Label: "Y", Min: -8, Max: 8, Step: 1, Def: 0,
		Signed: true, PermaKey: "py", LEDID: "slider-value-pany", ResetID: "rst-pany",
		Apply: func(v float64) { cachedPanY = float32(v) }})
	adoptDescControl(ControlDesc{ID: "rainbow-freq", Label: "period", Min: 0.05, Max: 20, Step: 0.05, Def: 1,
		PermaKey: "rf", LEDID: "slider-value-rfreq", ResetID: "rst-rfreq",
		Apply: func(v float64) { gradientFreq = float32(v) }})
	// Speed: the slider runs log10 (-2..2) while the LED shows the effective
	// multiplier (0.01..100), whole sub-step counts at ≥1 — the one mapping
	// pair below is the SSOT both directions.
	adoptDescControl(ControlDesc{ID: "speed-slider", Label: "Speed", Min: -2, Max: 2, Step: 0.1, Def: 0,
		PermaKey: "sp", LEDID: "slider-value-speed", ResetID: "rst-speed",
		Apply:       applySpeedLog,
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
			gl.Call("lineWidth", v)
		}})
	adoptDescControl(ControlDesc{ID: "trail-slider", Label: "Trail", Min: 1000, Max: 500000, Step: 1000, Def: 20000,
		PermaKey: "tr", LEDID: "slider-value-trail", ResetID: "rst-trail",
		Apply: func(v float64) {
			newSteps := int(v)
			if newSteps != steps {
				steps = newSteps
				vertBuf = make([]float32, steps*4)
				jsVertUint8 = js.Global().Get("Uint8Array").New(steps * 4 * 4)
				buf := jsVertUint8.Get("buffer")
				jsVertFloat = js.Global().Get("Float32Array").New(buf, 0, steps*4)
				resetAttractorState()
				refreshGradient()
			}
		},
		ResetExtra: func() {
			// Resetting the trail also drops persist mode (matches the old
			// bespoke reset: a persisted trail makes the new length invisible).
			persistTrail = false
			doc.Call("getElementById", "persist-trail").Set("checked", false)
		}})
	// Model Out (sonification): trace-rate knob on the generators' concert-
	// pitch semitone scale (the LED shows Hz both directions via the mapping
	// pair), plus output level. The MAP ring is wired in buildSonifyModule.
	adoptDescControl(ControlDesc{ID: "sonify-freq", Label: "trace", Min: 0, Max: float64(genSemitones), Step: 1, Def: 24,
		PermaKey: "sf", LEDID: "sonify-led", ResetID: "",
		Apply:       func(v float64) { sonifyHz = freqFromKnob(v) },
		SliderToVal: sonifyFreqFromSlider,
		ValToSlider: sonifySliderFromFreq,
		LEDMin:      genFreqLo, LEDMax: genFreqHi, LEDStep: 1})
	adoptDescControl(ControlDesc{ID: "sonify-lvl", Label: "lvl", Min: 0, Max: 100, Step: 1, Def: 60,
		PermaKey: "sv", LEDID: "sonify-lvl-led", ResetID: "",
		Apply: func(v float64) { sonifyLevel = v / 100 }})

	// View spin rates: the last controls on the legacy wiring path. Reset
	// also zeroes the axis ANGLE state and rebuilds the matrices (parity
	// with the old bespoke rst-rx/ry/rz handlers).
	adoptDescControl(ControlDesc{ID: "rotation-controls-x", Label: "X rate", Min: -1, Max: 1, Step: 0.1, Def: 0,
		Signed: true, PermaKey: "rx", LEDID: "slider-value-x", ResetID: "rst-rx",
		Apply: func(v float64) { cachedRotX = float32(v) },
		ResetExtra: func() {
			rotationX, rotationX1, angleX = 0, 0, 0
			rebuildModelMatrix()
			updateModelMatrix()
			updateRotKnobs()
			syncKnobs()
		}})
	adoptDescControl(ControlDesc{ID: "rotation-controls-y", Label: "Y rate", Min: -1, Max: 1, Step: 0.1, Def: 0,
		Signed: true, PermaKey: "ry", LEDID: "slider-value-y", ResetID: "rst-ry",
		Apply: func(v float64) { cachedRotY = float32(v) },
		ResetExtra: func() {
			rotationY, rotationY1, angleY = 0, 0, 0
			clearAutoRotateFlag() // Y spin (incl. auto) just zeroed
			rebuildModelMatrix()
			updateModelMatrix()
			updateRotKnobs()
		}})
	adoptDescControl(ControlDesc{ID: "rotation-controls-z", Label: "Z rate", Min: -1, Max: 1, Step: 0.1, Def: 0,
		Signed: true, PermaKey: "rz", LEDID: "slider-value-z", ResetID: "rst-rz",
		Apply: func(v float64) { cachedRotZ = float32(v) },
		ResetExtra: func() {
			rotationZ, rotationZ1, angleZ = 0, 0, 0
			rebuildModelMatrix()
			updateModelMatrix()
			updateRotKnobs()
		}})

	// Prime every registry control once: run its Apply from the DOM's current
	// value so engine state matches the panel by construction (the old code
	// did this ad hoc — applyLineWidth() at wiring, readSliderCache, …).
	for _, c := range builtControls {
		c.slider.Call("dispatchEvent", js.Global().Get("Event").New("input"))
	}

	// Permalink: capture pristine control defaults, restore any state
	// encoded in the URL hash, then keep the hash in sync with the live
	// state so the current view is always shareable.
	capturePermaDefaults()
	applyStateFromHash()
	startPermalinkSync()

	// Final tooltip pass now that every selector (gradient / model / style) is
	// built — some are created after the first buildParamPanel's annotate.
	annotateControlTooltips()

	// Start animation loop
	done := make(chan struct{})
	renderFrame = trackedFuncOf(renderLoop)
	js.Global().Call("requestAnimationFrame", renderFrame)

	// Set initial trail-controls visibility for the starting mode.
	updateTrailVisibility()

	// Wire input listeners for the fixed sliders so the cached vars
	// + visible text output stay in sync with user interaction. The
	// renderLoop reads cachedZoom/RotX/Y/Z instead of polling
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
		nav := doc.Call("getElementById", "extra-nav")
		if nav.Truthy() {
			nav.Set("innerHTML", ExtraNavHTML)
		}
	}

	// Inject a host-page CSS rule killing vertical scroll caused by
	// the controls panel growing the body height. Targets the actual
	// magnetosphere.net symptom (~1cm of overflow) without breaking
	// pages that legitimately scroll — Run() is only invoked on the
	// animation page.
	noScrollStyle := doc.Call("createElement", "style")
	noScrollStyle.Set("textContent", "html,body{overflow:hidden!important;margin:0;padding:0;}")
	doc.Get("head").Call("appendChild", noScrollStyle)

	// Random initial orientation + low-rate rotation so the model doesn't start
	// in the same pose every load — UNLESS the permalink pinned an explicit pose
	// (&rot / &drag), in which case that must win so a shared still-view link is
	// restored faithfully. Must run AFTER the rotation-controls-x/y/z elements
	// are created and queried.
	if !hashPinnedPose {
		randomizeOrientation()
		// randomizeOrientation zeroed the rate sliders — put back any spin
		// rates the permalink explicitly pinned (&rx/&ry/&rz).
		for ax, v := range hashPinnedSpin {
			if sl := doc.Call("getElementById", "rotation-controls-"+ax); sl.Truthy() {
				sl.Set("value", v)
				sl.Call("dispatchEvent", js.Global().Get("Event").New("input"))
			}
		}
		if len(hashPinnedSpin) > 0 {
			syncKnobs()
		}
	} else {
		// A pinned pose (&rot) is by construction a STILL view — rot is only
		// serialized when auto-rotate is off and all spin rates are zero — so
		// clear any residual spin rate (e.g. the auto-rotate Y contribution the
		// restored ar=0 subtracted from an unzeroed base) so the restored view
		// doesn't drift away from the shared pose.
		for _, ax := range []string{"x", "y", "z"} {
			if sl := doc.Call("getElementById", "rotation-controls-"+ax); sl.Truthy() && sl.Get("value").String() != "0" {
				sl.Set("value", "0")
				sl.Call("dispatchEvent", js.Global().Get("Event").New("input"))
			}
		}
	}
	// The spectrogram wants a static, face-on default instead — undo the
	// randomized pose/spin for an initial #spectrogram load (mode switches
	// go through onModeChange, which already handles this).
	if isSpectroSurface(selectedMode) {
		setSpectrogramCamera()
	}

	// randomizeOrientation zeroed the spin rates; if auto-rotate is on
	// (default, or per the permalink), re-apply its Y-rate contribution so
	// the model actually spins and the Y rate knob reflects it.
	if selectedMode != "spectrogram" && selectedMode != "fvf" && autoRotate {
		autoRotate = false
		setAutoRotate(true)
	}

	// Wheel-on-input: scroll over a range/number input adjusts its
	// value by `step` instead of scrolling the host page. Fires the
	// input event so the existing listener pipelines react.
	bindWheelEl := func(el js.Value) {
		if !el.Truthy() {
			return
		}
		el.Call("addEventListener", "wheel", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			e := args[0]
			e.Call("preventDefault")
			deltaY := e.Get("deltaY").Float()
			step, _ := strconv.ParseFloat(el.Get("step").String(), 64)
			if step == 0 {
				step = 1
			}
			cur, _ := strconv.ParseFloat(el.Get("value").String(), 64)
			if deltaY < 0 {
				cur += step
			} else {
				cur -= step
			}
			if minS := el.Get("min").String(); minS != "" {
				if mn, err := strconv.ParseFloat(minS, 64); err == nil && cur < mn {
					cur = mn
				}
			}
			if maxS := el.Get("max").String(); maxS != "" {
				if mx, err := strconv.ParseFloat(maxS, 64); err == nil && cur > mx {
					cur = mx
				}
			}
			el.Set("value", strconv.FormatFloat(cur, 'f', -1, 64))
			evtInit := js.Global().Get("Object").New()
			evtInit.Set("bubbles", true)
			evt := js.Global().Get("Event").New("input", evtInit)
			el.Call("dispatchEvent", evt)
			return nil
		}))
	}
	bindWheelToInput := func(id string) {
		bindWheelEl(doc.Call("getElementById", id))
	}
	for _, id := range []string{
		"camera-zoom", "rotation-controls-x", "rotation-controls-y",
		"rotation-controls-z", "speed-slider", "trail-slider", "line-width",
	} {
		bindWheelToInput(id)
	}

	// Wheel-on-select: hover-scroll over a <select> cycles its
	// selectedIndex (option groups skipped automatically — they
	// aren't part of .options). Fires the change event so existing
	// listeners (mode-select onModeChange, gradient-type handler)
	// react as if the user clicked.
	bindWheelToSelect := func(id string) {
		el := doc.Call("getElementById", id)
		if !el.Truthy() {
			return
		}
		el.Call("addEventListener", "wheel", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			e := args[0]
			e.Call("preventDefault")
			idx := el.Get("selectedIndex").Int()
			n := el.Get("options").Get("length").Int()
			if e.Get("deltaY").Float() > 0 {
				idx++
			} else {
				idx--
			}
			if idx < 0 {
				idx = 0
			}
			if idx >= n {
				idx = n - 1
			}
			el.Set("selectedIndex", idx)
			evtInit := js.Global().Get("Object").New()
			evtInit.Set("bubbles", true)
			evt := js.Global().Get("Event").New("change", evtInit)
			el.Call("dispatchEvent", evt)
			return nil
		}))
	}
	bindWheelToSelect("cat-select")
	bindWheelToSelect("model-select")
	bindWheelToSelect("gradient-source")
	bindWheelToSelect("gradient-colors")
	// Also wheel-bind every numeric input the param panel builds.
	// Re-invoke after each buildParamPanel so newly-added inputs get
	// wired too — wrap the existing helper into a package-level
	// rebinder we can call from buildParamPanel.
	rebindParamWheel = func() {
		params := doc.Call("getElementById", "params")
		if !params.Truthy() {
			return
		}
		inputs := params.Call("querySelectorAll", "input[type=range], input[type=number]")
		for i := 0; i < inputs.Length(); i++ {
			bindWheelEl(inputs.Index(i)) // by element — the number boxes have no id
		}
	}
	rebindParamWheel()

	// Window resize: keep canvas pixel dimensions in sync with the
	// viewport so the model doesn't get stretched when devtools opens
	// or closes (or on phone orientation change).
	js.Global().Call("addEventListener", "resize", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if sizeCanvasToViewport() {
			gl.Call("viewport", 0, 0, width, height)
			setupMatrices()
		}
		return nil
	}))

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

	<-done
}

func postDebugStats() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	avgMs := float32(0)
	fps := float32(0)
	if frameCount > 0 {
		avgMs = frameTotalMs / float32(frameCount)
		fps = 1000.0 / avgMs
	}

	payload := fmt.Sprintf(
		`{"mode":"%s","paused":%t,"fps":%.1f,"frame_avg_ms":%.2f,"frame_min_ms":%.2f,"frame_max_ms":%.2f,"frame_count":%d,"speed_steps":%d,"speed_scale":%.4f,"trail_steps":%d,"heap_alloc_mb":%.2f,"heap_sys_mb":%.2f,"heap_objects":%d,"gc_runs":%d,"goroutines":%d}`,
		selectedMode, paused, fps, avgMs, frameMinMs, frameMaxMs, frameCount,
		speedSteps, speedScale, steps,
		float64(ms.HeapAlloc)/1048576, float64(ms.HeapSys)/1048576,
		ms.HeapObjects, gcRunsCount(&ms), runtime.NumGoroutine(),
	)

	// Reset frame stats for next interval
	frameCount = 0
	frameTotalMs = 0
	frameMinMs = 999
	frameMaxMs = 0

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
	js.Global().Call("addEventListener", "unhandledrejection", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if len(a) > 0 {
			js.Global().Get("console").Call("warn", "[async rejection contained]", a[0].Get("reason"))
			a[0].Call("preventDefault")
		}
		return nil
	}))
}

func onResetAll(this js.Value, args []js.Value) interface{} {
	// Reset camera
	defaultCameraDist = initCameraDist
	rotationX1, rotationY1, rotationZ1 = 0, 0, 0

	// Static geometry may need re-upload (params reset to defaults).
	staticGeomDirty = true

	// Reset attractor position
	resetAttractorState()

	// Registry-owned controls (zoom, pan X/Y, rainbow period, …): each resets
	// itself — value, LED format, and any reset hook — so Reset All can never
	// silently miss one again. (Rotation sliders + movMatrix are re-randomized
	// below so the model never lands on the same view twice.)
	for _, c := range builtControls {
		c.resetToDefault()
	}

	// Reset all parameters to defaults
	for _, params := range attractorParams {
		for _, p := range params {
			*p.Value = p.Def
		}
	}
	buildParamPanel(selectedMode)

	// Reset auto-rotate, draw mode. (Speed / line width / trail — values,
	// LEDs, buffer realloc, persist drop — are registry-owned above.)
	paused = false
	if ps := doc.Call("getElementById", "pause-sw"); ps.Truthy() {
		ps.Set("checked", false)
	}
	usePoints = false
	attractorDrawMode = glTypes.LineStrip
	dragMatrix = mgl32.Ident4() // clear trackball drag orientation
	doc.Call("getElementById", "auto-rotate").Set("checked", true)
	doc.Call("getElementById", "use-points").Set("checked", false)
	doc.Call("getElementById", "show-info").Set("checked", false)
	doc.Call("getElementById", "info-overlay").Get("style").Set("display", "none")
	persistTrail = false
	doc.Call("getElementById", "persist-trail").Set("checked", false)
	gradientSource = 2
	gradientColors = 2
	gradientReverse = false
	doc.Call("getElementById", "gradient-source").Set("value", "2")
	doc.Call("getElementById", "gradient-colors").Set("value", "2")
	doc.Call("getElementById", "gradient-reverse").Set("checked", false)
	updateGradientUI()

	// Reset colors
	baseColor = [3]float32{1.0, 0.0, 0.0}
	midColor = [3]float32{0.0, 1.0, 0.0}
	topColor = [3]float32{0.0, 0.0, 1.0}
	bgColor = [3]float32{0, 0, 0}
	doc.Call("getElementById", "color-base").Set("value", "#ff0000")
	doc.Call("getElementById", "color-mid").Set("value", "#00ff00")
	doc.Call("getElementById", "color-top").Set("value", "#0000ff")
	doc.Call("getElementById", "color-bg").Set("value", "#000000")
	gl.Call("uniform3f", uBaseColorLoc, baseColor[0], baseColor[1], baseColor[2])
	gl.Call("uniform3f", uMidColorLoc, midColor[0], midColor[1], midColor[2])
	gl.Call("uniform3f", uTopColorLoc, topColor[0], topColor[1], topColor[2])
	// Alpha=0: don't paint over the host page's bg (SVG logo etc).
	gl.Call("clearColor", 0, 0, 0, 0)

	// Reset the remaining effect switches to their defaults — dispatch 'change'
	// so each effect's own handler applies it (single source of truth). Layout
	// prefs (dock edge, interface size) and mode toggles (Edit eqn, Fullscreen)
	// are intentionally left alone.
	swDefaults := []struct {
		id  string
		def bool
	}{
		{"model-front", false}, {"spect-fill", false}, {"audio-mod", false},
		{"test-tone", false}, {"fg-on", false}, {"spectro-skin", false},
		{"bg-spectro", false}, {"bg-xy", false}, {"tpl-on", false}, {"patch-on", false}, {"jam-sw", false}, {"show-meters", true},
		{"ring-sw", false}, {"twin-sw", false}, {"sect-sw", false},
	}
	for _, s := range swDefaults {
		if sw := doc.Call("getElementById", s.id); sw.Truthy() && sw.Get("checked").Bool() != s.def {
			sw.Set("checked", s.def)
			sw.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		}
	}
	// Display-style + knob-behavior selectors back to defaults.
	resetSel := func(id, val string) {
		if s := doc.Call("getElementById", id); s.Truthy() && s.Get("value").String() != val {
			s.Set("value", val)
			s.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		}
	}
	resetSel("knob-style", "std")
	resetSel("step-ratio", "1")
	resetSel("fine-ratio", "0.1")
	resetSel("sonify-map", "off")                          // Model Out ring back to off (silence)
	for _, id := range []string{"phosphor", "led-color"} { // populated at runtime → first option is the default
		if s := doc.Call("getElementById", id); s.Truthy() && s.Get("selectedIndex").Int() != 0 {
			s.Set("selectedIndex", 0)
			s.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		}
	}
	// Signal-generator oscillators back to their default note / level / wave, off.
	for _, g := range []struct {
		id        string
		freq, lvl int
	}{{"gen-x", 34, 80}, {"gen-y", 41, 80}, {"gen-z", 29, 80}} {
		setInput := func(sid, v string) {
			if e := doc.Call("getElementById", sid); e.Truthy() && e.Get("value").String() != v {
				e.Set("value", v)
				e.Call("dispatchEvent", js.Global().Get("Event").New("input"))
			}
		}
		setInput(g.id+"-freq", strconv.Itoa(g.freq))
		setInput(g.id+"-lvl", strconv.Itoa(g.lvl))
		resetSel(g.id+"-wave", "0")
		resetSel(g.id+"-out", "off")
	}

	// Randomized starting pose + low-rate rotation. Replaces the old
	// identity-matrix reset so each click of Reset All produces a
	// fresh viewing angle. randomizeOrientation zeroes the spin rates;
	// re-enable the gentle auto-spin afterward (so its Y-rate shows).
	randomizeOrientation()
	autoRotate = false
	setAutoRotate(true)

	// Reset view
	generateForMode(selectedMode)
	updateViewMatrix()
	updateModelMatrix()

	return nil
}
