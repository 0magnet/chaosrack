//go:build js && wasm

package attractor

import (
	"strconv"
	"sync"
	"syscall/js"

	"github.com/0magnet/desk/panes/term"
	"github.com/0magnet/seat"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/racktui"
)

// The page's screens (0magnet/seat), switched as a machine switches between
// its consoles and its desktop, with Ctrl+Alt and a digit:
//
//	1 instrument  a terminal at the bottom of the page running `chaosrack`:
//	              the model and its panel, laid over the terminal's cells at
//	              the page's own size, so it looks as the page always has
//	2 desktop     the desk over the model, the rack a window on it (what the
//	              Desk switch turns on, and still does)
//	3, 4          terminals, and nothing else
//
// The instrument is a program in a terminal because that is what it is: the
// model and the panel are one process, started by `chaosrack` and stopped
// with it (Ctrl+C), and `chaosrack --plain` draws it in the cells alone. Its
// terminal is the page's bottom layer, under everything else the page floats.

var pageSeat *seat.Seat

const instrumentGreeting = "" +
	"\x1b[1;35mchaosrack\x1b[0m — the instrument is this terminal's program\r\n" +
	"\x1b[2mctrl+c stops it, model and panel; \x1b[0m\x1b[1mchaosrack\x1b[0m\x1b[2m starts it again (--plain: in cells alone)\x1b[0m\r\n" +
	"\x1b[2mctrl+alt+1 instrument · ctrl+alt+2 desktop · ctrl+alt+3, 4 terminals\x1b[0m\r\n\r\n"

// useHostFS gives every terminal the server's filesystem, once, whichever
// asks first: the instrument, a terminal screen or the desk.
var useHostFS = sync.OnceFunc(term.UseHostFS)

// wireSeat makes the page's screens, the instrument in front.
func wireSeat() {
	s := seat.New(seat.Options{})
	inst := mountInstrument()
	s.AddPage("instrument", func(front bool) {
		if inst.Truthy() {
			vis := "hidden"
			if front {
				vis = ""
			}
			inst.Get("style").Set("visibility", vis)
			// The parts go back to the page while another screen is in front.
			rackOverlay().Call("hold", !front)
		}
	})
	s.AddPage("desktop", func(front bool) {
		desktopFront = front
		setDeskContain(front)
		presentRack()
		// The Desk switch shows which screen this is, however it was reached.
		if dc := dom.Doc.Call("getElementById", "desk-contain"); dc.Truthy() {
			dc.Set("checked", front)
		}
	})
	for n := 3; n <= 4; n++ {
		s.Add("terminal "+strconv.Itoa(n), &ttyScreen{n: n})
	}
	_ = s.Show("instrument") //nolint:errcheck // a page screen always shows
	pageSeat = s
}

// WHO PRESENTS THE PANEL. Without an instrument the page does, as it always
// has: docked to an edge or floating in its own window. With one, the panel is
// its program's, and nothing else puts it on the screen: `chaosrack` shows it
// in its terminal's window on the instrument, and as a window on the desk on
// the desktop, made when it is wanted and closed when it is not. While no
// program runs there is no panel anywhere — its shell waits in a store that is
// not a window and is not on the screen, as a stopped program's state is not.
var (
	instrumentOn      bool // the page has an instrument's terminal
	instrumentRunning bool // `chaosrack` is running in it
	desktopFront      bool
	rackStore         js.Value
	// instrumentPane is the instrument's terminal, the program's.
	instrumentPane *term.Pane
)

// presentRack puts the panel's shell where its presenter says it goes now.
func presentRack() {
	if !instrumentOn {
		return
	}
	shell := dom.Doc.Call("getElementById", "panel-shell")
	if !shell.Truthy() {
		return
	}
	if instrumentRunning && desktopFront {
		// The program's window on the desk. The frame itself is home in the
		// shell already: the instrument gave it back as the desktop came up.
		if shell.Get("parentNode").Equal(rackStore) {
			dom.Body.Call("appendChild", shell)
		}
		shell.Get("style").Set("display", "")
		layout.applyDock("float")
		trackRackInDeskPanel()
		rackHidesOnMinimize()
		return
	}
	// The instrument's window is the terminal's own (rackoverlay_js.go), and
	// with no program running there is none: the page's window is closed, not
	// hidden, and its shell stored.
	unfloatPanelWindow()
	if !rackStore.Truthy() {
		rackStore = dom.Doc.Call("createElement", "div")
		rackStore.Set("id", "rack-store")
		// Laid out, off the screen: the panel drawn in cells (--plain, or w)
		// is a picture of the frame measured where it is, and a frame that is
		// not laid out measures nothing. Not hidden either: the picture leaves
		// out what is hidden, as the page does not draw it.
		rackStore.Get("style").Set("cssText", "position:fixed;left:-300vw;top:0;pointer-events:none")
		dom.Body.Call("appendChild", rackStore)
	}
	rackStore.Call("appendChild", shell)
}

// instrumentProgram is `chaosrack` starting (true) and stopping in the
// instrument's terminal: the model and its panel with it.
func instrumentProgram(running bool) {
	instrumentRunning = running
	setPowerState(running)
	syncCategoryRotaries()
	presentRack()
}

// mountInstrument puts the instrument's terminal at the bottom of the page and
// starts `chaosrack` in it; zero where the page has no panel to run.
func mountInstrument() js.Value {
	if !dom.Doc.Call("getElementById", "panel-shell").Truthy() {
		return js.Value{}
	}
	useHostFS()
	el := dom.Doc.Call("createElement", "div")
	el.Set("id", "instrument")
	// Over the model's own container, whose canvas it takes into its cells,
	// and under the page's windows and switches.
	el.Get("style").Set("cssText", "position:fixed;inset:0;z-index:3;background:#000")
	if c := dom.Doc.Call("getElementById", "gocanvas-container"); c.Truthy() {
		c.Call("after", el)
	} else {
		dom.Body.Call("appendChild", el)
	}
	p := term.New(instrumentGreeting, "chaosrack").Exec(instrumentCommand).Run("chaosrack")
	if err := p.Mount(el); err != nil {
		el.Call("remove")
		js.Global().Get("console").Call("error", "chaosrack: the instrument's terminal: "+err.Error())
		return js.Value{}
	}
	instrumentPane = p
	instrumentOn = true
	presentRack()
	// The page's ▤ button shows and hides the panel's window, which is the
	// program's now: it works that window while the program runs, and with
	// none running there is nothing for it to show.
	if t := dom.Doc.Call("getElementById", "panel-toggle"); t.Truthy() {
		t.Call("addEventListener", "click", js.FuncOf(func(_ js.Value, a []js.Value) any {
			if desktopFront {
				return nil // the desk's window, the page's own button as ever
			}
			a[0].Call("stopImmediatePropagation")
			a[0].Call("preventDefault")
			if instrumentRunning && windowHand != nil {
				windowHand(racktui.WindowAct{Kind: "hide"})
			}
			return nil
		}), map[string]any{"capture": true})
	}
	return el
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

// ttyScreen is a terminal filling the screen, made when it is first shown.
type ttyScreen struct {
	n int
	p *term.Pane
}

func (t *ttyScreen) Mount(el js.Value) error {
	useHostFS()
	t.p = term.New("\x1b[2mterminal "+strconv.Itoa(t.n)+" · ctrl+alt+1 for the instrument\x1b[0m\r\n\r\n", "chaosrack")
	return t.p.Mount(el)
}

func (t *ttyScreen) Close() {
	if t.p != nil {
		t.p.Close()
	}
}
