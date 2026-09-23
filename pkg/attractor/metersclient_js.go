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

var (
	metersW       js.Value // the Worker
	metersWReady  bool     // its Go instance has reported in
	metersWTried  bool
	metersWCfg    string // the last config sent, to avoid resending it
	metersWL      js.Value
	metersWR      js.Value
	metersWCap    int
	metersWantNow uint8
)

// startMetersWorker builds the Worker. Called once from Run; failure is not
// an error, it just leaves the in-thread path in charge.
func startMetersWorker() {
	if metersWTried {
		return
	}
	metersWTried = true
	ctor := js.Global().Get("Worker")
	if !ctor.Truthy() {
		return
	}
	defer func() {
		// A Content-Security-Policy that forbids workers throws here, and the
		// recover is the point: the panel keeps its own analyzers.
		if recover() != nil {
			metersW, metersWReady = js.Value{}, false
		}
	}()
	metersW = ctor.New(metersWorkerURL)
	metersW.Set("onmessage", dom.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		if len(args) == 0 {
			return nil
		}
		onMetersMessage(args[0].Get("data"))
		return nil
	}))
	metersW.Set("onerror", dom.FuncOf(func(_ js.Value, _ []js.Value) interface{} {
		// It failed to load or it panicked. Either way the panel takes the
		// analyzers back rather than showing dashes forever.
		metersWReady = false
		return nil
	}))
}

// onMetersMessage takes a result and writes it to the readouts.
func onMetersMessage(data js.Value) {
	var msg metersproto.Envelope
	if err := json.Unmarshal([]byte(data.String()), &msg); err != nil {
		return
	}
	switch msg.T {
	case metersproto.TypeReady:
		metersWReady = true
		metersWCfg = "" // the far side knows nothing yet; tell it on the next block
	case metersproto.TypeResult:
		if msg.V == nil {
			return
		}
		if r := msg.V.Lufs; r != nil {
			lufsRes = *r
			showLoudness()
		}
		if r := msg.V.Thd; r != nil {
			thdRes = *r
			showDistortion()
		}
		if r := msg.V.Wf; r != nil {
			wfRes = *r
			showWowFlutter()
		}
	}
}

// metersWorkerWant is which analyzers are on screen, which is the only thing
// the far side needs to know about the panel.
func metersWorkerWant() uint8 {
	var w uint8
	if moduleOnScreen("lufs-module") {
		w |= metersproto.WantLufs
	}
	if moduleOnScreen("thd-module") {
		w |= metersproto.WantThd
	}
	if moduleOnScreen("wf-module") {
		w |= metersproto.WantWf
	}
	return w
}

// sendMetersConfig tells the worker what the panel's switches say, and only
// when one of them has moved.
func sendMetersConfig(sr int) {
	b, err := json.Marshal(metersproto.Config{
		SampleRate: sr,
		ThdPeriod:  thdPeriodMs,
		ThdHarm:    int(thdHarmF),
		WfPeriod:   wfPeriodMs,
		WfWindow:   wfWindowSec,
		WfNominal:  float64(wfNominal),
		LufsPeriod: lufsPeriodMs,
		Want:       metersWantNow,
	})
	if err != nil {
		return
	}
	s := string(b)
	if s == metersWCfg {
		return
	}
	metersWCfg = s
	m := js.Global().Get("Object").New()
	m.Set("t", metersproto.TypeConfig)
	m.Set("v", s)
	metersW.Call("postMessage", m)
}

// metersWorkerArrays are the buffers the blocks cross in, grown rather than
// reallocated: a fresh pair per block is two finalized js.Values a block.
func metersWorkerArrays(n int) bool {
	if metersWCap >= n && metersWL.Truthy() {
		return true
	}
	f32 := js.Global().Get("Float32Array")
	if !f32.Truthy() {
		return false
	}
	metersWL = f32.New(n)
	metersWR = f32.New(n)
	metersWCap = n
	return true
}

// metersWorkerTick drains the tap and hands the audio over. Returns false if
// the worker is not carrying the analyzers, so the caller runs its own.
func metersWorkerTick() bool {
	if !metersWReady || !metersW.Truthy() {
		return false
	}
	want := metersWorkerWant()
	if want != metersWantNow {
		metersWantNow = want
		metersWCfg = "" // the far side is told on the next block
	}
	sr := takensSourceRate()
	if sr <= 0 {
		return true // nothing to send, but the analyzers are still not ours
	}
	sendMetersConfig(sr)
	if want == 0 {
		// Nothing on screen. The tap still has to be drained or its ring
		// wraps and the next reading starts mid-sentence.
		drainMetersTap()
		return true
	}
	var sl, sr2 [4096]float32
	for {
		n := tapReadStereo(&metersCursor, sl[:], sr2[:])
		if n <= 0 {
			break
		}
		if metersWorkerArrays(n) {
			js.CopyBytesToJS(js.Global().Get("Uint8Array").New(metersWL.Get("buffer"), 0, n*4), sliceToByteSlice(sl[:n]))
			js.CopyBytesToJS(js.Global().Get("Uint8Array").New(metersWR.Get("buffer"), 0, n*4), sliceToByteSlice(sr2[:n]))
			m := js.Global().Get("Object").New()
			m.Set("t", metersproto.TypeAudio)
			m.Set("l", metersWL.Call("subarray", 0, n))
			m.Set("r", metersWR.Call("subarray", 0, n))
			metersW.Call("postMessage", m)
		}
		if n < len(sl) {
			break
		}
	}
	return true
}

// drainMetersTap throws away what has arrived, so the cursor keeps up.
func drainMetersTap() {
	var sl, sr2 [4096]float32
	for {
		n := tapReadStereo(&metersCursor, sl[:], sr2[:])
		if n <= 0 || n < len(sl) {
			break
		}
	}
}

// metersCursor is the worker's own read position in the tap, separate from
// every in-thread analyzer's: the tap hands each sample over once per reader.
var metersCursor = tapUnjoined
