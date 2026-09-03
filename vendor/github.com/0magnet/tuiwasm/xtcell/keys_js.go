//go:build js && wasm

package xtcell

import (
	"syscall/js"

	"github.com/gdamore/tcell/v3"
)

// Input does not come through xterm-go.
//
// xterm-go's OnData hands over what a terminal would send to a pty: "\x1b[A"
// for up-arrow, "\x03" for ctrl-C. Turning that back into the key and modifiers
// tcell wants is lossy — "\x03" alone cannot say whether it was ctrl-C or a
// literal ETX, and the alt-modified keys collapse together — so keystrokes are
// taken from a keydown listener instead, which is where the DOM has them in
// full.
//
// The listener goes on the terminal's own hidden textarea — the element
// xterm-go focuses and reads keys through — and not on the document. On the
// document it fired for the whole page: every open window received every
// keystroke, including ones meant for a different one, because the handler also
// calls preventDefault. Scoped to the textarea, a window gets keys when it is
// focused and not otherwise, which is what focus is for.

// bindInput attaches the key handler to term's textarea inside el.
func (s *Screen) bindInput(el js.Value) {
	s.input = findTextarea(el)

	s.keydown = js.FuncOf(func(_ js.Value, a []js.Value) any {
		if len(a) == 0 {
			return nil
		}
		// A keystroke has one destination even though several screens may be
		// drawing at once.
		if current != nil && current != s {
			return nil
		}
		ev := a[0]
		key := ev.Get("key").String()

		ctrl, meta := ev.Get("ctrlKey").Bool(), ev.Get("metaKey").Bool()
		if k, r, mod, ok := decodeKey(key,
			ev.Get("shiftKey").Bool(), ev.Get("altKey").Bool(), ctrl, meta); ok {
			s.post(tcell.NewEventKey(k, r, mod))
		}

		// The browser would otherwise act on the ones a TUI needs most: tab
		// moves focus out of the terminal, ctrl-W closes the tab. Copy and
		// paste are left alone so the terminal keeps working.
		if !(ctrl || meta) || isEditing(key) {
			ev.Call("preventDefault")
		}
		return nil
	})

	// No textarea found, which should not happen for an opened terminal.
	// Falling back to the document keeps the demo usable rather than silently
	// dead, at the cost of the scoping above. If there is no document either —
	// off a page entirely, which is where the tests run — there is nothing to
	// listen on, and trying anyway panics rather than degrades.
	if target := s.keyTarget(); target.Truthy() {
		target.Call("addEventListener", "keydown", s.keydown, true)
	}

	// xterm-go would otherwise also encode the keystroke and pass it on as
	// terminal input, so it would arrive twice. Held rather than dropped, so
	// Fini can put it back.
	s.savedData = s.term.OnData()
	s.term.SetOnData(func(string) {})
}

// decodeKey turns a DOM KeyboardEvent.key and its modifiers into a tcell key.
// ok is false for a keystroke that should not produce an event at all.
func decodeKey(key string, shift, alt, ctrl, meta bool) (tcell.Key, string, tcell.ModMask, bool) {
	// A modifier pressed by itself is not a keystroke.
	switch key {
	case "Control", "Alt", "Meta", "Shift":
		return 0, "", 0, false
	}

	mod := tcell.ModNone
	if shift {
		mod |= tcell.ModShift
	}
	if alt {
		mod |= tcell.ModAlt
	}
	if ctrl {
		mod |= tcell.ModCtrl
	}
	if meta {
		mod |= tcell.ModMeta
	}

	// The named keys.
	if k, ok := webKeyNames[key]; ok {
		return k, "", mod, true
	}

	// Anything else is the text it is. A control chord is reported as the text
	// plus ModCtrl and left at that: NewEventKey turns a rune with ModCtrl into
	// the control key itself, so ev.Key() == tcell.KeyCtrlC matches without this
	// having to know what number KeyCtrlA happens to be.
	return tcell.KeyRune, key, mod, true
}

// webKeyNames maps a DOM KeyboardEvent.key onto the tcell key it means.
//
// tcell v2 exported this as WebKeyNames, for the wasm screen that read the same
// events. v3 deleted that screen, and the table with it, so it lives here now —
// which is where it belongs anyway: it describes what a browser sends, and this
// is the only part of the program that talks to a browser.
//
// Only the keys a browser actually produces are listed. tcell's table also
// carried entries for keys HTML has no name for, which could never match.
var webKeyNames = map[string]tcell.Key{
	"Enter":       tcell.KeyEnter,
	"Backspace":   tcell.KeyBackspace2,
	"Tab":         tcell.KeyTab,
	"Escape":      tcell.KeyEsc,
	"Delete":      tcell.KeyDelete,
	"Insert":      tcell.KeyInsert,
	"ArrowUp":     tcell.KeyUp,
	"ArrowDown":   tcell.KeyDown,
	"ArrowLeft":   tcell.KeyLeft,
	"ArrowRight":  tcell.KeyRight,
	"Home":        tcell.KeyHome,
	"End":         tcell.KeyEnd,
	"PageUp":      tcell.KeyPgUp,
	"PageDown":    tcell.KeyPgDn,
	"Clear":       tcell.KeyClear,
	"Cancel":      tcell.KeyCancel,
	"Pause":       tcell.KeyPause,
	"PrintScreen": tcell.KeyPrint,
	"F1":          tcell.KeyF1,
	"F2":          tcell.KeyF2,
	"F3":          tcell.KeyF3,
	"F4":          tcell.KeyF4,
	"F5":          tcell.KeyF5,
	"F6":          tcell.KeyF6,
	"F7":          tcell.KeyF7,
	"F8":          tcell.KeyF8,
	"F9":          tcell.KeyF9,
	"F10":         tcell.KeyF10,
	"F11":         tcell.KeyF11,
	"F12":         tcell.KeyF12,
}

// keyTarget is what the key handler is attached to: this terminal's own
// textarea, or the document if it has none. Either may be absent.
func (s *Screen) keyTarget() js.Value {
	if s.input.Truthy() {
		return s.input
	}
	return js.Global().Get("document")
}

func (s *Screen) detachKeys() {
	if s.keydown.Truthy() {
		if target := s.keyTarget(); target.Truthy() {
			target.Call("removeEventListener", "keydown", s.keydown, true)
		}
		s.keydown.Release()
		s.keydown = js.Func{}
	}
	if s.savedData != nil {
		s.term.SetOnData(s.savedData)
		s.savedData = nil
	}
}

// focusInput puts the caret in this terminal, so keys arrive here.
func (s *Screen) focusInput() {
	if s.input.Truthy() {
		s.input.Call("focus")
	}
}

// findTextarea locates the hidden input xterm-go reads keys through. It is
// created by Open and identified by its class, since the package exposes no
// accessor for it.
func findTextarea(el js.Value) js.Value {
	if !el.Truthy() {
		return js.Undefined()
	}
	return el.Call("querySelector", ".xterm-helper-textarea")
}

// isEditing reports whether a ctrl/meta chord is one a TUI expects to receive
// rather than one the browser should keep. Ctrl-C and ctrl-V stay with the
// browser so copy and paste still work; everything else goes to the program.
func isEditing(key string) bool {
	switch key {
	case "c", "C", "v", "V", "x", "X":
		return false
	}
	return true
}
