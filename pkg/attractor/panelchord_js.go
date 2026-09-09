//go:build js && wasm

package attractor

import (
	"strings"
	"syscall/js"
)

// The reveal chord: a key combination that brings the whole control surface on
// screen, for host pages that want the model and nothing else until asked.
//
// PanelStartHidden was not enough for that. It collapses the panel's CONTENTS
// and leaves the furniture — the ▤ toggle bottom-left and the dock-target
// cluster bottom-right — which is still a rack's worth of chrome on a page
// whose point is a logo over a turning globe. Measured on magnetosphere.net's
// front page with PanelStartHidden already set: a 218x26 dock cluster and a
// 35x25 toggle button, both visible, on every view of the page.
//
// It is a reveal rather than a second "start hidden" flag because the controls
// have to stay REACHABLE. A page that simply dropped them would be the old
// globe-only backdrop, and the whole rack is the interesting part; what was
// wanted is for it to be out of the way, not gone.

// PanelRevealChord hides the entire control surface — panel, ▤ toggle and dock
// cluster — until this key combination is pressed, which toggles it back and
// forth. Empty (the default) leaves every host exactly as it was.
//
// The syntax is modifiers and one key joined by "+", case-insensitive:
//
//	"ctrl+alt+r"    "shift+meta+K"    "f2"
//
// Recognized modifiers are ctrl, alt, shift and meta (cmd); anything else is
// the key, matched against KeyboardEvent.key. Set BEFORE calling Run().
//
// Revealing does not open the panel. The chord shows the surface in whatever
// state it was left — with PanelStartHidden that is the ▤ button and the dock
// cluster, one press away from the controls — so the chord and the toggle stay
// two separate questions ("may I see the rack at all" and "is it open"), and a
// host can answer them independently.
var PanelRevealChord string

// chord is a parsed PanelRevealChord.
type chord struct {
	ctrl, alt, shift, meta bool
	key                    string // lowercased KeyboardEvent.key
}

// parseChord reads the "ctrl+alt+r" syntax. ok is false for an empty spec or
// one that is all modifiers and no key, so a typo disables the feature rather
// than binding every Ctrl press.
func parseChord(spec string) (chord, bool) {
	var c chord
	for _, part := range strings.Split(spec, "+") {
		switch p := strings.ToLower(strings.TrimSpace(part)); p {
		case "":
			continue
		case "ctrl", "control":
			c.ctrl = true
		case "alt", "option":
			c.alt = true
		case "shift":
			c.shift = true
		case "meta", "cmd", "command", "super", "win":
			c.meta = true
		default:
			c.key = p
		}
	}
	return c, c.key != ""
}

// matches reports whether a keypress is this chord. Modifiers are matched
// exactly in both directions: a chord without shift must not fire on
// Ctrl+Shift+R, which is a browser command the page has no business eating.
func (c chord) matches(key string, ctrl, alt, shift, meta bool) bool {
	return strings.ToLower(key) == c.key &&
		ctrl == c.ctrl && alt == c.alt && shift == c.shift && meta == c.meta
}

// panelSurfaceHidden tracks the chord's own state. It is not read back off the
// DOM because the two elements it hides have their own visibility rules — the
// ▤ button hides the shell, applyDock re-shows it — and a toggle that guessed
// from either one would fall out of step with the other.
var panelSurfaceHidden bool

// controlSurface is every element the chord governs: the shell (panel, resize
// strip, float grip, dock cluster) and the free-floating ▤ button, which is a
// sibling of the shell rather than a child so that it can bring the shell back.
func controlSurface() []js.Value {
	var els []js.Value
	if sh := doc.Call("getElementById", "panel-shell"); sh.Truthy() {
		els = append(els, sh)
	}
	if b := doc.Call("getElementById", "panel-toggle"); b.Truthy() {
		els = append(els, b)
	}
	return els
}

// setPanelSurfaceHidden shows or hides the control surface as a unit.
//
// visibility, not display: display:none would take the shell out of layout,
// and applyDock measures it to place the dock cluster and the resize strip off
// its edge. Hiding it that way and revealing it later left the furniture
// placed off a zero-sized box. Keeping it in flow costs nothing on a page that
// never reveals it — an invisible element is not painted — and means the
// reveal needs no re-layout pass at all.
func setPanelSurfaceHidden(hide bool) {
	panelSurfaceHidden = hide
	v := ""
	if hide {
		v = "hidden"
	}
	for _, el := range controlSurface() {
		el.Get("style").Set("visibility", v)
	}
}

// initPanelRevealChord hides the surface and binds the chord. A no-op unless
// the host set one.
//
// The listener goes on the window in the CAPTURE phase, ahead of the module
// keyboards. Keys plays notes off plain letters and the knob keys steer the
// selected knob, and while all of those ignore modifiers today, a reveal chord
// that stopped working because some later module claimed Alt would be a
// puzzling thing to debug — the one binding that has to work when the rack is
// invisible is the one that makes it visible.
func initPanelRevealChord() {
	c, ok := parseChord(PanelRevealChord)
	if !ok {
		return
	}
	setPanelSurfaceHidden(true)
	js.Global().Call("addEventListener", "keydown", trackedFuncOf(func(_ js.Value, a []js.Value) interface{} {
		if len(a) == 0 {
			return nil
		}
		e := a[0]
		if !c.matches(e.Get("key").String(), e.Get("ctrlKey").Bool(), e.Get("altKey").Bool(),
			e.Get("shiftKey").Bool(), e.Get("metaKey").Bool()) {
			return nil
		}
		e.Call("preventDefault")
		setPanelSurfaceHidden(!panelSurfaceHidden)
		if !panelSurfaceHidden {
			// First reveal after a hidden boot: nothing inside the panel was
			// ever measurable, so the module widths have no quantization and
			// the resize strip no position. Same recovery the ▤ button does.
			quantizeModuleWidths()
			positionResizeHandle()
		}
		return nil
	}), map[string]any{"capture": true})
}
