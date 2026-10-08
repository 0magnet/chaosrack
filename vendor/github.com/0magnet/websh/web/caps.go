//go:build js && wasm

package web

import (
	"runtime/debug"
	"syscall/js"

	"github.com/0magnet/websh/progressive"
)

// caps is this session's answer to the caps query (PROTOCOL.md, Discovery):
// what it offers the program whose output is on the terminal now, by how far
// it trusts that output, and the cell metrics a picture needs to fit cells.
func (s *Session) caps(p *placements) *progressive.Caps {
	c := &progressive.Caps{V: 1, Host: "websh", Version: version(), Trust: s.Shell.Source()}
	c.Cols, c.Rows = s.Term.Core.Cols(), s.Term.Core.Rows()
	if screen := p.root.Call("querySelector", ".xterm-screen"); screen.Truthy() && c.Cols > 0 && c.Rows > 0 {
		c.Cell.W = screen.Get("clientWidth").Float() / float64(c.Cols)
		c.Cell.H = screen.Get("clientHeight").Float() / float64(c.Rows)
	}
	c.DPR = js.Global().Get("devicePixelRatio").Float()
	c.Features = []string{"place", "place.input", "event", "post", "ship", "download", "clipboard", "mirror"}
	if c.Trust != "remote" {
		c.Features = append(c.Features, "widget.offer", "font", "title", "page")
	}
	c.Path = s.linkPath()
	return c
}

// version is websh's module version in this binary: the main module's
// when websh is the program, the dependency's when it is part of another.
func version() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	if bi.Main.Path == "github.com/0magnet/websh" {
		return bi.Main.Version
	}
	for _, d := range bi.Deps {
		if d.Path == "github.com/0magnet/websh" {
			return d.Version
		}
	}
	return ""
}
