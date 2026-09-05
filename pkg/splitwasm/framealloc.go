//go:build js && wasm

package splitwasm

import "syscall/js"

// What the frame allocates.
//
// The prototype's renderer was written to allocate nothing, and with nothing
// allocating there is no collection and the control plane's 18 MB is never
// marked — which makes the monolith look fine and the comparison meaningless.
// chaosrack's frame path is not allocation-free: measured on the TinyGo build,
// the live set churns ~1.6 MB between collections and a collection arrives
// every ~370 ms, i.e. ~4.3 MB/s, ~72 KB per frame. Almost all of it is
// runtime, not application: 88% of allocation traced to internal/task.start,
// TinyGo spawning a scheduler task per JS callback, plus the arg slices every
// syscall/js call boxes.
//
// So the renderer here allocates the measured amount per frame, in
// pointer-bearing objects, because that is the thing that triggers the
// collections whose cost the split is supposed to change.
const frameAllocKB = 72

type frameJunk struct {
	tag  string
	refs []*frameJunk
}

var junkSink []*frameJunk

func allocFrame(kb int) {
	if kb <= 0 {
		return
	}
	n := kb * 1024 / 64
	junkSink = make([]*frameJunk, 0, n)
	var prev *frameJunk
	for i := 0; i < n; i++ {
		o := &frameJunk{tag: "frame", refs: make([]*frameJunk, 4)}
		o.refs[0] = prev
		junkSink = append(junkSink, o)
		prev = o
	}
}

// frameAllocKBFromQuery lets the rate be dialed for a sweep; ?alloc=0 turns it
// off entirely, which is how the allocation-free case is measured.
func frameAllocKBFromQuery() int {
	s := js.Global().Get("location").Get("search").String()
	if containsStr(s, "alloc=0") {
		return 0
	}
	if containsStr(s, "alloc=144") {
		return 144
	}
	if containsStr(s, "alloc=36") {
		return 36
	}
	return frameAllocKB
}

// Why the sizes vary.
//
// TinyGo's alloc only runs a collection when popFreeRange cannot find a
// CONTIGUOUS run of blocks big enough (gc_blocks.go); afterwards it grows the
// heap only if the cycle failed to free a third of it. So how often the
// collector runs is set by fragmentation, not by an allocation threshold. A
// frame that allocates one uniform size keeps the free list tidy and almost
// never fails a request — which is why a first version of this probe, matching
// chaosrack's live set and its 4.3 MB/s exactly, still ran at 60 fps. A real
// UI allocates strings, slices, maps and closures of every size, and that is
// what makes the free list ragged enough to force a cycle every few hundred
// milliseconds.
var fragSink [][]byte
var fragSeed uint32 = 12345

func allocFragmented(kb int) {
	if kb <= 0 {
		return
	}
	remaining := kb * 1024
	fragSink = fragSink[:0]
	for remaining > 0 {
		// xorshift, so the sizes are spread but the run is repeatable
		fragSeed ^= fragSeed << 13
		fragSeed ^= fragSeed >> 17
		fragSeed ^= fragSeed << 5
		sz := int(fragSeed%8192) + 16
		if sz > remaining {
			sz = remaining
		}
		b := make([]byte, sz)
		b[0] = byte(sz) //nolint:gosec // G115: sz is 16..8207 and the byte is only touched so the allocation is not optimized away; the value is never read
		fragSink = append(fragSink, b)
		remaining -= sz
	}
}
