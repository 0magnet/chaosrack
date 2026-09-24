// Package panelart draws panel hardware — knobs, lamps, LED readouts — into a
// pixel buffer, and turns a pixel buffer into terminal cells.
//
// The point is that the drawing is ORDINARY GO with no DOM and no terminal in
// it. A knob is painted from its value and its detent count into an image, and
// that image can be blitted to a canvas in the page or emitted as half-block
// cells in a terminal. That is what makes one implementation serve both
// surfaces, and it is the difference between this and photographing the page:
// a screenshot is the HTML implementation plus a camera, it cannot be
// interactive, and overlaying controls on a picture means keeping a second
// coordinate system in step with the first forever.
//
// Why half blocks. A terminal cell is about twice as tall as it is wide, so
// U+2580 (UPPER HALF BLOCK) with a foreground and a background color is two
// stacked square pixels — the vertical resolution doubles and the color is
// exact. Measured against the alternative: libcaca's img2txt renders a dark
// control panel as noise, because it is a 16-color glyph-ramp renderer built
// for photographs and a near-black panel of subtle greys gives it no dynamic
// range. Half blocks at 24-bit make the same module legible at 40 columns, and
// a knob drawn this way reads as a knob from about 8 columns up.
package panelart

import (
	"image"
	"image/color"
	"math"
)

// Palette is the panel's colors. One struct so a theme is one value, and so
// the terminal and the page cannot drift apart by each hardcoding its own.
type Palette struct {
	Panel  color.RGBA // the panel behind everything
	Body   color.RGBA // the knob's own color, at full light
	Shadow color.RGBA // its unlit side
	Rim    color.RGBA
	Mark   color.RGBA // the pointer
	Detent color.RGBA
	LEDOn  color.RGBA
	LEDOff color.RGBA
}

// Dark is chaosrack's panel: near-black frame, grey hardware, red readouts.
var Dark = Palette{
	Panel:  color.RGBA{10, 13, 20, 255},
	Body:   color.RGBA{190, 190, 196, 255},
	Shadow: color.RGBA{54, 56, 62, 255},
	Rim:    color.RGBA{28, 30, 36, 255},
	Mark:   color.RGBA{240, 240, 245, 255},
	Detent: color.RGBA{60, 200, 90, 255},
	LEDOn:  color.RGBA{255, 40, 40, 255},
	LEDOff: color.RGBA{60, 12, 12, 255},
}

// knobSweep is the arc a panel knob turns through, and where it starts.
//
// 300 degrees with the dead zone at the BOTTOM, which is what a real
// potentiometer does and why a knob's minimum points down-left rather than
// straight down. A full 360 would make the ends indistinguishable.
const (
	knobSweep = 300 * math.Pi / 180
	knobStart = math.Pi/2 + (2*math.Pi-knobSweep)/2
)

// KnobAngle is where the pointer sits for a fraction of the sweep. Exported
// because hit-testing a drag wants the same arithmetic the drawing used.
func KnobAngle(frac float64) float64 {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	return knobStart + frac*knobSweep
}

// Knob paints a knob of the given pixel size, turned to frac of its sweep,
// with detents ticks around it. detents below 2 draws none — a continuous
// control has no positions to mark.
func Knob(size int, frac float64, detents int, p Palette) *image.RGBA {
	if size < 2 {
		size = 2
	}
	im := image.NewRGBA(image.Rect(0, 0, size, size))
	fill(im, p.Panel)

	c := float64(size-1) / 2
	r := c * 0.72
	for y := range size {
		for x := range size {
			dx, dy := float64(x)-c, float64(y)-c
			d := math.Hypot(dx, dy)
			if d > r {
				continue
			}
			if d > r*0.93 {
				im.SetRGBA(x, y, p.Rim)
				continue
			}
			// Lit from the top left, which is where every panel photograph in
			// the world is lit from, and what makes a flat disc read as a
			// cylinder you could take hold of.
			l := clamp01((-dx/r*0.5 - dy/r*0.7 + 1) / 2)
			im.SetRGBA(x, y, mix(p.Shadow, p.Body, l))
		}
	}

	ang := KnobAngle(frac)
	// The pointer stops short of the center: a line all the way through looks
	// like a crack, and a real pointer is a flat milled into the skirt.
	for t := 0.18; t < 0.9; t += 0.5 / r {
		plot(im, c+math.Cos(ang)*r*t, c+math.Sin(ang)*r*t, p.Mark)
	}
	if detents >= 2 {
		for i := range detents {
			a := knobStart + knobSweep*float64(i)/float64(detents-1)
			for t := 1.06; t < 1.24; t += 0.5 / r {
				plot(im, c+math.Cos(a)*r*t, c+math.Sin(a)*r*t, p.Detent)
			}
		}
	}
	return im
}

// Lamp paints a round indicator, lit or not.
func Lamp(size int, on bool, p Palette) *image.RGBA {
	if size < 2 {
		size = 2
	}
	im := image.NewRGBA(image.Rect(0, 0, size, size))
	fill(im, p.Panel)
	c := float64(size-1) / 2
	r := c * 0.8
	body := p.LEDOff
	if on {
		body = p.LEDOn
	}
	for y := range size {
		for x := range size {
			d := math.Hypot(float64(x)-c, float64(y)-c)
			if d > r {
				continue
			}
			// A lit lamp blooms toward its middle; an unlit one is just a
			// dark lens, so the falloff is flatter.
			var k float64
			if on {
				k = 0.55 + 0.45*(1-d/r)
			} else {
				k = 0.8 + 0.2*(1-d/r)
			}
			im.SetRGBA(x, y, scale(body, k))
		}
	}
	return im
}

func fill(im *image.RGBA, c color.RGBA) {
	b := im.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			im.SetRGBA(x, y, c)
		}
	}
}

func plot(im *image.RGBA, fx, fy float64, c color.RGBA) {
	x, y := int(fx+0.5), int(fy+0.5)
	if (image.Point{X: x, Y: y}).In(im.Bounds()) {
		im.SetRGBA(x, y, c)
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func mix(a, b color.RGBA, t float64) color.RGBA {
	t = clamp01(t)
	return color.RGBA{
		R: uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		G: uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		B: uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		A: 255,
	}
}

func scale(c color.RGBA, k float64) color.RGBA {
	f := func(v uint8) uint8 {
		n := float64(v) * k
		if n > 255 {
			n = 255
		}
		if n < 0 {
			n = 0
		}
		return uint8(n)
	}
	return color.RGBA{R: f(c.R), G: f(c.G), B: f(c.B), A: 255}
}
