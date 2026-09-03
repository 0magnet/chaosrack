// Package demos is the registry the showcase reads.
//
// A demo declares which of the two shapes it is and nothing else. It does not
// know whether it is running in a browser, in websh, or in a real terminal —
// one takes an io.Writer, the other a tcell.Screen, and both of those exist
// everywhere. That is what lets the same demo be a page today and a websh
// applet later without being rewritten.
package demos

import (
	"io"
	"sort"

	"github.com/gdamore/tcell/v3"
)

// Demo is one runnable example. Exactly one of Text or Screen is set.
type Demo struct {
	Name string
	Desc string

	// Mirror reports whether a glyph should be drawn flipped left-to-right.
	// nil, which is nearly always right, draws everything the way round the
	// font has it.
	//
	// It is here for one demo. The Matrix's code rain uses mirrored katakana,
	// and Unicode encodes none — so the demo asks for ordinary kana and asks
	// the terminal to draw them the other way round. A terminal that cannot,
	// which is every real one, shows them unflipped instead of showing nothing.
	Mirror func(string) bool

	// Text is a demo that writes styled text. Most libraries are this shape:
	// lipgloss, glamour, chroma, go-pretty, asciigraph, the progress bars.
	Text func(w io.Writer, cols, rows int) error

	// Screen is a demo that paints cells and reads keys. tcell itself, and
	// everything built on it — tview, termdash.
	//
	// cols and rows are the terminal's real size. They are passed rather than
	// left to s.Size() because tcell's web screen hardcodes 80x24 in Init,
	// and anything that calls Init again — tview's SetScreen does — puts that
	// back. A demo that re-initializes the screen should SetSize afterwards.
	Screen func(s tcell.Screen, cols, rows int) error
}

var registry = map[string]Demo{}

// Register adds a demo. Demos call this from an init, so importing the
// package is enough to make one available and the showcase needs no list of
// its own.
func Register(d Demo) { registry[d.Name] = d }

// Lookup finds a demo by name.
func Lookup(name string) (Demo, bool) {
	d, ok := registry[name]
	return d, ok
}

// All returns every registered demo, ordered by name so the menu is stable.
func All() []Demo {
	out := make([]Demo, 0, len(registry))
	for _, d := range registry {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
