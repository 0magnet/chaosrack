//go:build js && wasm

package attractor

// In-app recorder (Setup > Record): canvas.captureStream → MediaRecorder →
// a .webm download when the switch flips off. Pure browser plumbing, no
// server. VP9 when the browser offers it, otherwise whatever default WebM
// flavor MediaRecorder picks.

import (
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// canvasRecorder is the in-app recorder: the MediaRecorder, its chunks, and
// the audio fed into it.
type canvasRecorder struct {
	on       bool
	recorder js.Value
	chunks   js.Value
	stopFn   js.Func
	dataFn   js.Func

	// The feed: what the video stream is captured from.
	//
	// With no region that is the canvas itself, captured directly — no copy, no
	// per-frame cost, and the recording is exactly what is on screen. With a region
	// it is a canvas of the region's size that the region is drawn into every
	// frame, and the stream comes from that.
	//
	// This is what the AREA switch already promises. The monitor showed the region,
	// the still wrote the region, the GIF recorded the region — and the video
	// recorded the whole canvas, because it handed captureStream the canvas and
	// never looked at the area at all.
	//
	// At the region's own resolution, not scaled down. Scaling is the wrong lever
	// for this picture: the content is one-pixel lines, and resampling a one-pixel
	// line is how you lose it. Fewer pixels to encode is a real benefit of
	// recording a region, but it comes from recording less of the picture, not from
	// recording the same picture worse.
	feed    js.Value
	feedCtx js.Value
	feedFn  js.Func
	feedOn  bool
}

var rec canvasRecorder

func (c *canvasRecorder) startRecording() {
	canvas := modelCanvas()
	if !canvas.Truthy() {
		return
	}
	mr := js.Global().Get("MediaRecorder")
	if !mr.Truthy() {
		return
	}
	const fps = 60
	src := c.streamSource(canvas)
	stream := src.Call("captureStream", fps)
	opts := js.Global().Get("Object").New()
	if mr.Call("isTypeSupported", "video/webm;codecs=vp9").Truthy() {
		opts.Set("mimeType", "video/webm;codecs=vp9")
	}
	opts.Set("videoBitsPerSecond", recBitrate(src, fps))
	c.recorder = mr.New(stream, opts)
	c.chunks = js.Global().Get("Array").New()
	c.dataFn = dom.FuncOf(func(this js.Value, a []js.Value) any {
		if d := a[0].Get("data"); d.Get("size").Int() > 0 {
			c.chunks.Call("push", d)
		}
		return nil
	})
	c.stopFn = dom.FuncOf(func(this js.Value, a []js.Value) any {
		blob := js.Global().Get("Blob").New(c.chunks, map[string]any{"type": "video/webm"})
		// Shared with the GIF path, which is where the download race this used
		// to have is explained.
		saveBlob(blob, "webm")
		c.chunks = js.Undefined()
		return nil
	})
	c.recorder.Set("ondataavailable", c.dataFn)
	c.recorder.Set("onstop", c.stopFn)
	c.recorder.Call("start", 1000) // chunk every second
}

func (c *canvasRecorder) stopRecording() {
	c.stopRecFeed()
	if c.recorder.Truthy() && c.recorder.Get("state").String() != "inactive" {
		c.recorder.Call("stop") // onstop finalizes + downloads
	}
	c.recorder = js.Undefined()
}

func (c *canvasRecorder) wireRecordSwitch() {
	sw := dom.Doc.Call("getElementById", "rec-sw")
	if !sw.Truthy() {
		return
	}
	sw.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, args []js.Value) any {
		c.on = sw.Get("checked").Bool()
		if c.on {
			recmod.noteTakeStart()
		}
		switch {
		case c.on && recWantsGIF():
			gif.startGIFRecording()
		case c.on:
			c.startRecording()
		default:
			// Both are stopped regardless of which the switch now says, because
			// the format switch can be flipped mid-recording and the one that
			// is actually running is the one that has to be told.
			c.stopRecording()
			gif.stopGIFRecording()
		}
		return nil
	}))
}

// recWantsGIF reads the format switch. It is read when recording starts rather
// than remembered, so flipping it while nothing is recording just changes what
// the next one will be.
func recWantsGIF() bool {
	sw := dom.Doc.Call("getElementById", "rec-gif-sw")
	return sw.Truthy() && sw.Get("checked").Bool()
}

// recBitrate is how many bits a second to give the encoder, from the size of
// the picture rather than from a fixed number.
//
// MediaRecorder's default is about 2.5 Mbit/s whatever the resolution, and for
// this content that is far too little. A canvas of 1911x994 is nearly two
// megapixels of thin bright lines on black — the worst case for a video codec,
// since every line is high-contrast detail one pixel wide and there is no
// texture anywhere for the bitrate to be saved on. At the default the encoder
// smears the lines and then drops frames trying to keep up: a five second take
// came back with fifty decodable frames and looked, in the reporter's words,
// like the little monitor rather than the canvas.
//
// A tenth of a bit per pixel per frame is generous for photography and about
// right here. The floor keeps small windows from being starved and the ceiling
// keeps a large one from asking for more than the encoder can deliver in real
// time, which would put the frame drops back.
func recBitrate(canvas js.Value, fps int) int {
	w := canvas.Get("width").Float()
	h := canvas.Get("height").Float()
	if w <= 0 || h <= 0 {
		return 12_000_000
	}
	const bitsPerPixelPerFrame = 0.10
	bps := max(int(w*h*float64(fps)*bitsPerPixelPerFrame), 8_000_000)
	if bps > 40_000_000 {
		bps = 40_000_000
	}
	return bps
}

// streamSource returns the element to capture, building the region feed when
// one is needed.
func (c *canvasRecorder) streamSource(canvas js.Value) js.Value {
	sx, sy, sw, sh := recRegionRect(canvas)
	// Handing the canvas straight to MediaRecorder is the cheap path and stays
	// the default — but it can only be done when ONE canvas holds the picture.
	// Split across two, it would record the far half and drop everything in
	// front of the panel, so the per-frame feed below (already built for
	// regions) does the compositing instead.
	if sw >= canvas.Get("width").Float() && sh >= canvas.Get("height").Float() && !splitDrawing() {
		c.stopRecFeed()
		return canvas
	}

	if !c.feed.Truthy() {
		c.feed = dom.Doc.Call("createElement", "canvas")
	}
	c.feed.Set("width", sw)
	c.feed.Set("height", sh)
	c.feedCtx = c.feed.Call("getContext", "2d")
	c.feedOn = true

	// Driven by rAF rather than a timer: the stream samples the canvas when it
	// changes, and the moment it changes is the frame.
	c.feedFn = js.FuncOf(func(js.Value, []js.Value) any {
		if !c.feedOn {
			return nil
		}
		// Cleared first, for the reason the GIF path is: the model's canvas is
		// transparent where nothing is drawn, so drawing it onto another canvas
		// composites instead of replacing and every frame keeps the last one.
		c.feedCtx.Set("fillStyle", "#000000")
		c.feedCtx.Call("fillRect", 0, 0, sw, sh)
		c.feedCtx.Call("drawImage", near.captureCanvas(canvas), sx, sy, sw, sh, 0, 0, sw, sh)
		js.Global().Call("requestAnimationFrame", c.feedFn)
		return nil
	})
	js.Global().Call("requestAnimationFrame", c.feedFn)
	return c.feed
}

func (c *canvasRecorder) stopRecFeed() {
	c.feedOn = false
	if c.feedFn.Truthy() {
		c.feedFn.Release()
		c.feedFn = js.Func{}
	}
}
