//go:build js && wasm

package attractor

// ── Debug stats ─────────────────────────────────────────────────────────────

var (
	debugEnabled   bool
	frameCount     int
	frameTotalMs   float32
	frameMinMs     float32 = 999
	frameMaxMs     float32
	lastFrameStart float32
)
