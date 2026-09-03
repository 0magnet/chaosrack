//go:build js && wasm

// Package deskapp puts the demos into desk, one to a window.
//
// Each window gets its own xterm-go terminal, so several demos are on screen
// at once rather than one to a page. The text demos have no limit worth
// mentioning — they write to their own terminal and nothing is shared.
//
// The tcell demos take turns. tcell's wasm screen reaches the page through
// global function names and installs its own onKeyEvent among them, so only
// one can hold the keyboard; see xtcell. A window claims the globals when it
// is clicked, which suspends whichever screen had them. So several tcell
// windows can be open, and the one you are looking at is the one that runs.
//
// The work of getting a demo onto a terminal is not here — it is in play,
// which does the same for anything embedding these demos rather than only for
// a desk window. This file is the adapter between that and desk's Pane.
package deskapp

import (
	"syscall/js"

	"github.com/0magnet/desk"

	"github.com/0magnet/tuiwasm/demos"
	"github.com/0magnet/tuiwasm/play"
)

// RegisterAll turns every registered demo into a desk app.
func RegisterAll() {
	for _, d := range demos.All() {
		d := d
		desk.Register(desk.App{
			Name:   d.Name,
			Title:  d.Name,
			Help:   d.Desc,
			Width:  760,
			Height: 460,
			Open: func([]string) (desk.Pane, error) {
				return &demoPane{demo: d}, nil
			},
		})
	}
}

// demoPane is a demo in a desk window.
//
// Both shapes of demo are the same pane now: play works out which one it has
// and what that needs. The window only has to say where it wants it and let go
// of it afterwards.
type demoPane struct {
	demo    demos.Demo
	session *play.Session
}

func (p *demoPane) Mount(el js.Value) error {
	s, err := play.Mount(p.demo, el)
	if err != nil {
		return err
	}
	p.session = s
	return nil
}

func (p *demoPane) Close() {
	p.session.Close()
	p.session = nil
}
