//go:build js && wasm

package audiosrc

// Capture on the audio thread, which is the fix DefaultMicBufferSize's
// comment names and defers.
//
// A ScriptProcessorNode runs its callback on the MAIN thread, alongside the
// WebGL render loop, and cannot call back until it has collected a whole
// buffer. That makes its buffer size the display latency — 1024 frames is
// 21 ms — and it makes the capture vulnerable to the renderer: if the main
// thread is late, the node dumps a queue and the samples in it are gone. A
// gap in the ring is worse than a stale frame, which is why the buffer could
// not simply be made smaller.
//
// An AudioWorkletProcessor runs on the audio thread. It cannot be starved by
// rendering, its quantum is 128 frames whatever else is happening, and the
// only thing the main thread has to do is pick messages off a port. So the
// buffer stops being a latency floor and becomes a batching choice: this
// posts every workletFrames samples, which at 256 is 5.3 ms at 48 kHz —
// four times less than the ScriptProcessor path and no longer able to lose
// audio when a frame runs long.
//
// It is a JS module, which this package had avoided shipping. It is a dozen
// lines, it is built here as a Blob rather than served as a file so nothing
// has to be deployed alongside the wasm, and the ScriptProcessor path stays
// exactly as it was for any browser that refuses it.

import (
	"syscall/js"
	"unsafe"
)

// workletFrames is how many frames the processor gathers before posting.
//
// 128 (one quantum) would be 2.7 ms and three hundred messages a second;
// 256 is 5.3 ms and half that, which is under a rendered frame either way.
// The batching is about message count, not latency: the audio thread has
// the samples immediately in both cases.
const workletFrames = 256

// workletName is the registered processor name.
const workletName = "chaosrack-capture"

// workletSource is the module. Two channels always — the second is a copy
// of the first on a mono input, so the Go side does not have to care which
// it got — and the buffers are transferred rather than copied.
const workletSource = `
class ChaosrackCapture extends AudioWorkletProcessor {
  constructor(opts) {
    super();
    const o = (opts && opts.processorOptions) || {};
    this.n = o.frames || 256;
    this.l = new Float32Array(this.n);
    this.r = new Float32Array(this.n);
    this.i = 0;
  }
  process(inputs) {
    const inp = inputs[0];
    if (!inp || inp.length === 0) return true;
    const l = inp[0];
    const r = inp.length > 1 ? inp[1] : inp[0];
    for (let k = 0; k < l.length; k++) {
      this.l[this.i] = l[k];
      this.r[this.i] = r[k];
      if (++this.i === this.n) {
        const a = this.l, b = this.r;
        this.port.postMessage({l: a, r: b}, [a.buffer, b.buffer]);
        this.l = new Float32Array(this.n);
        this.r = new Float32Array(this.n);
        this.i = 0;
      }
    }
    return true;
  }
}
registerProcessor('` + workletName + `', ChaosrackCapture);
`

// startWorklet tries to build the capture graph on an AudioWorklet, calling
// done(true) when it is running and done(false) when the caller should fall
// back to a ScriptProcessorNode.
//
// Asynchronous because addModule is: the module has to be compiled on the
// audio thread before the node can be constructed, and there is no way to
// know it will succeed without waiting. The caller keeps its stream and its
// context either way.
func (m *micSource) startWorklet(done func(ok bool)) {
	aw := m.audioCtx.Get("audioWorklet")
	if !aw.Truthy() {
		done(false)
		return
	}
	blob := js.Global().Get("Blob").New(
		[]any{workletSource},
		map[string]any{"type": "application/javascript"},
	)
	url := js.Global().Get("URL").Call("createObjectURL", blob)

	var okFn, failFn js.Func
	cleanup := func() {
		js.Global().Get("URL").Call("revokeObjectURL", url)
		okFn.Release()
		failFn.Release()
	}
	okFn = js.FuncOf(func(js.Value, []js.Value) any {
		cleanup()
		if m.closed {
			done(false)
			return nil
		}
		done(m.attachWorkletNode())
		return nil
	})
	failFn = js.FuncOf(func(js.Value, []js.Value) any {
		// A browser without worklets, or a CSP that refuses a blob module.
		// Neither is an error worth surfacing: the fallback works.
		cleanup()
		done(false)
		return nil
	})
	aw.Call("addModule", url).Call("then", okFn).Call("catch", failFn)
}

// attachWorkletNode builds the node and wires its port to the rings.
func (m *micSource) attachWorkletNode() bool {
	ctor := js.Global().Get("AudioWorkletNode")
	if !ctor.Truthy() {
		return false
	}
	node := ctor.New(m.audioCtx, workletName, map[string]any{
		"numberOfInputs":   1,
		"numberOfOutputs":  0,
		"channelCount":     m.channels,
		"processorOptions": map[string]any{"frames": workletFrames},
	})
	if !node.Truthy() {
		return false
	}
	m.onMessage = js.FuncOf(m.handleWorkletMessage)
	node.Get("port").Set("onmessage", m.onMessage)
	m.src.Call("connect", node)
	// No connection to destination: a worklet with no outputs is pulled by
	// the graph on its own, unlike a ScriptProcessorNode, so the capture
	// does not have to be wired to the speakers to run.
	m.worklet = node
	return true
}

// handleWorkletMessage copies one posted batch into the rings.
func (m *micSource) handleWorkletMessage(_ js.Value, args []js.Value) any {
	if m.closed || len(args) == 0 {
		return nil
	}
	data := args[0].Get("data")
	if !data.Truthy() {
		return nil
	}
	m.pullTyped(data.Get("l"), m.ringL)
	if m.ringR != nil {
		m.pullTyped(data.Get("r"), m.ringR)
	}
	return nil
}

// pullTyped moves one Float32Array into a ring, the way pullChannel does it
// for the ScriptProcessor path: one bulk copy into the scratch, then a
// reinterpret rather than a second pass.
func (m *micSource) pullTyped(arr js.Value, r *ring) {
	if !arr.Truthy() || r == nil {
		return
	}
	n := arr.Get("length").Int()
	if n <= 0 {
		return
	}
	byteLen := n * 4
	if byteLen > len(m.byteScratch) {
		byteLen = len(m.byteScratch)
		n = byteLen / 4
	}
	u8 := js.Global().Get("Uint8Array").New(arr.Get("buffer"))
	js.CopyBytesToGo(m.byteScratch[:byteLen], u8)
	samples := unsafe.Slice((*float32)(unsafe.Pointer(&m.byteScratch[0])), n) //nolint:gosec // same reinterpret pullChannel documents
	r.write(samples)
}
