//go:build js && wasm

package splitwasm

import (
	"runtime"
	"syscall/js"
)

// ExportStats publishes this module's live-heap numbers under the given name,
// so a page running two modules can ask each of them separately what its own
// collector is working over. That separation is the whole claim being tested:
// after the split the renderer's cycles should be marking a heap of a few
// hundred kilobytes while the control plane still holds its megabytes.
func ExportStats(name string) {
	js.Global().Set(name, js.FuncOf(func(js.Value, []js.Value) interface{} {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return map[string]interface{}{
			"heapAlloc":   float64(m.HeapAlloc),
			"heapSys":     float64(m.HeapSys),
			"heapObjects": float64(m.HeapObjects),
			"sys":         float64(m.Sys),
		}
	}))
}
