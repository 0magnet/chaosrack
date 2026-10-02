//go:build js && wasm

package audiosrc

import "syscall/js"

// NewTap captures node, a stereo node in the audio context ctx, the way
// NewMic captures a microphone: into rings, through a worklet where the
// browser takes one. What a rack built in Web Audio needs to hear its own
// instruments the way it hears the capture — the Mixer's return.
//
// The tap listens through a node of its own, so closing it lets go of node
// without disconnecting anything else from it.
func NewTap(ctx, node js.Value) Source {
	opts := MicOptions{Stereo: true, BufferSize: DefaultMicBufferSize, RingSize: DefaultRingSize, Context: ctx}
	m := &micSource{opts: opts, ringL: newRing(opts.RingSize), ringR: newRing(opts.RingSize)}
	m.audioCtx = ctx
	m.sampleRate = ctx.Get("sampleRate").Int()
	m.channels = 2
	m.src = ctx.Call("createGain")
	node.Call("connect", m.src)
	m.byteScratch = make([]byte, opts.BufferSize*4)
	m.startWorklet(func(ok bool) {
		if m.closed {
			return
		}
		if !ok {
			m.startScriptProcessor()
		}
		m.ready = true
	})
	return m
}
