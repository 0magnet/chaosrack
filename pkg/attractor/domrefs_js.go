//go:build js && wasm

package attractor

import (
	"syscall/js"
)

// ── DOM element refs ─────────────────────────────────────────────────────────

var (
	rtc               js.Value
	cameraControl     js.Value
	rotationControlsX js.Value
	rotationControlsY js.Value
	rotationControlsZ js.Value
	sliderZoom        js.Value
	sliderX           js.Value
	sliderY           js.Value
	sliderZ           js.Value
	renderFrame       js.Func
)
