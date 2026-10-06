//go:build js && wasm

package audiosrc

import "syscall/js"

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
// source as its listener, and take returns the oldest item or undefined.
type jsQueue struct{ v js.Value }

// jsQueueJS makes a queue. field names the property of each pushed item to
// keep (an event's "data"), or "" for the item itself. At most limit items are
// kept, the oldest dropped first: a page that is not drawing reads nothing,
// and its audio is stale by the time it does.
const jsQueueJS = `return function(field, limit) {
	const q = [];
	return {
		push: (x) => {
			q.push(field ? x[field] : x);
			if (q.length > limit) q.shift();
		},
		take: () => q.shift(),
	};
};`

var jsQueueNew js.Value

func newJSQueue(field string, limit int) jsQueue {
	if jsQueueNew.IsUndefined() {
		jsQueueNew = js.Global().Get("Function").New(jsQueueJS).Invoke()
	}
	return jsQueue{jsQueueNew.Invoke(field, limit)}
}

// push is the JS function that queues an item.
func (q jsQueue) push() js.Value { return q.v.Get("push") }

// each hands every queued item to f, oldest first. A zero queue (none made
// yet) holds nothing.
func (q jsQueue) each(f func(js.Value)) {
	if q.v.IsUndefined() {
		return
	}
	for {
		x := q.v.Call("take")
		if x.IsUndefined() {
			return
		}
		f(x)
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
func pumpStream(reader js.Value, q jsQueue, onEnd js.Func) {
	if datagramPump.IsUndefined() {
		datagramPump = js.Global().Get("Function").New(datagramPumpJS).Invoke()
	}
	datagramPump.Invoke(reader, q.push(), onEnd)
}
