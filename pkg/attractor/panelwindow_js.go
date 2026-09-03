//go:build js && wasm

package attractor

// Float mode: the panel as a real window, from github.com/0magnet/winbox-go.
//
// This replaces about a hundred and thirty lines of hand-rolled window — a
// drag title bar, a corner resize grip, a clamp to keep the title bar reachable
// after the viewport shrank, and geometry persisted by hand. All of it worked;
// none of it was about attractors, and winbox does the same job with eight
// resize edges instead of one corner, plus maximize, minimize to a taskbar, and
// browser fullscreen for nothing.
//
// Only float. The docked edges are not windows here and deliberately have no
// top chrome — the dock cluster clips onto the resize bar instead, which is why
// the panel can be a thin strip along an edge without wasting a title bar's
// height on it. winbox would dock this perfectly well and would insist on the
// chrome; since the shell made docking forty lines of CSS, there is nothing
// left there for a window manager to improve.
//
// What moves into the window is the SHELL, not the panel, so the resize bar and
// the dock cluster travel with it — the cluster is how you get back out to a
// docked edge, so it has to come along.

import winbox "github.com/0magnet/winbox-go"

var panelWindow *winbox.WinBox

// floatPanelWindow puts the shell in a window, creating it on first use and
// reusing it after — a window closed and rebuilt would lose its position, and
// the position is the thing the user arranged.
func floatPanelWindow() {
	shell := doc.Call("getElementById", "panel-shell")
	if !shell.Truthy() {
		return
	}
	if panelWindow != nil {
		panelWindow.Show().Focus()
		return
	}

	clampFloatPos() // a stale saved position must not strand it off-screen

	panelWindow = winbox.New(&winbox.Options{
		ID:        "panel-window",
		Title:     "chaosrack controls",
		Class:     []string{"panel-window"},
		Mount:     shell,
		X:         winbox.Px(floatX),
		Y:         winbox.Px(floatY),
		Width:     winbox.Px(floatW),
		Height:    winbox.Px(floatH),
		MinWidth:  winbox.Px(floatMinW),
		MinHeight: winbox.Px(floatMinH),
		// PARKED IS NOT PLACED. winbox puts a minimized window in a slot along
		// the bottom, and a maximized one over the whole viewport, by the same
		// path a drag takes — so these fire for both, with a geometry the person
		// never chose. Saving it meant the panel came back from a minimize as a
		// bare title bar, and came back that way on every later load, because
		// the slot's 251x35 had been written to localStorage. Min and Max are
		// set before the parking now (winbox-go 8ce684c), which is what makes
		// this check possible at all.
		OnMove: func(w *winbox.WinBox, x, y float64) {
			if w.Min || w.Max {
				return
			}
			floatX, floatY = x, y
			saveFloatGeom()
		},
		OnResize: func(w *winbox.WinBox, wd, h float64) {
			if w.Min || w.Max {
				return
			}
			floatW, floatH = wd, h
			saveFloatGeom()
			// The modules re-flow into however many columns now fit.
			quantizeModuleWidths()
		},
		// Closing means different things depending on whether the desk is the
		// environment.
		//
		// On its own, the panel has nowhere else to be, so closing puts it back
		// on the edge it was last docked to — what the dock cluster's buttons
		// would have done. Losing the whole control surface with no way to ask
		// for it back is not a thing a close button should be able to do.
		//
		// Under Contain there IS somewhere else: the desk's launcher carries a
		// chaosrack entry, so closing can mean closed, like every other window
		// on that desk. The shell has to be taken out of the window first —
		// it is a CHILD of it, and letting winbox remove the window would take
		// the entire rack's DOM along with it.
		OnClose: func(w *winbox.WinBox, _ bool) bool {
			panelWindow = nil
			if deskContain {
				w.Unmount(body)
				if sh := doc.Call("getElementById", "panel-shell"); sh.Truthy() {
					sh.Get("style").Set("display", "none")
				}
				return false
			}
			applyDock(lastDockedEdge())
			return false
		},
	})

	// Both of these belong to the window rather than to the moment Contain was
	// switched on, and the window can be closed and rebuilt — docking away and
	// floating again does exactly that. Attaching them here is what keeps a
	// rebuilt window behaving like the one it replaced.
	rackHidesOnMinimize()
	if deskContain {
		trackRackInDeskPanel()
	}
}

// unfloatPanelWindow takes the shell back out of the window and closes it, for
// when a dock edge is chosen instead.
func unfloatPanelWindow() {
	if panelWindow == nil {
		return
	}
	w := panelWindow
	// Cleared first: Close runs OnClose, which would otherwise re-enter
	// applyDock and fight the dock that is being applied right now.
	panelWindow = nil
	w.Unmount(body)
	w.Close(true)
}

// lastDockedEdge is where the panel goes when the window is closed: whatever it
// was docked to before floating, or the bottom.
func lastDockedEdge() string {
	if s, ok := lsGet("wasmstuff-dock-prefloat"); ok && s != "float" {
		return s
	}
	return "bottom"
}

// rememberPreFloatEdge records the edge to come back to, called as float is
// entered from somewhere else.
func rememberPreFloatEdge(edge string) {
	if edge == "float" {
		return
	}
	lsSet("wasmstuff-dock-prefloat", edge)
}

// reclampPanelWindow pulls the floating window back into view after the
// viewport shrank, so a position saved on a big screen cannot strand the panel
// off the edge of a small one.
//
// winbox does not do this by itself — a floating window keeps its coordinates
// across a resize, which is the right default for a window the user placed and
// the wrong one for a control panel restored from storage.
func reclampPanelWindow() {
	if panelWindow == nil {
		return
	}
	clampFloatPos()
	panelWindow.Move(winbox.Px(floatX), winbox.Px(floatY))
}
