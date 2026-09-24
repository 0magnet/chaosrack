//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"strconv"
	"strings"
	"syscall/js"

	winbox "github.com/0magnet/winbox-go"
)

// infoPane is the Info text's window.
type infoPane struct {
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
	window *winbox.WinBox

	// Where the window was left. Saved like the floating panel's geometry, because
	// the position is the thing the person arranged and a window that forgets it is
	// a window they have to arrange again every time.
	x, y   float64
	w, h   float64
	placed bool
}

var info = infoPane{
	w: 520.0,
	h: 460.0,
}

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
func (in *infoPane) loadInfoGeom() {
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
	in.x, in.y, in.w, in.h = v[0], v[1], v[2], v[3]
	in.placed = true
}

func (in *infoPane) saveInfoGeom() {
	lsSet(infoLSKey, strconv.FormatFloat(in.x, 'f', 0, 64)+","+
		strconv.FormatFloat(in.y, 'f', 0, 64)+","+
		strconv.FormatFloat(in.w, 'f', 0, 64)+","+
		strconv.FormatFloat(in.h, 'f', 0, 64))
}

// placeInfoWindow picks a first position: clear of the control panel, so the
// window does not open on top of the thing that opened it.
func (in *infoPane) placeInfoWindow() {
	if in.placed {
		// A saved position from a larger screen must not strand it off-screen.
		if in.x > winW()-80 {
			in.x = winW() - in.w - 20
		}
		if in.y > winH()-80 {
			in.y = winH() - in.h - 20
		}
		if in.x < 0 {
			in.x = 20
		}
		if in.y < 0 {
			in.y = 70
		}
		return
	}
	in.x, in.y = 20, 70
	if layout.standalone {
		if p := dom.Doc.Call("getElementById", "controls-panel"); p.Truthy() &&
			p.Get("style").Get("display").String() != "none" {
			r := p.Call("getBoundingClientRect")
			switch layout.dockEdge {
			case "left":
				in.x = r.Get("right").Float() + 48 // clear the vertical dock-controls tab
			case "right":
				in.x = r.Get("left").Float() - in.w - 48
			case "top":
				in.y = r.Get("bottom").Float() + 20
			}
		}
	}
	if in.x < 0 {
		in.x = 20
	}
	if in.x+in.w > winW() {
		in.w = winW() - in.x - 20
		if in.w < infoMinW {
			in.x, in.w = 20, infoMinW
		}
	}
	if in.y+in.h > winH() {
		in.h = winH() - in.y - 20
		if in.h < infoMinH {
			in.h = infoMinH
		}
	}
	in.placed = true
}

// showInfoWindow opens the Info window, building it on first use and reusing it
// after — the same rule the floating panel follows, and for the same reason.
func (in *infoPane) showInfoWindow() {
	if in.window != nil {
		in.window.Show().Focus()
		updateInfoOverlay() // one place decides what the text says
		return
	}
	in.loadInfoGeom()
	in.placeInfoWindow()
	in.window = winbox.New(&winbox.Options{
		ID:        "info-window",
		Title:     infoTitle(),
		Class:     []string{"info-window"},
		Mount:     infoContent(),
		X:         winbox.Px(in.x),
		Y:         winbox.Px(in.y),
		Width:     winbox.Px(in.w),
		Height:    winbox.Px(in.h),
		MinWidth:  winbox.Px(infoMinW),
		MinHeight: winbox.Px(infoMinH),
		// Minimized and maximized geometry is winbox's, not the person's — the
		// floating panel learned this the hard way, coming back from a minimize
		// as a bare title bar because the parking slot's size had been saved.
		OnMove: func(w *winbox.WinBox, x, y float64) {
			if w.Min || w.Max {
				return
			}
			in.x, in.y = x, y
			in.saveInfoGeom()
		},
		OnResize: func(w *winbox.WinBox, wd, h float64) {
			if w.Min || w.Max {
				return
			}
			in.w, in.h = wd, h
			in.saveInfoGeom()
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
func (in *infoPane) hideInfoWindow() {
	if in.window != nil {
		in.window.Hide()
	}
}

// infoTitle names the model the text is about, so a window left open while the
// model changes says which one it is describing.
func infoTitle() string {
	if m, ok := modeInfo[run.selectedMode]; ok && m.Label != "" {
		return m.Label
	}
	return "model info"
}

func (in *infoPane) updateInfoTitle() {
	if in.window != nil {
		in.window.SetTitle(infoTitle())
	}
}
