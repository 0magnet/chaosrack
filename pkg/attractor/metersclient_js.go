//go:build js && wasm

package attractor

import (
	"encoding/json"
	"github.com/0magnet/chaosrack/pkg/dom"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/metersproto"
)

// The panel's half of the analyzers-in-a-worker arrangement.
//
// When the worker is up, the three analyzer ticks stop doing arithmetic on
// this thread: they still drain the tap — the tap must be drained by somebody
// or its ring wraps — but the samples go straight across and the readouts are
// written from what comes back. When it is not up, every one of them runs
// exactly as before. That is deliberate and not merely defensive: a Worker is
// unavailable under file:// on some browsers, and a page with no analyzers is
// a worse failure than a page with slow ones.
//
// The audio crosses as a pair of Float32Arrays per block. The analyzers'
// state — the loudness integration, the two sliding windows — never crosses
// at all, because it lives on the far side for its whole life.

const metersWorkerURL = "/assets/metersworker/worker.js"

// metersClient is the panel's half of the analyzers-in-a-worker arrangement.
type metersClient struct {
	w      js.Value // the Worker
	wReady bool     // its Go instance has reported in
	wTried bool
	wCfg   metersproto.Config // the last config sent, to avoid resending it
	wSent  bool               // whether wCfg has been sent since the worker last started
	wl     js.Value
	wr     js.Value
	wlb    js.Value // byte views of wl and wr, which the blocks are copied in by
	wrb    js.Value
	wCap   int
	// The tap's blocks are read into these: a pair of 16 KB arrays on the
	// stack was zeroed every call, every frame.
	tapL, tapR [4096]float32
	wantNow    uint8

	// cursor is the worker's own read position in the tap, separate from
	// every in-thread analyzer's: the tap hands each sample over once per reader.
	cursor int
}

var mc = metersClient{
	cursor: tapUnjoined,
}

// startMetersWorker builds the Worker. Called once from Run; failure is not
// an error, it just leaves the in-thread path in charge.
func (me *metersClient) startMetersWorker() {
	if me.wTried {
		return
	}
	me.wTried = true
	ctor := js.Global().Get("Worker")
	if !ctor.Truthy() {
		return
	}
	defer func() {
		// A Content-Security-Policy that forbids workers throws here, and the
		// recover is the point: the panel keeps its own analyzers.
		if recover() != nil {
			me.w, me.wReady = js.Value{}, false
		}
	}()
	me.w = ctor.New(metersWorkerURL)
	me.w.Set("onmessage", dom.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		me.onMetersMessage(args[0].Get("data"))
		return nil
	}))
	me.w.Set("onerror", dom.FuncOf(func(_ js.Value, _ []js.Value) any {
		// It failed to load or it panicked. Either way the panel takes the
		// analyzers back rather than showing dashes forever.
		me.wReady = false
		return nil
	}))
}

// onMetersMessage takes a result and writes it to the readouts.
func (me *metersClient) onMetersMessage(data js.Value) {
	var msg metersproto.Envelope
	if err := json.Unmarshal([]byte(data.String()), &msg); err != nil {
		return
	}
	switch msg.T {
	case metersproto.TypeReady:
		me.wReady = true
		me.wSent = false // the far side knows nothing yet; tell it on the next block
	case metersproto.TypeResult:
		if msg.V == nil {
			return
		}
		if r := msg.V.Lufs; r != nil {
			lufs.res = *r
			lufs.showLoudness()
		}
		if r := msg.V.Thd; r != nil {
			thd.res = *r
			thd.showDistortion()
		}
		if r := msg.V.Wf; r != nil {
			wow.res = *r
			wow.showWowFlutter()
		}
	}
}

// metersWorkerWant is which analyzers are on screen, which is the only thing
// the far side needs to know about the panel.
func metersWorkerWant() uint8 {
	var w uint8
	if onScreen.moduleOnScreen("lufs-module") {
		w |= metersproto.WantLufs
	}
	if onScreen.moduleOnScreen("thd-module") {
		w |= metersproto.WantThd
	}
	if onScreen.moduleOnScreen("wf-module") {
		w |= metersproto.WantWf
	}
	return w
}

// sendMetersConfig tells the worker what the panel's switches say, and only
// when one of them has moved. The settings are compared as they are and
// marshaled only to be sent: marshaling them to compare the text ran
// encoding/json every tick for an answer that was nearly always "the same".
func (me *metersClient) sendMetersConfig(sr int) {
	cfg := metersproto.Config{
		SampleRate: sr,
		ThdPeriod:  thd.periodMs,
		ThdHarm:    int(thd.harmF),
		WfPeriod:   wow.periodMs,
		WfWindow:   wow.windowSec,
		WfNominal:  float64(wow.nominal),
		LufsPeriod: lufs.periodMs,
		Want:       me.wantNow,
	}
	if me.wSent && cfg == me.wCfg {
		return
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	s := string(b)
	me.wCfg, me.wSent = cfg, true
	m := js.Global().Get("Object").New()
	m.Set("t", metersproto.TypeConfig)
	m.Set("v", s)
	me.w.Call("postMessage", m)
}

// workerArrays are the buffers the blocks cross in, grown rather than
// reallocated: a fresh pair per block is two finalized js.Values a block.
func (me *metersClient) workerArrays(n int) bool {
	if me.wCap >= n && me.wl.Truthy() {
		return true
	}
	f32 := js.Global().Get("Float32Array")
	if !f32.Truthy() {
		return false
	}
	me.wl = f32.New(n)
	me.wr = f32.New(n)
	u8 := js.Global().Get("Uint8Array")
	me.wlb = u8.New(me.wl.Get("buffer"))
	me.wrb = u8.New(me.wr.Get("buffer"))
	me.wCap = n
	return true
}

// workerTick drains the tap and hands the audio over. Returns false if
// the worker is not carrying the analyzers, so the caller runs its own.
func (me *metersClient) workerTick() bool {
	if !me.wReady || !me.w.Truthy() {
		return false
	}
	want := metersWorkerWant()
	if want != me.wantNow {
		me.wantNow = want
		me.wSent = false // the far side is told on the next block
	}
	sr := takensSourceRate()
	if sr <= 0 {
		return true // nothing to send, but the analyzers are still not ours
	}
	me.sendMetersConfig(sr)
	if want == 0 {
		// Nothing on screen. The tap still has to be drained or its ring
		// wraps and the next reading starts mid-sentence.
		me.drainMetersTap()
		return true
	}
	sl, sr2 := &me.tapL, &me.tapR
	for {
		n := tap.readStereo(&me.cursor, sl[:], sr2[:])
		if n <= 0 {
			break
		}
		if me.workerArrays(n) {
			js.CopyBytesToJS(me.wlb, sliceToByteSlice(sl[:n]))
			js.CopyBytesToJS(me.wrb, sliceToByteSlice(sr2[:n]))
			m := js.Global().Get("Object").New()
			m.Set("t", metersproto.TypeAudio)
			m.Set("l", me.wl.Call("subarray", 0, n))
			m.Set("r", me.wr.Call("subarray", 0, n))
			me.w.Call("postMessage", m)
		}
		if n < len(sl) {
			break
		}
	}
	return true
}

// drainMetersTap throws away what has arrived, so the cursor keeps up.
func (me *metersClient) drainMetersTap() {
	sl, sr2 := &me.tapL, &me.tapR
	for {
		n := tap.readStereo(&me.cursor, sl[:], sr2[:])
		if n <= 0 || n < len(sl) {
			break
		}
	}
}
