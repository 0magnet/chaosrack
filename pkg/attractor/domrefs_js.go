//go:build js && wasm

package attractor

import (
	"syscall/js"
)

// ── DOM element refs ─────────────────────────────────────────────────────────

// cameraPanel is the camera controls on the panel: the zoom and rate inputs
// and the readouts beside them.
type cameraPanel struct {
	cameraControl     js.Value
	rotationControlsX js.Value
	rotationControlsY js.Value
	rotationControlsZ js.Value
	sliderZoom        js.Value
	sliderX           js.Value
	sliderY           js.Value
	sliderZ           js.Value
}

var camPanel cameraPanel

var (
	rtc         js.Value
	renderFrame js.Func
)
