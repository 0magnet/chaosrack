//go:build js && wasm

package audiosrc

import (
	"encoding/binary"
	"syscall/js"
	"time"
)

// The audio sources' arrivals wait in JS until Go reads samples.
//
// Audio arrives in small pieces — a WebTransport datagram, a WebSocket
// message, a worklet's batch — some 180 to 190 of them a second, and a Go
// handler for each was a call into Go for each. Under TinyGo every such call
// is a goroutine, and up to 0.42 every goroutine leaves its 64 KB stack
// behind: twelve megabytes of garbage a second from the audio alone, and the
// collections it forced stopped the page for 120–150 ms every ~0.7 s, a skip
// in every animation. A handler written in JS costs nothing of the kind, so
// the pieces are queued there, and every reader of samples takes what has
// come (take) before it reads — no later than they were taken before.

// jsQueue is a bounded FIFO in JS: push is a JS function to hand an event
// source as its listener; each and eachBytes hand over what has come.
//
// Each piece taken as a JS value was a new JS object in Go, and every one of
// those gets a finalizer: some 190 a second from the audio alone, which is
// what started TinyGo's collections, each a 27 ms stop. eachBytes has JS copy
// whatever has come into one buffer kept for the purpose and answer how many
// bytes, a number: two calls a read, however many pieces, and no objects.
type jsQueue struct {
	v           js.Value
	take, drain js.Value // the queue's functions, called without naming them
	stage       js.Value // the Uint8Array drain packs into
	buf         []byte
	// empty is when the queue was last found empty. Readers ask a few dozen
	// times a frame and pieces come every few milliseconds, so a read within
	// a millisecond of finding nothing finds nothing again without asking JS.
	empty time.Time
}

// stageSize is the most drain packs at once; it drains again for the rest.
const stageSize = 256 << 10

// jsQueueJS makes a queue. field names the property of each pushed item to
// keep (an event's "data"), or "" for the item itself. At most limit items are
// kept, the oldest dropped first: a page that is not drawing reads nothing,
// and its audio is stale by the time it does.
//
// drain packs the oldest items into the stage as a little-endian uint32
// length and the bytes, as many whole ones as fit, and answers the bytes
// packed. An item is bytes however it came: an ArrayBuffer, a typed array,
// or base64 text (the old WebSocket frames). Anything else is dropped, as
// one that could never fit is.
const jsQueueJS = `return function(field, limit) {
	const q = [];
	let stage = null;
	const bytesOf = (x) => {
		if (x instanceof ArrayBuffer) return new Uint8Array(x);
		if (ArrayBuffer.isView(x)) return new Uint8Array(x.buffer, x.byteOffset, x.byteLength);
		if (typeof x === "string") {
			try {
				const s = atob(x);
				const b = new Uint8Array(s.length);
				for (let i = 0; i < s.length; i++) b[i] = s.charCodeAt(i);
				return b;
			} catch (e) {
				return null;
			}
		}
		return null;
	};
	return {
		push: (x) => {
			q.push(field ? x[field] : x);
			if (q.length > limit) q.shift();
		},
		take: () => q.shift(),
		stage: (n) => (stage = new Uint8Array(n)),
		drain: () => {
			let off = 0;
			while (q.length > 0) {
				const b = bytesOf(q[0]);
				if (b === null || 4 + b.length > stage.length) {
					q.shift();
					continue;
				}
				if (off + 4 + b.length > stage.length) break;
				const n = b.length;
				stage[off] = n & 255;
				stage[off + 1] = (n >>> 8) & 255;
				stage[off + 2] = (n >>> 16) & 255;
				stage[off + 3] = n >>> 24;
				stage.set(b, off + 4);
				off += 4 + n;
				q.shift();
			}
			return off;
		},
	};
};`

var jsQueueNew js.Value

func newJSQueue(field string, limit int) *jsQueue {
	if jsQueueNew.IsUndefined() {
		jsQueueNew = js.Global().Get("Function").New(jsQueueJS).Invoke()
	}
	v := jsQueueNew.Invoke(field, limit)
	return &jsQueue{v: v, take: v.Get("take"), drain: v.Get("drain")}
}

// push is the JS function that queues an item.
func (q *jsQueue) push() js.Value { return q.v.Get("push") }

// idle reports whether the queue was found empty less than a millisecond ago.
func (q *jsQueue) idle() bool {
	return !q.empty.IsZero() && time.Since(q.empty) < time.Millisecond
}

// each hands every queued item to f, oldest first. A nil queue (none made
// yet) holds nothing.
func (q *jsQueue) each(f func(js.Value)) {
	if q == nil || q.idle() {
		return
	}
	for {
		x := q.take.Invoke()
		if x.IsUndefined() {
			q.empty = time.Now()
			return
		}
		f(x)
	}
}

// eachBytes hands the bytes of every queued item to f, oldest first. The
// bytes are the queue's own, good until f returns.
func (q *jsQueue) eachBytes(f func([]byte)) {
	if q == nil || q.idle() {
		return
	}
	if q.stage.IsUndefined() {
		q.stage = q.v.Call("stage", stageSize)
		q.buf = make([]byte, stageSize)
	}
	for {
		n := q.drain.Invoke().Int()
		if n <= 0 {
			q.empty = time.Now()
			return
		}
		b := q.buf[:n]
		js.CopyBytesToGo(b, q.stage)
		for len(b) >= 4 {
			l := int(binary.LittleEndian.Uint32(b))
			if 4+l > len(b) {
				return
			}
			f(b[4 : 4+l])
			b = b[4+l:]
		}
	}
}

// datagramPumpJS reads a stream into a queue, calling onEnd once when the
// stream ends or fails — the one call into Go a stream makes.
const datagramPumpJS = `return function(reader, push, onEnd) {
	(async () => {
		try {
			for (;;) {
				const r = await reader.read();
				if (r.done) break;
				push(r.value);
			}
			onEnd();
		} catch (e) {
			onEnd(e);
		}
	})();
};`

var datagramPump js.Value

// pumpStream reads reader into q until it ends, then calls onEnd.
func pumpStream(reader js.Value, q *jsQueue, onEnd js.Func) {
	if datagramPump.IsUndefined() {
		datagramPump = js.Global().Get("Function").New(datagramPumpJS).Invoke()
	}
	datagramPump.Invoke(reader, q.push(), onEnd)
}
