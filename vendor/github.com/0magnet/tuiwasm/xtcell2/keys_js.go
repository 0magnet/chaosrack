//go:build js && wasm

package xtcell2

import (
	"syscall/js"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
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
func decodeKey(key string, shift, alt, ctrl, meta bool) (tcell.Key, rune, tcell.ModMask, bool) {
	// A modifier pressed by itself is not a keystroke.
	switch key {
	case "Control", "Alt", "Meta", "Shift":
		return 0, 0, 0, false
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

	// The named keys, from tcell's own table so the two agree.
	if k, ok := tcell.WebKeyNames[key]; ok {
		return k, 0, mod, true
	}

	// Anything else is the rune it is. A control chord is reported as the rune
	// plus ModCtrl and left at that: NewEventKey turns KeyRune with ModCtrl
	// into the control key itself (key.go:270), so ev.Key() == tcell.KeyCtrlC
	// matches without this having to know that KeyCtrlA happens to be 65.
	r, _ := utf8.DecodeRuneInString(key)
	return tcell.KeyRune, r, mod, true
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
