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

	"github.com/0magnet/chaosrack/pkg/attractor"
)

// Control is one control as a panel needs it: what it IS, from the registry,
// plus what it currently SAYS, which only the running rack knows.
type Control struct {
	attractor.ControlInfo
	Value   string
	Options []string // a selector's detents, in order; nil for a dial
}

// Source is where the panel reads the rack and where its changes go.
//
// Three methods and no notion of transport, so the terminal panel does not
// care whether the rack is in this process, in a browser on this machine, or
// on the other end of a cable.
type Source interface {
	// Rack is the bays, already drawn. Text because the drawing is the
	// rack's own (rackascii.go) and a front end that redrew it would be a
	// second opinion about the layout.
	Rack() (string, error)
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
