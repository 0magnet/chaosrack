package attractor

import (
	"slices"
	"strconv"
	"strings"

	"github.com/0magnet/chaosrack/pkg/colormap"
	"github.com/0magnet/chaosrack/pkg/colorspace"
)

// The backdrop's own colors (backlayer_js.go applies them).
//
// A backdrop is a flat picture filling the canvas behind the model: the
// spectrogram, the xy scope, a terminal, an animation, the desk. Flat, and
// face-on, so turning it would only show its edge: the one thing about it
// worth setting apart from the model in front is its COLOR. It gets a palette
// of its own, and the rack does not get a second Colors module for it: the
// Back switch points the one it has at the backdrop, as the view focus
// points it at one view of a grid.

// flatBackdrops are the BEHIND positions that are pictures the palette can
// color, each also a model of the same name — so while Back is on, the MODEL
// knob and the bank show the backdrop's own controls. Water is not one: it is
// a lens drawn through the finished frame, not a picture behind it.
var flatBackdrops = []string{"spectrogram", "xy", "terminal", "termanim", "desk"}

// flatBackdrop reports whether kind is a backdrop with colors of its own.
func flatBackdrop(kind string) bool { return slices.Contains(flatBackdrops, kind) }

// backdropRecolors reports whether a backdrop with source src is drawn through
// its palette. The spectrogram always is: its picture IS a magnitude through a
// map. The others have colors of their own — a terminal's, a window's, the
// scope's phosphor — which source OFF leaves alone and any other source
// replaces with the palette, by brightness.
func backdropRecolors(kind string, src int) bool {
	return kind == "spectrogram" || src != GradientSourceOff
}

// layerPalette is everything the Colors module sets that a layer can have
// its own of: what the color follows, the map, its three swatches and the
// window across it.
type layerPalette struct {
	src, cols      int
	base, mid, top [3]float32
	freq, shift    float32
}

// defaultBackPalette is a backdrop's palette until it is set: its own colors
// (source off), and a map that reads as a magnitude — the spectrogram's heat —
// from black, so a recolored terminal or desk keeps its dark ground.
var defaultBackPalette = layerPalette{
	src: GradientSourceOff, cols: colormap.First,
	base: [3]float32{0, 0, 0}, mid: [3]float32{0, 1, 0}, top: [3]float32{0.2, 1, 0.4},
	freq: 1,
}

// String is the palette as a link carries it: seven fields joined by "_".
func (p layerPalette) String() string {
	hex := func(c [3]float32) string { return strings.TrimPrefix(colorspace.Hex(c), "#") }
	f := func(x float32) string { return strconv.FormatFloat(float64(x), 'g', 4, 32) }
	return strings.Join([]string{strconv.Itoa(p.src), strconv.Itoa(p.cols),
		hex(p.base), hex(p.mid), hex(p.top), f(p.freq), f(p.shift)}, "_")
}

// parseLayerPalette reads a palette back from its String; ok is false for
// anything that is not one.
func parseLayerPalette(s string) (layerPalette, bool) {
	fs := strings.Split(s, "_")
	if len(fs) != 7 {
		return layerPalette{}, false
	}
	var p layerPalette
	var err error
	if p.src, err = strconv.Atoi(fs[0]); err != nil {
		return layerPalette{}, false
	}
	if p.cols, err = strconv.Atoi(fs[1]); err != nil {
		return layerPalette{}, false
	}
	for i, c := range []*[3]float32{&p.base, &p.mid, &p.top} {
		if len(fs[2+i]) != 6 {
			return layerPalette{}, false
		}
		*c = colorspace.ParseHex("#" + fs[2+i])
	}
	for i, x := range []*float32{&p.freq, &p.shift} {
		v, err := strconv.ParseFloat(fs[5+i], 32)
		if err != nil {
			return layerPalette{}, false
		}
		*x = float32(v)
	}
	return p, true
}
