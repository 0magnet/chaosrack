//go:build js && wasm

package attractor

import (
	"sync"
	"syscall/js"

	"github.com/0magnet/desk/panes/term"
	"github.com/0magnet/seat"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// The page's screens (0magnet/seat), switched as a machine switches between
// its consoles and its desktop, with Ctrl+Alt and a digit:
//
//	1 instrument  the page as it is: the scene, and the rack docked or floating
//	2 console     a shell filling the screen — `rack` is the panel, in cells,
//	              with the page's own parts laid over them
//	3 desktop     the desk over the scene, the rack a window on it (what the
//	              Desk switch turns on, and still does)
//
// The instrument and the desktop are both the page itself, arranged two ways;
// the console is a screen of its own over it, and keeps running behind the
// others once it has been opened, as a console does.

var pageSeat *seat.Seat

const consoleGreeting = "" +
	"\x1b[1;35mchaosrack\x1b[0m — console\r\n" +
	"\x1b[2mthe model is still running behind this screen\x1b[0m\r\n\r\n" +
	"  \x1b[1mrack\x1b[0m — the control surface, as cells, with the page's own parts over them\r\n" +
	"  \x1b[2mctrl-wheel, or ctrl-+/-, resizes the cell · rack --plain for cells only\x1b[0m\r\n" +
	"  \x1b[2mctrl+alt+1 instrument · ctrl+alt+2 console · ctrl+alt+3 desktop\x1b[0m\r\n\r\n"

// useHostFS gives every terminal the server's filesystem, once, whichever
// asks first: the console or the desk.
var useHostFS = sync.OnceFunc(term.UseHostFS)

// wireSeat makes the page's screens, the instrument in front.
func wireSeat() {
	s := seat.New(seat.Options{})
	s.AddPage("instrument", nil)
	s.Add("console", &consoleScreen{})
	s.AddPage("desktop", func(front bool) {
		setDeskContain(front)
		// The Desk switch shows which screen this is, however it was reached.
		if dc := dom.Doc.Call("getElementById", "desk-contain"); dc.Truthy() {
			dc.Set("checked", front)
		}
	})
	_ = s.Show("instrument") //nolint:errcheck // a page screen always shows
	pageSeat = s
}

// showDesktop is the Desk switch: the desktop, or back to the instrument.
func showDesktop(on bool) {
	if pageSeat == nil {
		setDeskContain(on)
		return
	}
	switch {
	case on:
		_ = pageSeat.Show("desktop") //nolint:errcheck // a page screen always shows
	case pageSeat.Current() == "desktop":
		_ = pageSeat.Show("instrument") //nolint:errcheck // as above
	}
}

// consoleScreen is a shell with the rack's commands in it, made when the
// console is first shown.
type consoleScreen struct {
	p  *term.Pane
	el js.Value
}

func (c *consoleScreen) Mount(el js.Value) error {
	useHostFS()
	c.el = el
	c.p = term.New(consoleGreeting, "chaosrack").Exec(rackShellCommand)
	return c.p.Mount(el)
}

// Shown lends the page back the rack's panel while the console is not in
// front, if `rack` here had laid it over its cells (rackoverlay_js.go).
func (c *consoleScreen) Shown(front bool) {
	if c.el.Truthy() {
		rackOverlay().Call("hold", c.el, !front)
	}
}

func (c *consoleScreen) Close() {
	if c.p != nil {
		c.p.Close()
	}
}
