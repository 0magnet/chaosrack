//go:build js && wasm

package attractor

// Recording to GIF, and recording a region rather than the whole canvas.
//
// The WebM path next door hands the canvas straight to MediaRecorder and never
// touches a pixel. GIF cannot work that way: there is no browser GIF encoder,
// so the frames have to come back into Go, and coming back into Go is also what
// makes a region crop possible. Both go through one scratch canvas — the region
// is drawn into it, the frames are read out of it — so cropping and encoding
// are the same mechanism rather than two.
//
// Frames are collected during the recording and encoded when it stops. Encoding
// as you go would be tidier and is not possible here: wasm is single-threaded,
// palettizing a frame takes longer than a frame lasts, and doing it inline
// would stutter the very animation being recorded.
//
// The encoder is pkg/gifenc, shared with `uitool gifs`, so a clip recorded in
// the app looks like the GIFs in the README rather than merely similar.

import (
	"bytes"
	"image"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/gifenc"
)

// gifFPS is how often frames are taken. Twelve is a compromise: the delay
// gifenc writes is 12/100s, so this plays back at about the speed it was
// recorded, and every extra frame is a full palettization at stop time.
const gifFPS = 12

// gifMaxFrames caps a clip at about a minute, so a forgotten recording cannot
// fill the tab. Reaching it stops the recording rather than dropping frames,
// because a clip that silently skips is worse than a short one.
const gifMaxFrames = 720

// gifMaxDim is the longest side a recorded GIF will have.
//
// Not a preference — a limit the format and the runtime both need. Frames are
// held as raw RGBA until the clip ends, and at the canvas's own size that is
// about 7MB each: four seconds at 12fps is 350MB of wasm heap, which is where
// the first attempt at this quietly died and produced no file at all. It would
// not have been usable even if it had survived, since palettizing two million
// pixels a frame single-threaded takes far longer than anyone will wait.
//
// 640 across is also simply what a GIF should be. The scratch canvas scales in
// the drawImage it was already doing, so this costs nothing.
const gifMaxDim = 640.0

// gifScale is how much to shrink a region to fit gifMaxDim, never enlarging —
// a small region should stay its own size rather than being blown up.
func gifScale(w, h float64) float64 {
	longest := w
	if h > longest {
		longest = h
	}
	if longest <= gifMaxDim || longest == 0 {
		return 1
	}
	return gifMaxDim / longest
}

var (
	gifRecording bool
	gifFrames    []*image.RGBA
	gifTimer     js.Value
	gifTickFn    js.Func

	// recScratch is where a frame is drawn before it is read: the crop happens
	// in the drawImage, so the pixels read back are already the region.
	recScratch    js.Value
	recScratchCtx js.Value
)

// recRegion is the area of the canvas to record, in canvas pixels. A zero width
// or height means the whole canvas.
var recRegion struct{ x, y, w, h float64 }

// recRegionRect resolves the region against the canvas, returning the source
// rectangle to copy. An unset or nonsensical region becomes the whole canvas,
// so a bad drag cannot produce an empty recording.
func recRegionRect(canvas js.Value) (x, y, w, h float64) {
	cw := canvas.Get("width").Float()
	ch := canvas.Get("height").Float()
	if recRegion.w <= 1 || recRegion.h <= 1 {
		return 0, 0, cw, ch
	}
	x, y, w, h = recRegion.x, recRegion.y, recRegion.w, recRegion.h
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x+w > cw {
		w = cw - x
	}
	if y+h > ch {
		h = ch - y
	}
	if w <= 1 || h <= 1 {
		return 0, 0, cw, ch
	}
	return x, y, w, h
}

// ensureScratch makes the scratch canvas the size of the region being recorded.
// willReadFrequently is set because every frame is read straight back out;
// without it the browser keeps the surface on the GPU and each getImageData
// stalls on a readback.
func ensureScratch(w, h float64) {
	if !recScratch.Truthy() {
		recScratch = doc.Call("createElement", "canvas")
	}
	if recScratch.Get("width").Float() != w || recScratch.Get("height").Float() != h {
		recScratch.Set("width", w)
		recScratch.Set("height", h)
		recScratchCtx = js.Undefined()
	}
	if !recScratchCtx.Truthy() {
		opts := js.Global().Get("Object").New()
		opts.Set("willReadFrequently", true)
		recScratchCtx = recScratch.Call("getContext", "2d", opts)
	}
}

// startGIFRecording begins collecting frames.
func startGIFRecording() {
	canvas := modelCanvas()
	if !canvas.Truthy() || gifRecording {
		return
	}
	sx, sy, sw, sh := recRegionRect(canvas)
	// Destination size: the region, shrunk to fit gifMaxDim. The crop and the
	// scale both happen in one drawImage below.
	k := gifScale(sw, sh)
	dw, dh := float64(int(sw*k)), float64(int(sh*k))
	ensureScratch(dw, dh)
	gifFrames = gifFrames[:0]
	gifRecording = true

	gifTickFn = trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if !gifRecording {
			return nil
		}
		// Cleared to opaque black FIRST, every frame.
		//
		// The model's canvas is transparent where nothing is drawn, so drawing
		// it onto the scratch composites rather than replaces: without this the
		// scratch keeps every earlier frame and the recording comes out looking
		// like persistence was left on, growing denser until the figure is a
		// solid mass. The frames themselves carry it, so no playback setting can
		// undo it.
		//
		// Black rather than transparent because GIF has no alpha to blend with
		// and the palette is built from opaque colors; this is also the
		// background the model is drawn against on the page.
		recScratchCtx.Set("fillStyle", "#000000")
		recScratchCtx.Call("fillRect", 0, 0, dw, dh)
		recScratchCtx.Call("drawImage", captureCanvas(canvas), sx, sy, sw, sh, 0, 0, dw, dh)
		img := recScratchCtx.Call("getImageData", 0, 0, dw, dh)
		data := img.Get("data")
		n := data.Get("length").Int()
		frame := image.NewRGBA(image.Rect(0, 0, int(dw), int(dh)))
		if len(frame.Pix) >= n {
			js.CopyBytesToGo(frame.Pix, js.Global().Get("Uint8Array").New(data.Get("buffer")))
		}
		gifFrames = append(gifFrames, frame)
		if len(gifFrames) >= gifMaxFrames {
			// Stop rather than drop: a clip that silently skips frames looks
			// like the app stuttering.
			stopGIFRecording()
			if sw := doc.Call("getElementById", "rec-sw"); sw.Truthy() {
				sw.Set("checked", false)
			}
		}
		return nil
	})
	gifTimer = js.Global().Call("setInterval", gifTickFn, 1000/gifFPS)
}

// stopGIFRecording encodes what was collected and hands it to the browser as a
// download.
func stopGIFRecording() {
	if !gifRecording {
		return
	}
	gifRecording = false
	if gifTimer.Truthy() {
		js.Global().Call("clearInterval", gifTimer)
		gifTimer = js.Undefined()
	}
	frames := gifFrames
	gifFrames = nil
	if len(frames) == 0 {
		return
	}

	var buf bytes.Buffer
	if err := gifenc.EncodeRGBA(&buf, frames, gifenc.DefaultDelay); err != nil {
		js.Global().Get("console").Call("warn", "gif encode: "+err.Error())
		return
	}
	downloadBytes(buf.Bytes(), "image/gif", "gif")
}

// downloadBytes hands a byte slice to the browser as a file. The copy into a
// JS array is unavoidable — a Go slice is not something Blob can see.
func downloadBytes(b []byte, mime, ext string) {
	arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(arr, b)
	parts := js.Global().Get("Array").New()
	parts.Call("push", arr)
	blob := js.Global().Get("Blob").New(parts, map[string]interface{}{"type": mime})
	saveBlob(blob, ext)
}

// saveBlob triggers a download of a Blob.
//
// The anchor goes into the document before it is clicked and the object URL is
// revoked on a timer rather than on the next line, and both of those matter.
// A detached anchor's click is not reliably treated as a user-initiated
// download, and revoking the URL immediately races the browser's read of the
// blob — the download is canceled before it starts, silently, with no error
// anywhere. That is what was happening here: the GIF encoded to 985KB and then
// no file appeared.
func saveBlob(blob js.Value, ext string) {
	url := js.Global().Get("URL").Call("createObjectURL", blob)
	anchor := doc.Call("createElement", "a")
	anchor.Set("href", url)
	anchor.Set("download", "chaosrack_"+fileStamp()+"."+ext)
	anchor.Get("style").Set("display", "none")
	body.Call("appendChild", anchor)
	anchor.Call("click")
	anchor.Call("remove")
	logTake(ext, blob.Get("size").Int())

	var revoke js.Func
	revoke = js.FuncOf(func(js.Value, []js.Value) interface{} {
		js.Global().Get("URL").Call("revokeObjectURL", url)
		revoke.Release()
		return nil
	})
	js.Global().Call("setTimeout", revoke, 60000)
}

// fileStamp is the local timestamp used in downloaded filenames, as
// 2026-08-18_21-03-44.
//
// Built in Go from the string, not by chaining JS calls onto it. The obvious
// spelling —
//
//	Date().toISOString().Call("slice", 0, 19).Call("replace", ...)
//
// panics: toISOString returns a JS *primitive* string, and syscall/js will not
// Call a method on a primitive. It takes the whole Go program down with it, so
// the failure is not a broken filename but a dead runtime and a download that
// silently never happens. That is what was wrong with the recorder before the
// GIF work: the blob was built, the object URL was created, and the panic came
// between that and the click.
func fileStamp() string {
	iso := js.Global().Get("Date").New().Call("toISOString").String()
	if len(iso) < 19 {
		return iso
	}
	s := []byte(iso[:19]) // 2026-08-18T21:03:44
	for i, c := range s {
		switch c {
		case 'T':
			s[i] = '_'
		case ':':
			s[i] = '-'
		}
	}
	return string(s)
}
