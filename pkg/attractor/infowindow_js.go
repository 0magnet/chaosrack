//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"strconv"
	"strings"
	"syscall/js"

	winbox "github.com/0magnet/winbox-go"
)

// The Info text, in a window.
//
// It was a fixed-position caption pinned over the canvas with pointer-events
// off, no height limit and no overflow — so a description taller than the
// viewport simply ran off the bottom of the screen, and could not be scrolled
// to, because an element that takes no pointer events takes no wheel either.
// That is not a rare case: measured across all 76 models, four descriptions
// overflow a 983-pixel viewport (the Waterfall's loses a fifth of itself), six
// overflow 800 and eleven overflow 640. The longest are exactly the ones worth
// reading — Waterfall, the stereo embedding, Takens — because a display that
// needs explaining has a long explanation.
//
// A window is the rack's own answer to this and costs nothing new: the control
// panel already becomes one in float mode, from the same package. It scrolls,
// it moves off whatever you wanted to look at, it resizes, and it has a close
// button, so the Info switch stops being the only way back out.
var infoWindow *winbox.WinBox

// Where the window was left. Saved like the floating panel's geometry, because
// the position is the thing the person arranged and a window that forgets it is
// a window they have to arrange again every time.
var (
	infoX, infoY = 0.0, 0.0
	infoW, infoH = 520.0, 460.0
	infoPlaced   bool
)

const (
	infoMinW  = 260.0
	infoMinH  = 140.0
	infoLSKey = "wasmstuff-infogeom"
)

// infoContent is the scrollable element the description is written into. It
// keeps the id the rest of the app knows it by, so updateInfoOverlay and the
// turtle's live shape line need no changes at all.
func infoContent() js.Value {
	el := dom.Doc.Call("getElementById", "info-overlay")
	if el.Truthy() {
		return el
	}
	el = dom.Doc.Call("createElement", "div")
	el.Set("id", "info-overlay")
	el.Set("className", "info-body")
	return el
}

// loadInfoGeom restores the saved window geometry, if there is one.
func loadInfoGeom() {
	s, ok := lsGet(infoLSKey)
	if !ok {
		return
	}
	p := strings.Split(s, ",")
	if len(p) != 4 {
		return
	}
	v := make([]float64, 4)
	for i, f := range p {
		n, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return
		}
		v[i] = n
	}
	if v[2] < infoMinW || v[3] < infoMinH {
		return // a stale minimized/parked size is not a size to come back to
	}
	infoX, infoY, infoW, infoH = v[0], v[1], v[2], v[3]
	infoPlaced = true
}

func saveInfoGeom() {
	lsSet(infoLSKey, strconv.FormatFloat(infoX, 'f', 0, 64)+","+
		strconv.FormatFloat(infoY, 'f', 0, 64)+","+
		strconv.FormatFloat(infoW, 'f', 0, 64)+","+
		strconv.FormatFloat(infoH, 'f', 0, 64))
}

// placeInfoWindow picks a first position: clear of the control panel, so the
// window does not open on top of the thing that opened it.
func placeInfoWindow() {
	if infoPlaced {
		// A saved position from a larger screen must not strand it off-screen.
		if infoX > winW()-80 {
			infoX = winW() - infoW - 20
		}
		if infoY > winH()-80 {
			infoY = winH() - infoH - 20
		}
		if infoX < 0 {
			infoX = 20
		}
		if infoY < 0 {
			infoY = 70
		}
		return
	}
	infoX, infoY = 20, 70
	if standalonePanel {
		if p := dom.Doc.Call("getElementById", "controls-panel"); p.Truthy() &&
			p.Get("style").Get("display").String() != "none" {
			r := p.Call("getBoundingClientRect")
			switch dockEdge {
			case "left":
				infoX = r.Get("right").Float() + 48 // clear the vertical dock-controls tab
			case "right":
				infoX = r.Get("left").Float() - infoW - 48
			case "top":
				infoY = r.Get("bottom").Float() + 20
			}
		}
	}
	if infoX < 0 {
		infoX = 20
	}
	if infoX+infoW > winW() {
		infoW = winW() - infoX - 20
		if infoW < infoMinW {
			infoX, infoW = 20, infoMinW
		}
	}
	if infoY+infoH > winH() {
		infoH = winH() - infoY - 20
		if infoH < infoMinH {
			infoH = infoMinH
		}
	}
	infoPlaced = true
}

// showInfoWindow opens the Info window, building it on first use and reusing it
// after — the same rule the floating panel follows, and for the same reason.
func showInfoWindow() {
	if infoWindow != nil {
		infoWindow.Show().Focus()
		updateInfoOverlay() // one place decides what the text says
		return
	}
	loadInfoGeom()
	placeInfoWindow()
	infoWindow = winbox.New(&winbox.Options{
		ID:        "info-window",
		Title:     infoTitle(),
		Class:     []string{"info-window"},
		Mount:     infoContent(),
		X:         winbox.Px(infoX),
		Y:         winbox.Px(infoY),
		Width:     winbox.Px(infoW),
		Height:    winbox.Px(infoH),
		MinWidth:  winbox.Px(infoMinW),
		MinHeight: winbox.Px(infoMinH),
		// Minimized and maximized geometry is winbox's, not the person's — the
		// floating panel learned this the hard way, coming back from a minimize
		// as a bare title bar because the parking slot's size had been saved.
		OnMove: func(w *winbox.WinBox, x, y float64) {
			if w.Min || w.Max {
				return
			}
			infoX, infoY = x, y
			saveInfoGeom()
		},
		OnResize: func(w *winbox.WinBox, wd, h float64) {
			if w.Min || w.Max {
				return
			}
			infoW, infoH = wd, h
			saveInfoGeom()
		},
		// The X closes the window AND turns the switch off, because the switch
		// is what the permalink carries and a window closed behind its own
		// switch's back would be restored open by a link that says it is.
		// Canceling the close and hiding instead keeps the position.
		OnClose: func(w *winbox.WinBox, _ bool) bool {
			w.Hide()
			if sw := dom.Doc.Call("getElementById", "show-info"); sw.Truthy() && sw.Get("checked").Bool() {
				sw.Set("checked", false)
				sw.Call("dispatchEvent", js.Global().Get("Event").New("change"))
			}
			return true
		},
	})
	// After the window exists, not before: updateInfoOverlay finds the body by
	// id, and until winbox has mounted it there is no such element to find.
	updateInfoOverlay()
}

// hideInfoWindow closes the window without destroying it.
func hideInfoWindow() {
	if infoWindow != nil {
		infoWindow.Hide()
	}
}

// infoTitle names the model the text is about, so a window left open while the
// model changes says which one it is describing.
func infoTitle() string {
	if m, ok := modeInfo[selectedMode]; ok && m.Label != "" {
		return m.Label
	}
	return "model info"
}

func updateInfoTitle() {
	if infoWindow != nil {
		infoWindow.SetTitle(infoTitle())
	}
}
