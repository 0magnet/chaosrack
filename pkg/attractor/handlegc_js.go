//go:build js && wasm

package attractor

import (
	"runtime"
	"runtime/metrics"
	"time"
)

// The Go build's collector runs when its heap has grown by half again, and a
// page that allocates a megabyte a minute goes minutes between collections.
// Every JS value Go holds is released only by a collection (its finalizer),
// and with it the JS object and the Blink object behind it: between
// collections Go's handles piled up by ten thousand a minute, Blink's heap sat
// at 160 MB and the tab at 370 MB (chaosrack mem --watch). A collection of this
// heap costs about 2 ms, so one is run whenever there has been none for
// handleGCEvery. TinyGo's runtime already collects on finalizer pressure.
const handleGCEvery = 10 * time.Second

// startHandleGC keeps the Go build collecting at least every handleGCEvery.
func startHandleGC() {
	if runtime.Compiler != "gc" {
		return
	}
	go func() {
		sample := []metrics.Sample{{Name: "/gc/cycles/total:gc-cycles"}}
		cycles := func() uint64 {
			metrics.Read(sample)
			if sample[0].Value.Kind() != metrics.KindUint64 {
				return 0
			}
			return sample[0].Value.Uint64()
		}
		last := cycles()
		t := time.NewTicker(handleGCEvery)
		defer t.Stop()
		for range t.C {
			if n := cycles(); n != last {
				last = n
				continue
			}
			runtime.GC()
			last = cycles()
		}
	}()
}
