//go:build js && wasm && !tinygo

package attractor

import "runtime"

// gcRunsCount surfaces runtime.MemStats.NumGC for the debug stats
// payload. TinyGo (as of 0.41) doesn't implement NumGC on its
// MemStats; gc_runs_tinygo.go returns 0 there. Standard Go reports
// the actual value via this build.
func gcRunsCount(ms *runtime.MemStats) uint32 { return ms.NumGC }
