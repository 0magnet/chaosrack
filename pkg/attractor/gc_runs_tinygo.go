//go:build js && wasm && tinygo

package attractor

import "runtime"

// gcRunsCount stub for TinyGo — runtime.MemStats.NumGC isn't
// implemented in TinyGo 0.41. Returns 0 so the debug payload stays
// well-formed JSON. Standard Go build provides the real value via
// gc_runs.go.
func gcRunsCount(_ *runtime.MemStats) uint32 { return 0 }
