//go:build js && wasm

package attractor

import (
	"syscall/js"
)

// ── DOM element refs ─────────────────────────────────────────────────────────

var (
	rtc                 js.Value
	cameraControl       js.Value
	rotationControlsX   js.Value
	rotationControlsY   js.Value
	rotationControlsZ   js.Value
	sliderZoom          js.Value
	sliderX             js.Value
	sliderY             js.Value
	sliderZ             js.Value
	uBaseColorLoc       js.Value
	uTopColorLoc        js.Value
	uMidColorLoc        js.Value
	uMinZLoc            js.Value
	uMaxZLoc            js.Value
	uMinXLoc            js.Value
	uMaxXLoc            js.Value
	uMinYLoc            js.Value
	uMaxYLoc            js.Value
	uGradientSourceLoc  js.Value
	uAudioLUTLoc        js.Value
	uPaletteLoc         js.Value
	uDashDutyLoc        js.Value
	uDashCountLoc       js.Value
	uGradientColorsLoc  js.Value
	uGradientFreqLoc    js.Value
	uGradientPhaseLoc   js.Value
	uGradientReverseLoc js.Value
	uPointSizeLoc       js.Value
	uMmatrixLoc         js.Value
	uVmatrixLoc         js.Value
	uTrailHeadLoc       js.Value
	uSplitZLoc          js.Value
	uSplitSideLoc       js.Value
	positionLoc         js.Value
	aTrailTLoc          js.Value
	aDwellLoc           js.Value
	shadersReady        bool
	renderFrame         js.Func
)
