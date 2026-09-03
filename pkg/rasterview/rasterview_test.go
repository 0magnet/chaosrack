package rasterview

import (
	"image"
	"math"
	"testing"

	"github.com/0magnet/chaosrack/pkg/geom"
)

func renderGlobe(t *testing.T, v View, g Gradient) *image.RGBA {
	t.Helper()
	l := geom.Globe(18, 36, 60)
	dst := image.NewRGBA(image.Rect(0, 0, 200, 200))
	v.Render(dst, l.Vertices, l.Indices, g)
	return dst
}

// The default look: a red→blue gradient along model Z. With the poles
// upright (AngleX=π/2 turns model Z to screen Y) the top and bottom of
// the frame land on opposite gradient ends.
func TestDefaultGradientEnds(t *testing.T) {
	dst := renderGlobe(t, View{AngleX: math.Pi / 2, Dist: 3.2}, DefaultGradient())
	var sawRed, sawBlue, lit bool
	b := dst.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := dst.RGBAAt(x, y)
			if c.R|c.G|c.B != 0 {
				lit = true
			}
			if c.R > 200 && c.B < 60 {
				sawRed = true
			}
			if c.B > 200 && c.R < 60 {
				sawBlue = true
			}
		}
	}
	if !lit {
		t.Fatal("nothing was drawn")
	}
	if !sawRed || !sawBlue {
		t.Errorf("gradient ends missing: red=%v blue=%v", sawRed, sawBlue)
	}
}

// Rendering composites over what dst holds; it must not clear the frame.
func TestRenderDoesNotClear(t *testing.T) {
	l := geom.Globe(18, 36, 60)
	dst := image.NewRGBA(image.Rect(0, 0, 50, 50))
	for i := range dst.Pix {
		dst.Pix[i] = 7
	}
	(View{Dist: 3.2}).Render(dst, l.Vertices, l.Indices, DefaultGradient())
	corner := dst.RGBAAt(0, 0)
	if corner.R != 7 || corner.G != 7 || corner.B != 7 {
		t.Errorf("corner was cleared: %+v", corner)
	}
}

func TestMonochromeAndRainbow(t *testing.T) {
	mono := DefaultGradient()
	mono.Colors = 1
	dst := renderGlobe(t, View{Dist: 3.2}, mono)
	b := dst.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := dst.RGBAAt(x, y)
			if c.G != 0 || c.B != 0 {
				t.Fatalf("monochrome red frame has non-red pixel %+v at %d,%d", c, x, y)
			}
		}
	}
	rain := DefaultGradient()
	rain.Colors = 4
	dst = renderGlobe(t, View{Dist: 3.2}, rain)
	var sawGreen bool
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := dst.RGBAAt(x, y)
			if c.G > 200 {
				sawGreen = true
			}
		}
	}
	if !sawGreen {
		t.Error("rainbow gradient never passed through green")
	}
}

// BackDim darkens only the far hemisphere.
func TestBackDim(t *testing.T) {
	bright := renderGlobe(t, View{Dist: 3.2}, DefaultGradient())
	dimmed := renderGlobe(t, View{Dist: 3.2, BackDim: 0.5}, DefaultGradient())
	sum := func(img *image.RGBA) (s int) {
		for _, p := range img.Pix {
			s += int(p)
		}
		return s
	}
	if sum(dimmed) >= sum(bright) {
		t.Error("BackDim did not reduce total brightness")
	}
}
