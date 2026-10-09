// Package rackpic is the rack's panel as a picture, for a front end that is
// not a browser.
//
// The terminal panel used to lay the rack out itself, from module widths and
// control counts, and draw its own idea of each control. However good that
// idea got, it was a second drawing of the panel, and every part the page
// grew — a lamp, a tick ring, a dial's printed positions — was a part the
// terminal had to be taught again. This takes the other road: the page says
// what is ON the panel, element by element, as boxes, discs, text and
// pointers in the panel's own pixels, and a front end paints that at a scale
// of its own. The two cannot disagree about where anything is, because only
// one of them decides.
//
// The page's half is collect.js, evaluated in the page by whichever Source
// is attached to it (in the page itself, or over a cable or a link). It reads
// the whole panel once, which is costly (about 0.3 s and 400 kB for the full
// rack), and after that only what a MutationObserver saw change, which is
// cheap enough to ask for four times a second.
package rackpic

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"strings"
)

// Script defines window.__rackpic in a page. Evaluating it again is harmless;
// evaluating a newer one replaces the older.
var Script = strings.Replace(script, "__RACKPIC_VERSION__", version(script), 1)

//go:embed collect.js
var script string

func version(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

// Calls into the script, as expressions evaluated in the page. Each returns
// a JSON string.
const (
	CallPicture = "window.__rackpic.picture()"
	callChanges = "window.__rackpic.changes(%d)"
	callAct     = "window.__rackpic.act(%d, %g, %g, %q, %g)"
)

// ChangesCall is the expression asking what changed since gen.
func ChangesCall(gen int) string { return fmt.Sprintf(callChanges, gen) }

// ActCall is the expression doing kind ("click" or "wheel") at x, y of the
// picture taken at gen; delta is the wheel's, in pixels, positive down.
func ActCall(gen int, x, y float64, kind string, delta float64) string {
	return fmt.Sprintf(callAct, gen, x, y, kind, delta)
}

// Item is what one element of the panel contributes: a box, filled or
// outlined, square or round; text; or a knob's pointer. Coordinates are the
// panel's own pixels at an interface scale of 1, from its top-left corner.
type Item struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
	// Fill is the background, 0xRRGGBB, absent for none.
	Fill *int `json:"b,omitempty"`
	// Border is a border of a color other than the fill.
	Border *int `json:"o,omitempty"`
	// Round is a box with fully rounded corners: a disc.
	Round int `json:"r,omitempty"`
	// Text, in Ink, aligned "" (left), "r" or "c"; Vertical stacks it one
	// letter under another, as the P-unit's legend is printed.
	Text     string `json:"t,omitempty"`
	Ink      *int   `json:"f,omitempty"`
	Align    string `json:"al,omitempty"`
	Vertical int    `json:"v,omitempty"`
	// Ticks are marks round a dial: [period, width, color] each, a mark
	// every period degrees clockwise from straight up, running in from the
	// rim to Inner of the radius.
	Ticks [][3]float64 `json:"k,omitempty"`
	Inner float64      `json:"ki,omitempty"`
	// Angle marks a knob's pointer, in degrees clockwise from straight up.
	// The box is the knob's; Fill is the pointer's color, not a fill.
	Angle *float64 `json:"a,omitempty"`
}

// Pointer reports whether the item is a knob's pointer.
func (it *Item) Pointer() bool { return it != nil && it.Angle != nil }

// Picture is the whole panel at one generation.
type Picture struct {
	Gen int     `json:"gen"`
	W   float64 `json:"w"`
	H   float64 `json:"h"`
	// Items in the page's order, which is the order to paint them in. A nil
	// item is an element that shows nothing of its own.
	Items []*Item `json:"items"`
	// Ctls is where each control's part is: x, y, w, h.
	Ctls map[string][4]float64 `json:"ctls"`
	Err  string                `json:"err,omitempty"`
}

// Patch is what changed since a picture was taken: items by index (nil for
// one that is gone), or, when the panel moved under it, a new picture.
type Patch struct {
	Gen   int           `json:"gen"`
	Items map[int]*Item `json:"items,omitempty"`
	Full  *Picture      `json:"full,omitempty"`
	Err   string        `json:"err,omitempty"`
}

// ParsePicture reads what CallPicture returned.
func ParsePicture(s string) (*Picture, error) {
	var p Picture
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return nil, err
	}
	if p.Err != "" {
		return nil, errors.New(p.Err)
	}
	return &p, nil
}

// ParsePatch reads what ChangesCall returned.
func ParsePatch(s string) (*Patch, error) {
	var p Patch
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return nil, err
	}
	if p.Err != "" {
		return nil, errors.New(p.Err)
	}
	return &p, nil
}

// ParseAct reads what ActCall returned.
func ParseAct(s string) error {
	var r struct {
		Err string `json:"err"`
	}
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return err
	}
	if r.Err != "" {
		return errors.New(r.Err)
	}
	return nil
}

// Apply brings p up to date with a patch, and reports whether anything
// changed. A patch carrying a whole picture replaces p's contents.
func (p *Picture) Apply(d *Patch) bool {
	if d == nil {
		return false
	}
	if d.Full != nil {
		*p = *d.Full
		return true
	}
	for i, it := range d.Items {
		if i < 0 {
			continue
		}
		for i >= len(p.Items) {
			p.Items = append(p.Items, nil)
		}
		p.Items[i] = it
	}
	return len(d.Items) > 0
}

// RGB is a 0xRRGGBB value as a color.
func RGB(v int) color.RGBA {
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255} //nolint:gosec // the bytes of a 24-bit color, each masked by the conversion
}
