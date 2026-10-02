package attractor

import "testing"

func TestOnlyFlatPicturesHaveBackdropColors(t *testing.T) {
	for _, k := range []string{"spectrogram", "xy", "terminal", "termanim", "desk"} {
		if !flatBackdrop(k) {
			t.Errorf("%s has no colors of its own", k)
		}
	}
	for _, k := range []string{"", "water", "model", "lorenz", "globe"} {
		if flatBackdrop(k) {
			t.Errorf("%s has backdrop colors", k)
		}
	}
}

// Source OFF leaves a picture's own colors alone, except the spectrogram's,
// which are a map by nature.
func TestSourceOffKeepsABackdropsOwnColors(t *testing.T) {
	if backdropRecolors("terminal", GradientSourceOff) || backdropRecolors("desk", GradientSourceOff) {
		t.Error("source off recolored a picture")
	}
	if !backdropRecolors("terminal", 2) {
		t.Error("a source did not recolor the terminal")
	}
	if !backdropRecolors("spectrogram", GradientSourceOff) {
		t.Error("the spectrogram lost its map")
	}
}

// A backdrop's palette survives a link, and a mangled one is refused.
func TestABackdropPaletteRoundTripsThroughALink(t *testing.T) {
	p := defaultBackPalette
	p.src, p.cols, p.freq, p.shift = 2, 7, 2.5, -0.25
	p.mid = [3]float32{1, 0, 1}
	got, ok := parseLayerPalette(p.String())
	if !ok || got != p {
		t.Fatalf("%q came back as %+v, %v", p.String(), got, ok)
	}
	for _, bad := range []string{"", "5_5", "5_5_000000_00ff00_33ff66_1", "x_5_000000_00ff00_33ff66_1_0", "5_5_000_00ff00_33ff66_1_0"} {
		if _, ok := parseLayerPalette(bad); ok {
			t.Errorf("%q parsed", bad)
		}
	}
}
