//go:build js && wasm

package attractor

import (
	"github.com/0magnet/tuiwasm/deskapp"

	// Importing a demo registers it, and RegisterAll turns whatever is
	// registered into a desk app. Adding one to this list is the whole of
	// wiring it up.
	_ "github.com/0magnet/tuiwasm/demos/anim"
	_ "github.com/0magnet/tuiwasm/demos/charts"
	_ "github.com/0magnet/tuiwasm/demos/proxima"
	_ "github.com/0magnet/tuiwasm/demos/styles"
	_ "github.com/0magnet/tuiwasm/demos/tables"
	_ "github.com/0magnet/tuiwasm/demos/upstream/boxes"
	_ "github.com/0magnet/tuiwasm/demos/upstream/colors"
	_ "github.com/0magnet/tuiwasm/demos/upstream/unicode"
)

// The terminal demos, as desk apps.
//
// tuiwasm is a collection of Go terminal libraries running in a browser — a
// couple of dozen of them, twenty-one being animations — and each one is a
// function that draws into a terminal. desk.Pane is Mount(el) and Close(), with
// nothing about terminals in it, so tuiwasm's deskapp already knows how to make
// one out of a demo. Registering them here is the whole of the integration.
//
// WHY THEY COMPOSE AT ALL, since chaosrack and tuiwasm share no types: desk is
// the seam, and desk knows nothing about tcell. tuiwasm is on tcell v3 and this
// program has no tcell of its own, so there is nothing to reconcile — the demos
// arrive as panes, which is a DOM element and two methods.
//
// Only the demos are taken. tuiwasm's deskapp will also register a shell and a
// file manager, and those are already here in better versions: this desk's
// terminal is websh over the host filesystem when an agent is there to serve
// it, which the tuiwasm one has no way to be.
func registerDemoApps() { deskapp.RegisterAll() }
