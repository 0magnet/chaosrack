//go:build js && wasm

package attractor

// In-app recorder (Setup > Record): canvas.captureStream → MediaRecorder →
// a .webm download when the switch flips off. Pure browser plumbing, no
// server. VP9 when the browser offers it, otherwise whatever default WebM
// flavor MediaRecorder picks.

import "syscall/js"

var (
	recOn     bool
	recorder  js.Value
	recChunks js.Value
	recStopFn js.Func
	recDataFn js.Func
)

func startRecording() {
	canvas := doc.Call("querySelector", "canvas")
	if !canvas.Truthy() {
		return
	}
	mr := js.Global().Get("MediaRecorder")
	if !mr.Truthy() {
		return
	}
	stream := canvas.Call("captureStream", 60)
	opts := js.Global().Get("Object").New()
	if mr.Call("isTypeSupported", "video/webm;codecs=vp9").Truthy() {
		opts.Set("mimeType", "video/webm;codecs=vp9")
	}
	recorder = mr.New(stream, opts)
	recChunks = js.Global().Get("Array").New()
	recDataFn = trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if d := a[0].Get("data"); d.Get("size").Int() > 0 {
			recChunks.Call("push", d)
		}
		return nil
	})
	recStopFn = trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		blob := js.Global().Get("Blob").New(recChunks, map[string]interface{}{"type": "video/webm"})
		url := js.Global().Get("URL").Call("createObjectURL", blob)
		anchor := doc.Call("createElement", "a")
		anchor.Set("href", url)
		stamp := js.Global().Get("Date").New().Call("toISOString").Call("slice", 0, 19).
			Call("replace", "T", "_").Call("replaceAll", ":", "-").String()
		anchor.Set("download", "chaosrack_"+stamp+".webm")
		anchor.Call("click")
		js.Global().Get("URL").Call("revokeObjectURL", url)
		recChunks = js.Undefined()
		return nil
	})
	recorder.Set("ondataavailable", recDataFn)
	recorder.Set("onstop", recStopFn)
	recorder.Call("start", 1000) // chunk every second
}

func stopRecording() {
	if recorder.Truthy() && recorder.Get("state").String() != "inactive" {
		recorder.Call("stop") // onstop finalizes + downloads
	}
	recorder = js.Undefined()
}

func wireRecordSwitch() {
	sw := doc.Call("getElementById", "rec-sw")
	if !sw.Truthy() {
		return
	}
	sw.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		recOn = sw.Get("checked").Bool()
		if recOn {
			startRecording()
		} else {
			stopRecording()
		}
		return nil
	}))
}
