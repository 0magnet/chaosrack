//go:build js && wasm

package attractor

import (
	"github.com/0magnet/desk"
	winbox "github.com/0magnet/winbox-go"
)

// Contain: the desk as the environment, and this app inside it.
//
// The OTHER arrangement puts the desk inside this app: the Desk model, drawn on
// a quad in the scene and turning with it. This is the inversion. The desk's
// panel takes the bottom of the screen, the rack becomes a WINDOW on that desk
// with a task button like anything else, and what is left behind them is the
// scene, still integrating.
//
// Those two are the whole set. A third once existed — a switch that put the
// desk's windows on the page over the canvas, with the rack still docked
// underneath — and it belonged to neither: not the model, not the environment,
// just windows floating over a scene. The switch that summons the desk means
// this arrangement now.
//
// IT NEEDED ALMOST NOTHING NEW, which is worth saying because it sounds like a
// rewrite. Three things already existed and only had to be pointed at each
// other:
//
//   - the rack has been a real winbox window in float mode for a long time,
//     with the minimize button winbox gives every window;
//   - desk's panel takes a Window that is a TITLE, a Focus function and an
//     Alive function — not a winbox handle, not a desk-launched app — so it
//     will adopt anything that can say those three things;
//   - the desk itself is already mounted, already stacked between the canvas
//     and the rack, and already knows how to keep out of the mouse's way.
//
// So the rack is floated, described to the desk's panel, and that is the whole
// of it. Minimizing the rack hides it and leaves its task button; clicking the
// button brings it back. That is xfce4 behavior, and none of it is
// reimplemented here.

var deskContain bool

// setDeskContain switches the arrangement.
func setDeskContain(on bool) {
	deskContain = on
	if !on {
		showDeskPanel(false)
		// And put the desk away, unless it is on screen for the other reason: this
		// switch is the only way to ask for the desk as an environment now, so
		// turning it off has to mean the desk goes -- otherwise its windows stay
		// floating over the canvas, which is the arrangement that was removed for
		// belonging to neither the model nor the desktop.
		if selectedMode != "desk" && bgVisual != "desk" {
			setDeskOn(false)
		}
		return
	}
	setDeskOn(true) // builds it, shows it, and applies the selected style
	if !deskEl.Truthy() {
		return
	}
	showDeskPanel(true)

	// The rack has to be a window before it can be a window ON something. A
	// docked rack owns the bottom edge, which is where the desk's panel goes,
	// and two bars fighting for one edge is the thing this arrangement exists
	// to stop.
	if dockEdge != "float" {
		applyDock("float")
	}
	trackRackInDeskPanel()
	rackHidesOnMinimize()
}

// showDeskPanel reveals or hides the desk's own panel.
//
// visibility rather than display, matching makeDeskPanel: hidden but LAID OUT,
// so the compositor can still read its rectangle and draw it when the desk is
// being used as a model.
func showDeskPanel(on bool) {
	if !deskEl.Truthy() {
		return
	}
	v := "hidden"
	if on {
		v = "visible"
	}
	for _, sel := range []string{".dk-panel", ".dk-menu"} {
		if el := deskEl.Call("querySelector", sel); el.Truthy() {
			el.Get("style").Set("visibility", v)
		}
	}
}

// trackRackInDeskPanel gives the rack a task button on the desk's panel.
//
// desk.Window is three fields and no window type at all, which is what lets an
// app the desk did not launch — and a window it did not create — sit in its
// task list beside the ones it did.
func trackRackInDeskPanel() {
	if deskPanel == nil || !deskEl.Truthy() {
		return
	}
	// IDEMPOTENT BY LOOKING, not by remembering. A one-shot flag was wrong: the
	// button is dropped by the panel's own tick whenever Alive goes false, which
	// happens every time the rack docks — and turning Contain off does exactly
	// that. Coming back afterwards found the flag still set and added nothing, so
	// the second Contain had a panel with no chaosrack button on it and a rack
	// with no way back from a minimize.
	tasks := deskEl.Call("querySelectorAll", ".dk-task")
	for i := 0; i < tasks.Length(); i++ {
		if tasks.Index(i).Get("textContent").String() == rackTaskTitle {
			return
		}
	}
	deskPanel.Track(&desk.Window{
		Title: rackTaskTitle,
		// SHOW, then restore, then focus, in that order and all three. Minimizing
		// hides the window outright now (see rackHidesOnMinimize), Show clears
		// hidden and leaves MINIMIZED alone because they are different states in
		// winbox, and focusing something still minimized looks like a dead button.
		Focus: func() {
			if panelWindow != nil {
				panelWindow.Show().Restore().Focus()
				return
			}
			floatPanelWindow()
		},
		// The button goes away if the rack is docked again, because then it is
		// not a window and there is nothing for the button to raise.
		Alive: func() bool { return panelWindow != nil && dockEdge == "float" },
	})
}

// rackTaskTitle is the rack's name on the desk's panel, and the thing looked for
// when deciding whether it is already there.
const rackTaskTitle = "chaosrack"

// rackHidesOnMinimize makes the rack window vanish when it is minimized, leaving
// its task button as the only thing representing it — which is what every
// paneled desktop does, and what winbox on its own does not.
//
// winbox parks a minimized window as a title-bar stub along the bottom. With a
// panel that is the same window twice, and it is a trap besides: on the parked
// stub the MINIMIZE control is hidden by CSS — measured width 0 — while MAXIMIZE
// sits where a person reaches to bring the window back, so the obvious click on
// a minimized rack fills the screen with it instead of restoring it.
//
// Conditional on the panel being there, and that is the whole of why: with no
// panel the stub is the only way back, and hiding on minimize would put the rack
// somewhere it cannot be reached from.
func rackHidesOnMinimize() {
	if panelWindow == nil {
		return
	}
	panelWindow.OnMinimize = func(wb *winbox.WinBox) {
		if deskContain && deskPanel != nil {
			wb.Hide()
		}
	}
}

// deskLayerVisible hides or shows the whole environment.
//
// This is what the ▤ button reaches for while Contain is on: with the desk as
// the environment, hiding "the controls" means hiding the environment — the
// panel and the windows — and leaving the model on screen. Hiding only the rack
// would leave a desktop with a panel and no application, which is not what
// anybody means by that button.
//
// THE RACK WINDOW HAS TO BE HIDDEN SEPARATELY, and the reason is worth keeping
// because it is invisible from the code: winbox mounts on document.body, so the
// rack is not a DESCENDANT of the desk's element even while it is a window ON
// the desk. Hiding the desk's element therefore left it behind — and not even
// as a window. The ▤ button hides #panel-shell, which is the rack's CONTENTS,
// so what stayed on screen was a 251x35 title bar with an empty body: the one
// piece of furniture in an otherwise cleared room.
//
// Hide, not collapse, and Show rather than Restore on the way back — a rack the
// person had minimized should still be minimized when the environment returns.
func deskLayerVisible(on bool) {
	if !deskEl.Truthy() {
		return
	}
	if on {
		deskEl.Get("style").Set("display", "")
	} else {
		deskEl.Get("style").Set("display", "none")
	}
	if panelWindow != nil && dockEdge == "float" {
		if on {
			panelWindow.Show()
		} else {
			panelWindow.Hide()
		}
	}
}

// deskContainHidesEverything reports whether the ▤ button should take the desk
// with it, which it should exactly when the desk is the environment.
func deskContainHidesEverything() bool { return deskContain && deskEl.Truthy() }

// relaunchRack brings the control panel back after its window was closed, and
// is what the desk's launcher entry runs.
//
// The shell was left in the body with display:none by the close handler, so it
// is put back on its feet before the window is built around it. Float is
// asserted rather than assumed: the dock cluster may have sent the panel to an
// edge in the meantime, and launching the rack from a menu should produce a
// window either way.
func relaunchRack() {
	if sh := doc.Call("getElementById", "panel-shell"); sh.Truthy() {
		sh.Get("style").Set("display", "")
	}
	if dockEdge != "float" {
		applyDock("float")
		return // applyDock builds the window itself
	}
	floatPanelWindow()
}
