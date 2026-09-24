//go:build js && wasm

package attractor

// The Record module: a monitor, two switches and a transport.
//
// Format and area are two-position switches with the legend either side, which
// is how a panel labels a switch that has two positions. They were rotary
// knobs for one revision, which was wrong: a knob is for three positions or a
// continuum, and spending one on a binary choice makes the reader look for the
// third option. Being switches, they are also simply the state the recorder
// reads — no hidden input behind them to keep in step.
//
// The transport is a pair of buttons, because a recorder has a record button
// and a stop button and not a checkbox labeled "record". Those do drive a
// hidden switch, since the recorder is started and stopped by one.
//
// The monitor shows the RECORDED area, not the canvas. That is what earns it
// the space: with a region set, the difference between what is on the canvas
// and what lands in the file is exactly what you cannot check until afterwards,
// and for a GIF afterwards is after the encode.
//
// Timecode and status are drawn INTO the picture rather than printed beside it,
// the way a field monitor overlays them — and for the same reason, which is
// that they belong to the take rather than to the furniture around it.

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"strconv"
	"syscall/js"
)

// recPreviewFPS is how often the monitor redraws. Low on purpose: this is a
// confidence check, not a second renderer, and it must not cost the model
// frames to look at.
const recPreviewFPS = 10

// recordModule is the Record module's monitor, tally and clock.
type recordModule struct {
	preview    js.Value
	previewCtx js.Value
	tally      js.Value
	counter    js.Value
	startMs    float64

	// takeStartMs is when the current take began, kept separately from the
	// preview's clock: that one is reset the moment recording stops, and the size
	// of a GIF is not known until after it has been encoded, which is later still.
	takeStartMs float64
}

var recmod recordModule

// wireRecordModule builds the module's controls and starts the monitor. Safe to
// call before the panel exists — it does nothing and can be called again.
func (r *recordModule) wireRecordModule() {
	r.preview = dom.Doc.Call("getElementById", "rec-preview")
	if !r.preview.Truthy() {
		return
	}
	r.tally = dom.Doc.Call("getElementById", "rec-tally")
	r.counter = dom.Doc.Call("getElementById", "rec-status")
	r.previewCtx = r.preview.Call("getContext", "2d")

	wireRecTransport()
	wireStillButton()

	tick := dom.FuncOf(func(js.Value, []js.Value) interface{} {
		r.drawRecPreview()
		return nil
	})
	js.Global().Call("setInterval", tick, 1000/recPreviewFPS)
	r.drawRecPreview()
}

// recSetSwitch flips a switch and tells the recorder, exactly as a click on it
// would. Used by the transport buttons, which drive the hidden record switch.
func recSetSwitch(id string, on bool) {
	sw := dom.Doc.Call("getElementById", id)
	if !sw.Truthy() {
		return
	}
	if sw.Get("checked").Bool() == on {
		return
	}
	sw.Set("checked", on)
	ev := js.Global().Get("Event").New("change", map[string]interface{}{"bubbles": true})
	sw.Call("dispatchEvent", ev)
}

func recSwitchOn(id string) bool {
	sw := dom.Doc.Call("getElementById", id)
	return sw.Truthy() && sw.Get("checked").Bool()
}

// wireRecTransport makes the two buttons work. Record toggles, so pressing it
// again stops — which is what a single-button recorder does and what anyone
// will try — and Stop only ever stops.
func wireRecTransport() {
	if b := dom.Doc.Call("getElementById", "rec-btn"); b.Truthy() {
		b.Call("addEventListener", "click", dom.FuncOf(func(js.Value, []js.Value) interface{} {
			recSetSwitch("rec-sw", !recSwitchOn("rec-sw"))
			return nil
		}))
	}
	if b := dom.Doc.Call("getElementById", "rec-stop-btn"); b.Truthy() {
		b.Call("addEventListener", "click", dom.FuncOf(func(js.Value, []js.Value) interface{} {
			recSetSwitch("rec-sw", false)
			return nil
		}))
	}
}

// recActive reports whether either recorder is running — what the tally light,
// the armed button and the clock are all about.
func recActive() bool {
	return gif.recording || (rec.on && rec.recorder.Truthy())
}

func (r *recordModule) drawRecPreview() {
	if !r.previewCtx.Truthy() {
		return
	}
	// Nothing to do if nobody can see it. offsetParent is null when the module
	// is hidden by its switch, when the panel is closed, or when a mode has
	// taken the module away — and a monitor drawn into a hidden element costs
	// exactly as much as one being looked at.
	//
	// The draw below copies out of the WebGL canvas, which makes the browser
	// snapshot the drawing buffer. That is not free, and doing it while the
	// panel is being dragged wider is how a resize came to cost the model a
	// frame: the rack re-measures every module on each pointer move, and this
	// forced a fresh snapshot in the middle of it. The picture is not
	// interesting during a drag anyway.
	if !recScreenPower.on(r.preview) {
		if recScreenPower.needsBlank() {
			r.previewCtx.Set("fillStyle", "#05070a")
			r.previewCtx.Call("fillRect", 0, 0,
				r.preview.Get("width").Float(), r.preview.Get("height").Float())
			recScreenPower.markBlanked()
		}
		return
	}
	pw := r.preview.Get("width").Float()
	ph := r.preview.Get("height").Float()

	// Cleared every frame: with a letterboxed picture the bars would otherwise
	// keep whatever was there when the region's shape last changed.
	r.previewCtx.Set("fillStyle", "#05070a")
	r.previewCtx.Call("fillRect", 0, 0, pw, ph)

	canvas := modelCanvas()
	if !canvas.Truthy() {
		return
	}
	sx, sy, sw, sh := recRegionRect(canvas)
	if sw < 1 || sh < 1 {
		return
	}

	// Fit, not fill: a monitor that stretched its picture would make every
	// region look like the monitor and tell you nothing about the file.
	k := pw / sw
	if ky := ph / sh; ky < k {
		k = ky
	}
	dw, dh := sw*k, sh*k
	dx, dy := (pw-dw)/2, (ph-dh)/2
	r.previewCtx.Call("drawImage", captureCanvas(canvas), sx, sy, sw, sh, dx, dy, dw, dh)

	r.drawRecOSD(pw, ph, sw, sh)
}

// drawRecOSD burns the take's own information into the picture: timecode at the
// top left, the recording flag at the top right, the format and size along the
// bottom.
func (r *recordModule) drawRecOSD(pw, ph, sw, sh float64) {
	live := recActive()
	ctx := r.previewCtx
	ctx.Set("font", "9px 'B612 Mono', monospace")
	ctx.Set("textBaseline", "top")

	// Timecode, running only while recording — a counter that ran while idle
	// would be measuring nothing.
	tc := "00:00:00"
	if live {
		now := js.Global().Get("Date").Call("now").Float()
		if r.startMs == 0 {
			r.startMs = now
		}
		tc = timecode(int((now - r.startMs) / 1000))
	} else {
		r.startMs = 0
	}
	ctx.Set("fillStyle", "rgba(0,0,0,0.55)")
	ctx.Call("fillRect", 0, 0, pw, 13)
	ctx.Call("fillRect", 0, ph-13, pw, 13)
	ctx.Set("fillStyle", "#9fb4c7")
	ctx.Call("fillText", tc, 4, 2)

	if live {
		ctx.Set("fillStyle", "#ff3b3b")
		label := "REC"
		if gif.recording {
			label = "REC GIF"
		}
		// Text only. The lamp on the bezel is the recording light; a second dot
		// inside the picture just repeats it.
		w := ctx.Call("measureText", label).Get("width").Float()
		ctx.Call("fillText", label, pw-w-5, 2)
	}

	// Bottom strip: what would be written if you pressed record now.
	area := strconv.Itoa(int(sw)) + "×" + strconv.Itoa(int(sh))
	if recRegion.w <= 1 || recRegion.h <= 1 {
		area += "  FULL"
	} else {
		area += "  REGION"
	}
	format := "WEBM"
	if recSwitchOn("rec-gif-sw") {
		format = "GIF"
		if live {
			area += "  " + strconv.Itoa(len(gif.frames)) + "f"
		}
	}
	ctx.Set("fillStyle", "#7c93a8")
	ctx.Call("fillText", format+"  "+area, 4, ph-11)

	r.updateRecChrome(live, tc)
}

// updateRecChrome drives the parts outside the picture: the tally lamp, the
// counter, and the record button's own lit state.
func (r *recordModule) updateRecChrome(live bool, tc string) {
	if r.tally.Truthy() {
		if live {
			r.tally.Get("classList").Call("add", "live")
		} else {
			r.tally.Get("classList").Call("remove", "live")
		}
	}
	if b := dom.Doc.Call("getElementById", "rec-btn"); b.Truthy() {
		if live {
			b.Get("classList").Call("add", "armed")
		} else {
			b.Get("classList").Call("remove", "armed")
		}
	}
	if r.counter.Truthy() {
		r.counter.Set("textContent", tc)
		if live {
			r.counter.Get("classList").Call("add", "live")
		} else {
			r.counter.Get("classList").Call("remove", "live")
		}
	}
	updateRecMeter(live)
}

// updateRecMeter fills the media bar with how much of the GIF frame budget the
// take has spent.
//
// Only GIF has a budget: its frames are held in memory until the clip ends, so
// there is a ceiling and the recording stops there by itself. WebM streams as
// it goes and has no such limit, which is why the bar is left empty for it —
// showing it full or part-full would be inventing a number.
//
// Amber at three quarters and red at nine tenths, because the interesting thing
// about the ceiling is not where it is but that you are approaching it.
func updateRecMeter(live bool) {
	fill := dom.Doc.Call("getElementById", "rec-meter-fill")
	if !fill.Truthy() {
		return
	}
	cl := fill.Get("classList")
	cl.Call("remove", "warn")
	cl.Call("remove", "full")

	if !live || !gif.recording {
		fill.Get("style").Set("width", "0%")
		return
	}
	frac := float64(len(gif.frames)) / float64(gifMaxFrames)
	if frac > 1 {
		frac = 1
	}
	switch {
	case frac >= 0.9:
		cl.Call("add", "full")
	case frac >= 0.75:
		cl.Call("add", "warn")
	}
	fill.Get("style").Set("width", strconv.Itoa(int(frac*100))+"%")
}

// timecode renders seconds as HH:MM:SS, which is what a recorder shows even
// when the take is four seconds long.
func timecode(secs int) string {
	if secs < 0 {
		secs = 0
	}
	return pad2(secs/3600) + ":" + pad2((secs/60)%60) + ":" + pad2(secs%60)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// noteTakeStart marks the beginning of a take.
func (r *recordModule) noteTakeStart() {
	r.takeStartMs = js.Global().Get("Date").Call("now").Float()
}

// logTake reports what the finished take came out as. A recorder that never
// said what it had just written would be a strange one, and for a GIF this is
// the only moment the size exists — it is encoded when the take ends.
func (r *recordModule) logTake(ext string, bytes int) {
	el := dom.Doc.Call("getElementById", "rec-log")
	if !el.Truthy() {
		return
	}
	secs := 0
	if r.takeStartMs > 0 {
		secs = int((js.Global().Get("Date").Call("now").Float() - r.takeStartMs) / 1000)
	}
	el.Set("textContent", upper(ext)+"  "+timecode(secs)+"  "+humanBytes(bytes))
	el.Get("classList").Call("add", "fresh")
}

// humanBytes is a size a person reads, not a byte count.
func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return strconv.Itoa(n/(1<<20)) + "." + strconv.Itoa((n%(1<<20))*10/(1<<20)) + " MB"
	case n >= 1<<10:
		return strconv.Itoa(n/(1<<10)) + " KB"
	default:
		return strconv.Itoa(n) + " B"
	}
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - ('a' - 'A')
		}
	}
	return string(b)
}

// wireStillButton makes the still button save a PNG of what the monitor shows.
//
// It moved here from the Console's row of global actions, and the move is not
// only tidiness: a still is a capture of the canvas exactly as a recording is,
// and living in this module it inherits the three things the module knows.
//
// It honors the AREA switch, so a region captures the region — the old button
// always wrote the whole canvas, which with a region set was not what the
// monitor was showing. It goes through saveBlob, so it gets the timestamped
// filename and the download that actually works; the old one wrote
// "attractor.png" every time, so a second still collided with the first. And
// it reports itself on the last-take line like any other capture.
//
// At full resolution, not the monitor's: the monitor is 244 across and a still
// should be worth keeping.
func wireStillButton() {
	btn := dom.Doc.Call("getElementById", "screenshot-btn")
	if !btn.Truthy() {
		return
	}
	btn.Call("addEventListener", "click", dom.FuncOf(func(js.Value, []js.Value) interface{} {
		takeStill()
		return nil
	}))
}

func takeStill() {
	canvas := modelCanvas()
	if !canvas.Truthy() {
		return
	}
	sx, sy, sw, sh := recRegionRect(canvas)
	if sw < 1 || sh < 1 {
		return
	}

	// Its own canvas, not the recorder's scratch: a still taken during a take
	// would resize the scratch out from under the frames being collected.
	still := dom.Doc.Call("createElement", "canvas")
	still.Set("width", sw)
	still.Set("height", sh)
	ctx := still.Call("getContext", "2d")
	ctx.Call("drawImage", captureCanvas(canvas), sx, sy, sw, sh, 0, 0, sw, sh)

	recmod.noteTakeStart() // so the still reports a duration of zero, not the last take's
	var cb js.Func
	cb = js.FuncOf(func(_ js.Value, a []js.Value) interface{} {
		defer cb.Release()
		if len(a) == 0 || !a[0].Truthy() {
			return nil
		}
		saveBlob(a[0], "png")
		return nil
	})
	still.Call("toBlob", cb, "image/png")
}
