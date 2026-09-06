//go:build js && wasm

package attractor

import (
	_ "embed"
	"syscall/js"
)

// ── Event handlers ───────────────────────────────────────────────────────────

func refreshGradient() {
	if shadersReady && len(attractorVertices) > 0 {
		updateGradientRange(attractorVertices)
	}
}

// standalonePanel is true when the controls are our own fixed overlay (not
// appended into a host page's <footer>); only then can we dock/move them.

func updateGradientUI() {
	// All palette knobs stay visible; the ones that don't apply to the current
	// color count are just dimmed (no populate/depopulate reflow when the count
	// changes). bg always applies.
	dim := func(id string, inactive bool) {
		if el := doc.Call("getElementById", id); el.Truthy() {
			el.Get("style").Set("display", "")
			el.Get("classList").Call("toggle", "pal-dim", inactive)
		}
	}
	dim("grp-cstart", gradientColors == 4)                         // no fixed colors in rainbow
	dim("grp-cmid", gradientColors != 3)                           // mid only in 3-color
	dim("grp-cend", !(gradientColors == 2 || gradientColors == 3)) // end in 2- / 3-color
	// The period is the palette WINDOW's width, and a window is something the
	// rainbow and the colormaps both have — it stopped being the rainbow's
	// private knob when the colormaps gained a shift to slide along it. The
	// shift itself stays a colormap control: the rainbow's offset is
	// uGradientPhase, which already exists and already animates.
	dim("grp-rainbow", gradientColors != 4 && gradientColors < paletteFirst)
	dim("grp-pshift", gradientColors < paletteFirst)
	if lbl := doc.Call("getElementById", "lbl-cstart"); lbl.Truthy() {
		if gradientColors == 1 {
			lbl.Set("textContent", "color")
		} else {
			lbl.Set("textContent", "start")
		}
	}
	updateViewModRows()
	quantizeModuleWidths()
}

// autoRotYDelta is the Y spin-rate the Auto-rotate switch contributes, so
// the auto-spin shows up on the Y rate knob rather than being a hidden term.

func onColorChange(this js.Value, args []js.Value) interface{} {
	baseHex := doc.Call("getElementById", "color-base").Get("value").String()
	midHex := doc.Call("getElementById", "color-mid").Get("value").String()
	topHex := doc.Call("getElementById", "color-top").Get("value").String()
	baseColor[0], baseColor[1], baseColor[2] = hexToRGB(baseHex)
	midColor[0], midColor[1], midColor[2] = hexToRGB(midHex)
	topColor[0], topColor[1], topColor[2] = hexToRGB(topHex)
	gl.Call("uniform3f", uBaseColorLoc, baseColor[0], baseColor[1], baseColor[2])
	gl.Call("uniform3f", uMidColorLoc, midColor[0], midColor[1], midColor[2])
	gl.Call("uniform3f", uTopColorLoc, topColor[0], topColor[1], topColor[2])
	return nil
}

// installErrorNet is the global backstop for the whole CLASS of uncaught async
// errors — promise rejections from browser APIs (fullscreen, permissions,
// media, AudioContext) that reject when the browser refuses, on mobile or in
// embedded/occluded contexts. Rather than chasing a missing .catch at every
// call site one at a time, this contains the class in one place: it logs the
// rejection and prevents the default so a stray one never degrades the session
// or shows the browser's uncaught-error UI. Call-site handling that also updates
// UI (e.g. re-syncing the Fullscreen switch) still lives where it matters; this
// is the safety net beneath it.
