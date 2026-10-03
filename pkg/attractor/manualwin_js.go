//go:build js && wasm

package attractor

// The manual's windows: a bay or the model, taken out of the manual page into
// a window of its own (winbox-go), to keep in view while the text about it is
// scrolled through. Windows stay where they are put; the manual scrolls under
// them. Closing one puts what it held back where it was.
//
// A bay, not a module: a control is worked beside the ones next to it in its
// bay, and a module on its own put some of those out of reach. A bay in a
// window is the same modules the page held (manuallive_js.go), moved, not
// copied; its place on the page says so and offers it back. The
// model's window holds the rack's own canvas, sized to the window, and is the
// rack switched on: opening it powers the rack up, closing it powers it down,
// and the Console's Power switch, in the manual, opens and closes it.

import (
	"strconv"
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	winbox "github.com/0magnet/winbox-go"
)

// manualWinZ is above the manual page (#rack-manual) and below nothing else.
const manualWinZ = 100010

// manualWins are the windows open, by what they hold: "b:8" a bay, "model"
// the model.
var manualWins = map[string]*winbox.WinBox{}

// manualCascade staggers windows as they open, so one does not land exactly
// over the last.
var manualCascade float64

// wireManualWindows answers the manual page's window buttons.
func wireManualWindows(page js.Value) {
	dom.On(page, "click", func(_ js.Value, a []js.Value) any {
		t := a[0].Get("target")
		if !t.Truthy() || !t.Get("closest").Truthy() {
			return nil
		}
		switch b := t.Call("closest", "[data-pop-bay],[data-back-bay],.mmodel"); {
		case !b.Truthy():
		case b.Call("hasAttribute", "data-pop-bay").Bool():
			if n, err := strconv.Atoi(b.Call("getAttribute", "data-pop-bay").String()); err == nil {
				popManualBay(n)
			}
		case b.Call("hasAttribute", "data-back-bay").Bool():
			if w, ok := manualWins["b:"+b.Call("getAttribute", "data-back-bay").String()]; ok {
				w.Close(true)
			}
		default:
			toggleModelWindow()
		}
		return nil
	})
	manualPowerHook = func(on bool) {
		if on != (modelWin.win != nil) {
			toggleModelWindow()
		}
	}
}

// manualOpen opens a window titled title holding body, w by h, cascaded from
// the top right, and returns it; closing it runs back and forgets key.
func manualOpen(key, title string, body js.Value, w, h float64, resized func(*winbox.WinBox), back func()) *winbox.WinBox {
	w = min(w, winW()-40)
	h = min(h, winH()-60)
	x := winW() - w - 24 - manualCascade
	y := 24 + manualCascade
	if key == "model" {
		// The model in a corner of its own, under the modules' cascade.
		x, y = winW()-w-24, winH()-h-24
	} else {
		manualCascade = float64(int(manualCascade+28) % 168)
	}
	win := winbox.New(&winbox.Options{
		Title:     title,
		Class:     []string{"manual-window"},
		Mount:     body,
		X:         winbox.Px(max(8, x)),
		Y:         winbox.Px(y),
		Width:     winbox.Px(w),
		Height:    winbox.Px(h),
		MinWidth:  winbox.Px(160),
		MinHeight: winbox.Px(90),
		Index:     manualWinZ,
		OnResize: func(win *winbox.WinBox, _, _ float64) {
			if resized != nil {
				resized(win)
			}
		},
		OnClose: func(*winbox.WinBox, bool) bool {
			delete(manualWins, key)
			back()
			return false
		},
	})
	manualWins[key] = win
	return win
}

// popManualBay takes every module of bay n out of the manual into one window,
// side by side as they stand in the rack.
func popManualBay(n int) {
	key := "b:" + strconv.Itoa(n)
	if w, ok := manualWins[key]; ok {
		w.Show().Focus()
		return
	}
	live := manualLive()
	holds := dom.Doc.Call("querySelectorAll", `#rack-manual section#bay-`+strconv.Itoa(n)+` .mlive[data-mloc]`)
	body := dom.Doc.Call("createElement", "div")
	body.Set("className", "mwin-body mwin-bay")
	var locs []string
	total := 0.0
	for i := range holds.Length() {
		loc := holds.Index(i).Call("getAttribute", "data-mloc").String()
		wrap := live.Call("wrapper", loc)
		if !wrap.Truthy() {
			continue
		}
		body.Call("appendChild", wrap)
		live.Call("away", loc)
		locs = append(locs, loc)
		total += live.Call("naturalWidth", loc).Float()
	}
	if len(locs) == 0 {
		return
	}
	fit := func(win *winbox.WinBox) {
		k := min(1, (body.Get("clientWidth").Float()-12)/total)
		for _, loc := range locs {
			live.Call("fit", loc, live.Call("naturalWidth", loc).Float()*k)
		}
	}
	width := min(total+24, winW()-40)
	win := manualOpen(key, bayTitleFromManual(n), body, width, 240, fit, func() {
		for _, loc := range locs {
			live.Call("home", loc)
		}
	})
	fit(win)
	if h := body.Get("scrollHeight").Float(); h > 0 {
		win.Resize(winbox.Px(width), winbox.Px(h+48))
	}
}

// bayTitleFromManual is bay n's heading on the manual page.
func bayTitleFromManual(n int) string {
	if h := dom.Doc.Call("querySelector", `#rack-manual section#bay-`+strconv.Itoa(n)+` h2`); h.Truthy() {
		t := h.Get("firstChild")
		if t.Truthy() {
			return strings.TrimSpace(t.Get("textContent").String())
		}
	}
	return "Bay " + strconv.Itoa(n)
}

// ── the model's window ───────────────────────────────────────────────────

// modelWin is the model's window, and where its canvas came from.
var modelWin struct {
	win    *winbox.WinBox
	body   js.Value
	home   js.Value // the canvas's own parent
	style  string   // the canvas's style before it moved
	moving bool     // the window is opening or closing the rack's power
}

// manualPowerHook is told when the rack's power changes, on the manual page.
var manualPowerHook func(on bool)

// toggleModelWindow opens the model's window, powering the rack up, or
// closes it, powering it down.
func toggleModelWindow() {
	if modelWin.moving {
		return
	}
	modelWin.moving = true
	defer func() { modelWin.moving = false }()
	if modelWin.win != nil {
		modelWin.win.Close(true)
		return
	}
	canvas := glctx.Canvas
	if !canvas.Truthy() {
		return
	}
	body := dom.Doc.Call("createElement", "div")
	body.Set("className", "mwin-body mwin-model")
	modelWin.body = body
	modelWin.home = canvas.Get("parentNode")
	modelWin.style = canvas.Get("style").Get("cssText").String()
	body.Call("appendChild", canvas)
	cs := canvas.Get("style")
	cs.Set("position", "absolute")
	cs.Set("left", "0")
	cs.Set("top", "0")
	canvasBox = func() (int, int) {
		return body.Get("clientWidth").Int(), body.Get("clientHeight").Int()
	}
	resize := func(*winbox.WinBox) {
		if gpu.sizeCanvasToViewport() {
			glctx.GL.Call("viewport", 0, 0, gpu.width, gpu.height)
			setupMatrices()
		}
	}
	title := "the model"
	if m, ok := modeInfo[run.selectedMode]; ok && m.Label != "" {
		title = m.Label
	}
	modelWin.win = manualOpen("model", title, body, min(640, winW()*0.45), min(480, winH()*0.5), resize, func() {
		modelWin.win = nil
		canvasBox = nil
		canvas.Get("style").Set("cssText", modelWin.style)
		if modelWin.home.Truthy() {
			modelWin.home.Call("appendChild", canvas)
		}
		prev := modelWin.moving
		modelWin.moving = true
		setPowerState(false)
		modelWin.moving = prev
	})
	resize(modelWin.win)
	setPowerState(true)
}
