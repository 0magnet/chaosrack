// Package racktui is the rack's control surface, in a terminal.
//
// It is not a view OF the panel and it does not know the panel exists. It
// renders what the rack says about itself — the bays that packBySection packs
// (drawn by rackascii.go) and the controls that adoptDescControl records
// (ControlRegistry) — and hands changes back the same way a knob does. The DOM
// panel is one front end over that model and this is another; neither is the
// original.
//
// That is the whole reason the model was untagged one piece at a time:
// racksection.go and rackunit.go have always been pure, pkg/rackspec has
// always been its own package, and controldesc.go became pure when the
// builders were split out of it. What is left in //go:build js is drawing, and
// drawing is what a front end IS.
//
// Where the rack lives is a Source's business. Today the only one is a cable
// to a running page (cmd/uitool), because the instrument is wasm and its
// controls are wired by code that needs a browser. A Source backed by the rack
// itself — the same tcell panel rendered inside the page, the way
// magnetosphere's store runs both natively and on the desk — needs nothing
// from this file.
package racktui

import (
	"context"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/chaosrack/pkg/controlspec"
	"github.com/0magnet/chaosrack/pkg/racksurface"
)

// Control is one control as a panel needs it: what it IS, from the registry,
// plus what it currently SAYS, which only the running rack knows.
type Control struct {
	controlspec.ControlInfo
	Value   string
	Options []string // a selector's detents, in order; nil for a dial
}

// Source is where the panel reads the rack and where its changes go.
//
// Three methods and no notion of transport, so the terminal panel does not
// care whether the rack is in this process, in a browser on this machine, or
// on the other end of a cable.
type Source interface {
	// Modules is what the rack holds: every module with its width in slots
	// and the bay it belongs to, already grouped into sections, plus how many
	// slots a row has.
	//
	// It used to be the bays ALREADY DRAWN, as text, on the reasoning that a
	// front end which redrew them would be a second opinion about the layout.
	// That was right about the danger and wrong about the cure: a drawing is
	// not something a panel can put controls ON, so the panel laid the modules
	// out again by wrapping them to the terminal's width — and THAT was the
	// second opinion, and it did not have bays in it at all.
	//
	// So what crosses now is the measurement, and the layout happens once, in
	// pkg/racksurface, for whoever is drawing. What a Source supplies is only
	// what has to be measured: a module's width is decided by its contents at
	// the interface scale in use, and only a laid-out panel knows that.
	//
	// racksurface.Item and not a type of this package's own, because the
	// sections and their order are the RACK's — a front end that decided which
	// bay a module belonged to would be inventing the layout rather than
	// drawing it. Rows is left zero here and filled in by the renderer, which
	// is the one dimension it does get to decide. See layout.
	Modules() (mods []racksurface.Item, slotsPerRow int, err error)
	// Controls is the whole surface with its current values.
	Controls() ([]Control, error)
	// Set moves one control, as a hand would.
	Set(id, value string) error
}

type screenKey struct{}

// WithScreen names the screen Run should use, for a caller where
// tcell.NewScreen is the wrong answer — a browser, where the screen has to be
// bound to the terminal this was typed into.
func WithScreen(ctx context.Context, newScreen func() (tcell.Screen, error)) context.Context {
	return context.WithValue(ctx, screenKey{}, newScreen)
}

func screenFrom(ctx context.Context) func() (tcell.Screen, error) {
	if ctx == nil {
		return nil
	}
	fn, _ := ctx.Value(screenKey{}).(func() (tcell.Screen, error))
	return fn
}

// Run drives the rack from a terminal until the user quits, on the screen ctx
// names or on a new tcell one.
func Run(ctx context.Context, src Source) error {
	if newScreen := screenFrom(ctx); newScreen != nil {
		sc, err := newScreen()
		if err != nil {
			return err
		}
		return RunOn(sc, src)
	}
	sc, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	return RunOn(sc, src)
}

// CellShaper is a Source that knows the shape of the terminal's character
// cell, which the dials need in order to come out round rather than as
// vertical ellipses.
//
// Optional, and asked for rather than required, because most Sources cannot
// answer it. A terminal does not report its font: there is no escape sequence
// and no termios field for "how many pixels is a cell", so a panel on a host
// terminal has to take the 1:2 that console fonts usually are. A panel running
// INSIDE the page is the exception — the terminal it is drawing on is an
// element it can measure — and that is the case this exists for.
type CellShaper interface {
	// CellAspect is cell height divided by cell width. Zero or less means
	// "not known", and the default stands.
	CellAspect() float64
}
