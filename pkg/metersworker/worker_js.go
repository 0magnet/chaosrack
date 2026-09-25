//go:build js && wasm

// Package metersworker is the rack's analyzers, running somewhere else.
//
// The meters were about a sixth of the main thread, and none of that work
// produces a pixel: it is arithmetic on audio that happens to be scheduled
// inside the animation frame, where it competes with the one thing that does.
// Moving it off the main thread does not make it cheaper — the machine does
// the same sums — but it takes it out of the frame's way, which is the
// difference between a rack that hesitates and one that does not.
//
// The audio arrives here as it arrives anywhere: in blocks, when there are
// blocks. That is the other half of it. On the main thread the tap was
// DRAINED once per animation frame, so a dropped frame meant twice as much
// audio to chew through on the next one and a bigger lump to chew it in.
// Here the work is paced by the data, which is the clock it was always
// defined against — and the readouts latch off a clock counted in SAMPLES
// SEEN rather than in wall time, so a reading is always over the audio it
// claims to be over even if the machine stalls.
//
// What crosses: Float32Array blocks in, a few numbers out, a few times a
// second. The analyzers' state — the loudness integration, the two sliding
// windows — lives on this side and never crosses at all.
package metersworker

import (
	"encoding/json"
	"syscall/js"
	"unsafe"

	"github.com/0magnet/chaosrack/pkg/meters"
	"github.com/0magnet/chaosrack/pkg/metersproto"
)

// thdWindowSamples is the distortion analysis length, fixed here as it is on
// the panel: a distortion figure whose bandwidth moves under you is not
// comparable with itself a moment ago.
const thdWindowSamples = 16384

type state struct {
	cfg metersproto.Config

	lufs     *meters.LoudnessMeter
	lufsNext float64

	thdWin  meters.SlidingWindow
	thdBuf  []float32
	thdNext float64

	wfWin  meters.SlidingWindow
	wfBuf  []float32
	wfNext float64

	// clock is milliseconds of AUDIO seen, not of wall time. The analyzers'
	// periods are about how much signal has gone by.
	clock float64
}

var st state

// Run installs the message handler and blocks. Called from cmd/wasmmeters.
func Run() {
	js.Global().Set("onmessage", js.FuncOf(onMessage))
	send(metersproto.Envelope{T: metersproto.TypeReady})
	select {}
}

func send(e metersproto.Envelope) {
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	js.Global().Call("postMessage", string(b))
}

func onMessage(_ js.Value, args []js.Value) any {
	if len(args) == 0 {
		return nil
	}
	data := args[0].Get("data")
	switch data.Get("t").String() {
	case metersproto.TypeConfig:
		applyConfig(data.Get("v").String())
	case metersproto.TypeAudio:
		onAudio(data)
	}
	return nil
}

func applyConfig(s string) {
	var c metersproto.Config
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return
	}
	// A change of source rate retunes the K-weighting, and an integrated
	// loudness averaged across two different filters describes neither.
	if st.lufs == nil || c.SampleRate != st.cfg.SampleRate {
		if c.SampleRate > 0 {
			st.lufs = meters.NewLoudnessMeter(c.SampleRate)
		}
		st.thdWin.Reset()
		st.wfWin.Reset()
	}
	// A shorter window is a different measurement, not a truncated one.
	if c.WfWindow != st.cfg.WfWindow {
		st.wfWin.Reset()
	}
	st.cfg = c
}

// onAudio takes one block of stereo samples and advances whatever is due.
func onAudio(data js.Value) {
	l := toGo(data.Get("l"), &inL)
	r := toGo(data.Get("r"), &inR)
	if len(l) == 0 || st.cfg.SampleRate <= 0 {
		return
	}
	sr := st.cfg.SampleRate
	st.clock += 1000 * float64(len(l)) / float64(sr)

	if st.cfg.Want&metersproto.WantLufs != 0 && st.lufs != nil {
		st.lufs.Add(l, r)
		// The true peak on the raw buffers, oversampled: it needs the samples
		// either side of each point and the meter is a per-sample loop.
		if p := meters.TruePeak(l); p > 0 {
			st.lufs.SetTruePeak(p)
		}
		if p := meters.TruePeak(r); p > 0 {
			st.lufs.SetTruePeak(p)
		}
	}
	if st.cfg.Want&(metersproto.WantThd|metersproto.WantWf) != 0 {
		m := midOf(l, r)
		if st.cfg.Want&metersproto.WantThd != 0 {
			st.thdWin.Resize(thdWindowSamples)
			st.thdWin.Push(m)
		}
		if st.cfg.Want&metersproto.WantWf != 0 {
			if want := sr * st.cfg.WfWindow; want > 0 {
				st.wfWin.Resize(want)
				st.wfWin.Push(m)
			}
		}
	}
	if out := latch(sr); out != nil {
		send(metersproto.Envelope{T: metersproto.TypeResult, V: out})
	}
}

// latch produces whatever has come due, or nil if nothing has.
func latch(sr int) *metersproto.Result {
	var out metersproto.Result
	found := false

	if st.cfg.Want&metersproto.WantLufs != 0 && st.lufs != nil && st.clock >= st.lufsNext {
		st.lufsNext = st.clock + st.cfg.LufsPeriod
		res := st.lufs.Result()
		out.Lufs, found = &res, true
	}
	if st.cfg.Want&metersproto.WantThd != 0 && st.thdWin.Full() && st.clock >= st.thdNext {
		st.thdNext = st.clock + st.cfg.ThdPeriod
		if len(st.thdBuf) != thdWindowSamples {
			st.thdBuf = make([]float32, thdWindowSamples)
		}
		st.thdWin.Linear(st.thdBuf)
		res := meters.AnalyzeDistortion(st.thdBuf, sr, st.cfg.ThdHarm)
		out.Thd, found = &res, true
	}
	if st.cfg.Want&metersproto.WantWf != 0 && st.clock >= st.wfNext {
		if want := sr * st.cfg.WfWindow; want > 0 {
			st.wfNext = st.clock + st.cfg.WfPeriod
			if len(st.wfBuf) != want {
				st.wfBuf = make([]float32, want)
			}
			n := st.wfWin.Linear(st.wfBuf)
			res := meters.AnalyzeWowFlutter(st.wfBuf[:n], sr, st.cfg.WfNominal)
			out.Wf, found = &res, true
		}
	}
	if !found {
		return nil
	}
	return &out
}

var midBuf []float32

// midOf is the mono sum the two window analyzers measure.
func midOf(l, r []float32) []float32 {
	if len(r) != len(l) {
		return l
	}
	if len(midBuf) < len(l) {
		midBuf = make([]float32, len(l))
	}
	m := midBuf[:len(l)]
	for i := range l {
		m[i] = (l[i] + r[i]) * 0.5
	}
	return m
}

var inL, inR []float32

// toGo copies a Float32Array into a reused Go slice — one memcpy a block,
// which is the whole budget this side has for getting the audio in.
func toGo(v js.Value, buf *[]float32) []float32 {
	if !v.Truthy() {
		return nil
	}
	n := v.Get("length").Int()
	if n <= 0 {
		return nil
	}
	if cap(*buf) < n {
		*buf = make([]float32, n)
	}
	out := (*buf)[:n]
	u8 := js.Global().Get("Uint8Array").New(v.Get("buffer"), v.Get("byteOffset"), n*4)
	js.CopyBytesToGo(f32Bytes(out), u8)
	return out
}

// f32Bytes reinterprets a float32 slice as the bytes behind it, so a block
// crosses as one memcpy rather than a value at a time.
func f32Bytes(s []float32) []byte {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*4) //nolint:gosec // reinterpreting a typed slice as its own storage, for js.CopyBytesToGo; the length is exactly four bytes an element
}
