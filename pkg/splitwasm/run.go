//go:build js && wasm

package splitwasm

import "syscall/js"

// heapFromQuery lets one build stand in for either side of the comparison, so
// the monolith and the split carry exactly the same control-plane heap and the
// only difference between them is which runtime owns it.
func heapFromQuery() (mb, objects int) {
	s := js.Global().Get("location").Get("search").String()
	if containsStr(s, "noheap") {
		return 0, 0
	}
	return 18, 25000
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// RunMonolith is the shape chaosrack has now: one module, one runtime, one
// heap, holding both the frame budget and everything that is not on it.
func RunMonolith() {
	ExportStats("__statsMonolith")
	mb, objs := heapFromQuery()
	StartControl(mb, objs)
	StartRenderer()
	select {}
}

// RunRenderer is the frame-budget half on its own.
func RunRenderer() {
	ExportStats("__statsRenderer")
	StartRenderer()
	select {}
}

// RunControl is everything that runs on events rather than every frame.
func RunControl() {
	ExportStats("__statsControl")
	mb, objs := heapFromQuery()
	StartControl(mb, objs)
	select {}
}
