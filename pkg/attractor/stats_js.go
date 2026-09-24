//go:build js && wasm

package attractor

// ── Debug stats ─────────────────────────────────────────────────────────────

// frameTiming is the debug overlay's frame-time statistics.
type frameTiming struct {
	count     int
	totalMs   float32
	minMs     float32
	maxMs     float32
	lastStart float32
}

var fstats = frameTiming{
	minMs: 999,
}

var (
	debugEnabled bool
)
