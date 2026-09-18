//go:build js && wasm

package xtcell

import (
	"syscall/js"

	"github.com/gdamore/tcell/v3"
)

// Mouse reporting. Pointer events on the terminal's element become tcell
// mouse events: button presses and releases (the release is the event
// whose buttons no longer include the one that went down, exactly as
// tcell reports it), wheel motion, and — where the EnableMouse flags ask
// for them — drag and bare motion, one event per cell the pointer enters.
//
// Unlike the keyboard, the pointer is spatial: the listeners live on this
// screen's own element, so there is no claim to arbitrate between screens —
// an event that lands here is ours. There is still one to make against the
// terminal underneath, which would otherwise read the same press as the start
// of a text selection; see claimMouse.

// buttonsMask maps the DOM MouseEvent.buttons bitmask onto tcell's.
func buttonsMask(b int) tcell.ButtonMask {
	var m tcell.ButtonMask
	if b&1 != 0 {
		m |= tcell.ButtonPrimary
	}
	if b&2 != 0 {
		m |= tcell.ButtonSecondary
	}
	if b&4 != 0 {
		m |= tcell.ButtonMiddle
	}
	return m
}

// wheelMask maps wheel deltas onto tcell's wheel "buttons".
func wheelMask(dx, dy float64) tcell.ButtonMask {
	var m tcell.ButtonMask
	switch {
	case dy < 0:
		m |= tcell.WheelUp
	case dy > 0:
		m |= tcell.WheelDown
	}
	switch {
	case dx < 0:
		m |= tcell.WheelLeft
	case dx > 0:
		m |= tcell.WheelRight
	}
	return m
}

// reportsMotion reports whether a pointer move with these buttons held is
// one the flags in force ask for.
//
// tcell's flags nest rather than partition — MouseMotionEvents is
// documented as "all mouse events (includes click and drag events)" — so a
// drag is reported under either of the two, and a bare move only under
// motion.
func reportsMotion(flags tcell.MouseFlags, btns tcell.ButtonMask) bool {
	if btns != tcell.ButtonNone {
		return flags&(tcell.MouseDragEvents|tcell.MouseMotionEvents) != 0
	}
	return flags&tcell.MouseMotionEvents != 0
}

func mouseMods(ev js.Value) tcell.ModMask {
	var mod tcell.ModMask
	if ev.Get("shiftKey").Bool() {
		mod |= tcell.ModShift
	}
	if ev.Get("altKey").Bool() {
		mod |= tcell.ModAlt
	}
	if ev.Get("ctrlKey").Bool() {
		mod |= tcell.ModCtrl
	}
	if ev.Get("metaKey").Bool() {
		mod |= tcell.ModMeta
	}
	return mod
}

// gridRect is the box the cells are actually drawn in.
//
// Not the mount element: xterm-go lays the grid out inside it beside a
// scrollbar, so the element is the wider of the two by whatever the scrollbar
// takes. Dividing by the element's width stretches every column slightly and
// the error accumulates rightward along the row — at the right-hand edge of a
// 270-column terminal it names a cell two to the left of the one under the
// pointer, which is enough to miss a link.
func (s *Screen) gridRect() js.Value {
	if screen := s.el.Call("querySelector", ".xterm-screen"); screen.Truthy() {
		return screen.Call("getBoundingClientRect")
	}
	return s.el.Call("getBoundingClientRect")
}

// cellAt turns an event's client coordinates into a cell position. The
// terminal is a uniform grid filling its element, so the division is the
// geometry; the emulator's own font metrics never need to be asked.
func (s *Screen) cellAt(ev js.Value) (int, int) {
	rect := s.gridRect()
	relX := ev.Get("clientX").Float() - rect.Get("left").Float()
	relY := ev.Get("clientY").Float() - rect.Get("top").Float()
	w, h := rect.Get("width").Float(), rect.Get("height").Float()
	s.mu.Lock()
	cols, rows := s.cols, s.rows
	s.mu.Unlock()
	if w <= 0 || h <= 0 || cols <= 0 || rows <= 0 {
		return 0, 0
	}
	x := int(relX * float64(cols) / w)
	y := int(relY * float64(rows) / h)
	if x < 0 {
		x = 0
	}
	if x >= cols {
		x = cols - 1
	}
	if y < 0 {
		y = 0
	}
	if y >= rows {
		y = rows - 1
	}
	return x, y
}

// bindMouse attaches the pointer handlers. They are always attached and
// gated by the EnableMouse flag at event time, so enabling the mouse
// before or after Init both work.
func (s *Screen) bindMouse(el js.Value) {
	if !el.Truthy() {
		// No element — the fake terminal in the tests. Keys have a
		// textarea to find or not; the pointer has only the element.
		return
	}
	button := func(a []js.Value) {
		if len(a) == 0 || !s.mouseOn {
			return
		}
		ev := a[0]
		x, y := s.cellAt(ev)
		s.lastCellX, s.lastCellY = x, y
		s.post(tcell.NewEventMouse(x, y, buttonsMask(ev.Get("buttons").Int()), mouseMods(ev)))
	}
	s.mdown = js.FuncOf(func(_ js.Value, a []js.Value) any {
		// A press with the pointer claimed is the one that would have
		// replaced any standing selection had the terminal been handling
		// it. Since it is not, drop the selection here — otherwise text
		// taken with shift stays painted over the application and no
		// click can dismiss it. The terminal keeps xterm.js's own rule
		// (it does not clear one there either); claiming the pointer is
		// this package's decision, so the consequence is too.
		if s.mouseOn && len(a) > 0 && !a[0].Get("shiftKey").Bool() {
			if h, ok := s.term.(mouseHost); ok {
				h.ClearSelection()
			}
		}
		button(a)
		return nil
	})
	s.mup = js.FuncOf(func(_ js.Value, a []js.Value) any {
		button(a)
		return nil
	})
	s.mmove = js.FuncOf(func(_ js.Value, a []js.Value) any {
		if len(a) == 0 || !s.mouseOn {
			return nil
		}
		ev := a[0]
		btns := buttonsMask(ev.Get("buttons").Int())
		if !reportsMotion(s.mouseFlags, btns) {
			return nil
		}
		x, y := s.cellAt(ev)
		// One event per cell entered, not per pixel traveled. A TUI
		// redraws on every event it is handed, and a pointer crossing a
		// terminal generates hundreds of moves within a single cell; at
		// that rate the queue (which drops when full) would throw away
		// the clicks among them.
		if x == s.lastCellX && y == s.lastCellY {
			return nil
		}
		s.lastCellX, s.lastCellY = x, y
		s.post(tcell.NewEventMouse(x, y, btns, mouseMods(ev)))
		return nil
	})
	s.mwheel = js.FuncOf(func(_ js.Value, a []js.Value) any {
		if len(a) == 0 || !s.mouseOn {
			return nil
		}
		ev := a[0]
		wheel := wheelMask(ev.Get("deltaX").Float(), ev.Get("deltaY").Float())
		if wheel == 0 {
			return nil
		}
		x, y := s.cellAt(ev)
		s.post(tcell.NewEventMouse(x, y, wheel|buttonsMask(ev.Get("buttons").Int()), mouseMods(ev)))
		// The page must not scroll under the terminal.
		ev.Call("preventDefault")
		return nil
	})
	el.Call("addEventListener", "mousedown", s.mdown, true)
	el.Call("addEventListener", "mouseup", s.mup, true)
	el.Call("addEventListener", "mousemove", s.mmove, true)
	// passive:false, or preventDefault is ignored for wheel events.
	el.Call("addEventListener", "wheel", s.mwheel, map[string]any{"passive": false, "capture": true})
}

func (s *Screen) detachMouse() {
	if !s.el.Truthy() {
		return
	}
	for _, h := range []struct {
		name string
		fn   *js.Func
	}{{"mousedown", &s.mdown}, {"mouseup", &s.mup}, {"mousemove", &s.mmove}, {"wheel", &s.mwheel}} {
		if h.fn.Truthy() {
			s.el.Call("removeEventListener", h.name, *h.fn, true)
			h.fn.Release()
			*h.fn = js.Func{}
		}
	}
}

// EnableMouse turns on mouse reporting: button presses and releases, the
// wheel, and — per the flags — drag and motion.
//
// The flags mean what they mean to tcell's own screens, no flags included:
// that is every kind of event, so a program that just calls EnableMouse()
// gets motion, as it would in a terminal.
func (s *Screen) EnableMouse(flags ...tcell.MouseFlags) {
	var f tcell.MouseFlags
	for _, flag := range flags {
		f |= flag
	}
	if f == 0 {
		f = tcell.MouseMotionEvents | tcell.MouseDragEvents | tcell.MouseButtonEvents
	}
	s.mouseFlags = f
	s.mouseOn = true
	s.claimMouse(true)
}

// DisableMouse stops mouse reporting.
func (s *Screen) DisableMouse() {
	s.mouseOn = false
	s.claimMouse(false)
}

// mouseHost is a terminal this screen can take the pointer from: told that
// an application has it, and told to drop a selection the application's
// click has displaced. Optional, because the fake terminal in the tests is
// neither.
type mouseHost interface {
	ClaimMouse(bool)
	ClearSelection()
}

// claimMouse tells the terminal that the pointer belongs to the application
// while this screen has the mouse enabled.
//
// The listeners above run in the capture phase, so the tcell event is posted
// either way — but the terminal's own handlers still run afterwards on the
// same click, and an emulator with no application asking for the mouse treats
// a press as the start of a text selection. The result was that every click on
// a link also smeared a selection across the screen, and it stayed there.
//
// Saying so through the core mouse service rather than swallowing the event is
// what makes shift-to-select keep working: xterm-go already yields the pointer
// to an application that has asked for it and takes it back for a shifted
// click, which is the convention every terminal emulator uses for exactly this
// conflict. It also stops the wheel scrolling the emulator's scrollback out
// from under a full-screen TUI.
//
// The reports the terminal now encodes go nowhere: bindInput has already
// replaced OnData for as long as this screen is running.
func (s *Screen) claimMouse(on bool) {
	if h, ok := s.term.(mouseHost); ok {
		h.ClaimMouse(on)
	}
}
