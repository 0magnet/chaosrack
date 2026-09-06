//go:build js && wasm

package attractor

import (
	"context"
	"fmt"
	"strings"
	"syscall/js"
	"time"

	"github.com/0magnet/desk"
	"github.com/0magnet/desk/panes/files"
	"github.com/0magnet/desk/panes/hostterm"
	"github.com/0magnet/desk/panes/term"
	"github.com/0magnet/sh/v3/interp"
	"github.com/0magnet/websh/shell"
	"github.com/0magnet/websh/web"
	xterm "github.com/0magnet/xterm-go"
)

// hostAppletTerminal finds the terminal an applet is running in. websh
// publishes the shell-to-session pairing for exactly this, so the terminal is
// reachable without keeping a private note of which pane is "the" shell —
// which is wrong the moment two terminals are open.
func hostAppletTerminal(s *shell.Shell) *xterm.Terminal { return web.TerminalFor(s) }

// A window manager over the scene.
//
// The Terminal model puts a shell ON a quad, inside the 3-D scene, where it
// rotates. This is the other arrangement, and they answer different wishes:
// here the scene stays a scene and the desk floats over it, so the attractor
// being integrated is the DESKTOP — not a wallpaper image but a system running
// underneath the windows, still driven by every knob on the rack.
//
// WHY NOT COMPOSITE THE DESK INTO THE SCENE, which was the original idea —
// desk as one texture on a rotatable plane. Because a window is not only its
// pane. The title, the buttons and the border are DOM, DOM cannot be sampled
// into a texture, and a desk rendered as one texture would be a desk whose
// windows have no chrome. desk's own WebGL compositor ran into exactly this and
// now composites pane content only, leaving the frames on the DOM. So the whole
// desk can be drawn over the scene, or its panes can be textures, but not both
// at once — and over the scene is the one that stays usable.
//
// THE STACKING CONTEXT IS THE LOAD-BEARING DETAIL. winbox numbers its windows
// from about 10 upward, and this app's own scale puts the control panel at 10
// and the status toast at 50; left alone, a raised window would climb over the
// rack. Giving the desk's root its own z-index makes it a stacking context, so
// every window's z is resolved INSIDE it and the whole desk sits at one layer:
// above the canvas, below the panel. Nothing had to be renumbered.
//
// Pointer events are off on the root and on again for each window, so the empty
// desktop is not a sheet of glass over the canvas — dragging where there is no
// window still rotates the model, which is the whole point of having a scene
// back there.

const deskRootID = "chaosrack-desk"

var (
	deskEl    js.Value
	deskBuilt bool
)

// deskGreeting is written into the first terminal.
const deskGreeting = "" +
	"\x1b[1;35mchaosrack\x1b[0m — a desk over the scene\r\n" +
	"\x1b[2mthe model behind these windows is still running, and still yours to tune\x1b[0m\r\n\r\n" +
	"  \x1b[1mapps\x1b[0m — what can be opened   ·   \x1b[1mopen files\x1b[0m   ·   \x1b[1mterm\x1b[0m\r\n\r\n"

// setDeskOn shows or hides the desk.
func setDeskOn(on bool) {
	if !on {
		if deskEl.Truthy() {
			deskEl.Get("style").Set("display", "none")
		}
		// Stop the per-frame loop as well as hiding the windows. A hidden
		// desktop still ticking is a BumpTop pile being simulated, and a cube
		// being dimmed, sixty times a second behind a display:none.
		setDeskTicking(false)
		return
	}
	ensureDesk()
	if deskEl.Truthy() {
		deskEl.Get("style").Set("display", "")
	}
	// Reapply whatever style is selected. This is not belt and braces: the
	// selector can be turned before the desk is switched on, and setDeskStyle
	// gives up early when there is no desk yet, so without this the chosen
	// desktop stays inert until the selector is touched a second time.
	setDeskStyle(deskStyle)
}

func ensureDesk() {
	if deskBuilt {
		return
	}
	deskBuilt = true

	deskEl = doc.Call("createElement", "div")
	deskEl.Set("id", deskRootID)
	// z-index 4 is the CRT overlay's layer: above the canvas at 3, below the
	// control panel at 10. Being NUMBERED at all is what matters — that is
	// what makes this a stacking context and keeps winbox's numbers inside.
	deskEl.Set("style", "position:fixed;inset:0;z-index:4;pointer-events:none;")
	doc.Get("body").Call("appendChild", deskEl)

	style := doc.Call("createElement", "style")
	// Windows take the mouse; the empty desktop does not. And when the desk is
	// being drawn as a MODEL the windows must not either: they are hidden at
	// opacity 0, and an element at opacity 0 still hit-tests, so an invisible
	// window would go on swallowing drags over wherever it used to be — which
	// is a drag meant to rotate the model it is now part of.
	// The panel and its menu take the mouse too. They are not .winbox, so the
	// window rule does not reach them, and a panel inside a pointer-events:none
	// root is a panel nothing can click — which is how the Applications menu
	// came to be unopenable the first time it was shown.
	style.Set("textContent", "#"+deskRootID+" .winbox,"+
		"#"+deskRootID+" .dk-panel,#"+deskRootID+" .dk-menu{pointer-events:auto;}"+
		"#"+deskRootID+".as-model .winbox,"+
		"#"+deskRootID+".as-model .dk-panel{pointer-events:none;}")
	doc.Get("head").Call("appendChild", style)

	desk.SetRoot(deskEl)

	// THE FILESYSTEM, IF THE SERVER IS SERVING ONE. The shell and the file
	// manager both work against an afero.Fs, so substituting one host-backed
	// implementation makes both of them real at once — `ls`, `cat`, `grep`,
	// globbing and redirection all land on the machine, while the interpreter
	// stays in the tab. Absent an agent this does nothing and websh keeps its
	// in-memory filesystem, which is what every static host will do.
	//
	// And if the page was served with --auth there is no token yet, so this
	// also arranges to swap memory for the machine the moment one is typed
	// into a terminal — see desk's panes/term. /bin stays synthetic either way.
	term.UseHostFS()

	desk.Register(desk.App{
		Name:   "term",
		Title:  "terminal",
		Help:   "a shell",
		Width:  720,
		Height: 420,
		Open: func([]string) (desk.Pane, error) {
			return term.New(deskGreeting, "chaosrack"), nil
		},
	})
	// Registered whether or not an agent is there to talk to. Hiding it when
	// the page was served without --shell would mean the only way to find out
	// it exists is to already know; opening it and being told which flag turns
	// it on is the discoverable version.
	desk.Register(desk.App{
		Name:   "host",
		Title:  "host shell",
		Help:   "a real shell on this machine (needs --shell; `host NAME` to reconnect)",
		Width:  760,
		Height: 460,
		Open: func(args []string) (desk.Pane, error) {
			// An argument NAMES the session, which is what makes it survive
			// the window: `host build` twice is the same shell the second
			// time, with what it printed in between replayed into it.
			//
			// Naming is the user’s job rather than the pane’s because only
			// they know whether a new window is meant to be the old one: an
			// id the pane invented would either give every host window the
			// same shell, or give a reopened window a different one.
			//
			// Needs --reconnect on the other end. Without it the name is
			// ignored and this is an ordinary host shell, so there is
			// nothing to check for here.
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return hostterm.NewSession(name), nil
		},
	})
	desk.Register(desk.App{
		Name:   "files",
		Title:  "files",
		Help:   "browse the filesystem",
		Width:  520,
		Height: 380,
		Open: func(args []string) (desk.Pane, error) {
			dir := ""
			if len(args) > 0 {
				dir = args[0]
			}
			return files.New(term.FS(), dir), nil
		},
	})
	// The rack itself, in the launcher beside the desk's own apps.
	//
	// Run rather than Open, because this window is not the desk's to build: the
	// control panel lives in a window chaosrack made, with the dock cluster in
	// it that takes the panel back to a screen edge. Under Contain the close
	// button now really closes it, and this is how it comes back — which is the
	// bargain that makes closing safe to mean closing.
	desk.Register(desk.App{
		Name:  "chaosrack",
		Title: "chaosrack controls",
		Help:  "the rack's control panel",
		Run: func([]string) error {
			relaunchRack()
			return nil
		},
	})
	registerDemoApps()
	registerDeskApplets()
	wireDeskGestures()
	wireDeskPassthrough()
	makeDeskPanel()

	if _, err := desk.Launch("term"); err != nil {
		js.Global().Get("console").Call("warn", "chaosrack: desk: "+err.Error())
	}
}

// makeDeskPanel creates the desk's panel — Applications menu, task buttons,
// clock — and then makes it invisible on the page.
//
// IT IS CREATED BEFORE THE FIRST WINDOW, because the panel tracks the windows
// it is told about and is told about them as they open. Created later, as it
// was at first, it comes up with no task buttons and never learns about the
// windows that were already there.
//
// visibility rather than display, and this is the whole trick: a hidden panel
// is still LAID OUT, so it still has a rectangle, and the compositor can read
// that rectangle and draw a copy of the panel into the canvas when the desk is
// a model. display:none would collapse it and the drawn copy would have
// nowhere to go.
//
// Invisible on the page in both arrangements, for different reasons. With the
// Desk SWITCH, desk's panel is a docked bar along the bottom and so is the
// rack — they would fight for the same edge, and the rack wins because it is
// the reason the page exists; the shell is the launcher there, which is what
// `apps` is for. As a MODEL there is no edge to fight over, so the panel is
// drawn into the scene where it belongs.
func makeDeskPanel() {
	if deskPanel != nil {
		return
	}
	deskPanel = desk.NewPanel()
	for _, sel := range []string{".dk-panel", ".dk-menu"} {
		if el := deskEl.Call("querySelector", sel); el.Truthy() {
			el.Get("style").Set("visibility", "hidden")
		}
	}
}

var deskPanel *desk.Panel

// registerDeskApplets lets the shell open windows.
func registerDeskApplets() {
	shell.RegisterApplet("open", "open a desk app in a window (open -l to list)",
		func(_ context.Context, _ *shell.Shell, hc *interp.HandlerContext, args []string) int {
			if len(args) == 0 || args[0] == "-l" || args[0] == "--list" {
				fmt.Fprint(hc.Stdout, deskAppList()) //nolint:errcheck // a closed pipe is the shell's business, not ours
				return 0
			}
			if _, err := desk.Launch(args[0], args[1:]...); err != nil {
				fmt.Fprintln(hc.Stderr, "open:", err) //nolint:errcheck // as above
				return 1
			}
			return 0
		})
	shell.RegisterApplet("host", "a shell on this machine, in this terminal (host NAME to reconnect)",
		func(ctx context.Context, s *shell.Shell, hc *interp.HandlerContext, args []string) int {
			term := hostAppletTerminal(s)
			if term == nil {
				fmt.Fprintln(hc.Stderr, "host: no terminal to attach to") //nolint:errcheck
				return 1
			}
			name := ""
			if len(args) > 0 {
				name = args[0]
			}

			// Raw mode BEFORE attaching: the shell otherwise line-buffers and
			// echoes, so the remote pty would receive whole lines late and the
			// terminal would show every keystroke twice.
			if s.RawMode != nil {
				s.RawMode(true)
				defer s.RawMode(false)
			}

			// A newline BEFORE the remote's first byte. Raw mode is already on,
			// so websh did not echo the Return that ran this — the cursor is
			// still sitting after "host demo" and the remote's first output
			// would start on that line, on top of the command that asked for
			// it. A shell echoes the newline for exactly this reason.
			fmt.Fprint(hc.Stdout, "\r\n") //nolint:errcheck

			att, err := hostterm.Attach(term, name)
			if err != nil {
				fmt.Fprintf(hc.Stderr, "host: %v\n", err) //nolint:errcheck
				return 1
			}
			defer att.Close()

			// Pump raw keystrokes to the pty. websh hands them to this
			// applet's stdin while raw mode is on, Ctrl+C included, which is
			// what lets the remote program see an interrupt instead of this
			// one being killed by it.
			go func() {
				buf := make([]byte, 1024)
				for {
					n, err := hc.Stdin.Read(buf)
					if n > 0 {
						att.Send(buf[:n])
					}
					if err != nil {
						return
					}
				}
			}()

			// Either the remote shell exited, or the session was canceled
			// from outside. Both mean the same thing here: give the terminal
			// back.
			select {
			case <-att.Done():
				// Let the last of the pty's output land before taking the
				// terminal back. The socket's close and the messages ahead of
				// it are separate JS tasks, so returning the instant Done
				// fires means websh draws its prompt and the remote's parting
				// "exit" is then written over it — which is what the two
				// shells fighting for one cursor looks like.
				time.Sleep(120 * time.Millisecond)
			case <-ctx.Done():
			}

			// A remote full-screen program may have left the cursor hidden or
			// a scroll region set, and the prompt about to be printed would
			// inherit both. Reset rather than clear: clearing would throw away
			// the session the user just had, which is the thing worth keeping.
			// Hand the terminal back in a known state. A remote full-screen
			// program may have left the cursor hidden or a scroll region set,
			// and the prompt about to be printed would inherit both. The
			// trailing newline is what puts that prompt on a line of its own
			// rather than at whatever column the remote stopped in.
			fmt.Fprint(hc.Stdout, "\x1b[?25h\x1b[r\x1b[0m\r\n") //nolint:errcheck
			return 0
		})
	shell.RegisterApplet("term", "open another terminal window",
		func(_ context.Context, _ *shell.Shell, hc *interp.HandlerContext, args []string) int {
			if _, err := desk.Launch("term", args...); err != nil {
				fmt.Fprintln(hc.Stderr, "term:", err) //nolint:errcheck // as above
				return 1
			}
			return 0
		})
	shell.RegisterApplet("apps", "list the windows this desk can open",
		func(_ context.Context, _ *shell.Shell, hc *interp.HandlerContext, _ []string) int {
			fmt.Fprint(hc.Stdout, deskAppList()) //nolint:errcheck // a closed pipe is the shell's business, not ours
			return 0
		})
}

func deskAppList() string {
	var b strings.Builder
	for _, a := range desk.Apps() {
		fmt.Fprintf(&b, "  %-10s %s\n", a.Name, a.Help)
	}
	return b.String()
}
