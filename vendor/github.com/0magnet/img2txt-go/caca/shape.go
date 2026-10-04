package caca

import (
	"math"

	"github.com/0magnet/img2txt-go/shape"
)

// ShapeEntries measures the shape vector of every printable ASCII glyph in f
// and normalizes them as a set. It is what `go generate` in package shape
// writes into shape.ASCII, from the built-in Monospace 9 font.
func ShapeEntries(f *Font) []shape.Entry {
	var runes []rune
	var vs []shape.Vector
	for r := rune(0x20); r <= 0x7e; r++ {
		w, h, pix, ok := f.Glyph(r)
		if !ok {
			continue
		}
		runes = append(runes, r)
		vs = append(vs, shape.Measure(w, h, func(x, y int) float64 {
			return float64(pix[y*w+x]) / 255
		}))
	}
	shape.Normalize(vs)
	out := make([]shape.Entry, len(runes))
	for i := range runes {
		out[i] = shape.Entry{R: runes[i], V: vs[i]}
	}
	return out
}

// SetShape turns shape matching on or off. With it on, DitherBitmap still
// picks every cell's colors exactly as before, but chooses the character by
// comparing the cell's shape with each printable ASCII glyph's (see package
// shape) instead of by its brightness. This is not a libcaca feature.
func (d *Dither) SetShape(on bool) { d.shape = on }

// shapeGlyph picks the glyph for the cell whose top-left corner is (x0,y0)
// in bitmap pixels and whose size is cw by ch, and returns it with the cell's
// foreground and background: fg and bg, swapped when bg is the lighter.
//
// The lighter color is the ink, so strokes trace the light parts of the
// image, as text on a dark terminal does. The cell shows the same pair of
// colors either way; only which one the glyph is drawn in changes.
//
// A point's ink is its lightness placed between the two colors' lightness:
// 0 at the background's, 1 at the foreground's, clamped. Lightness rather
// than a projection onto the line between the two colors, because the
// dither's integer arithmetic sometimes pairs colors that line does not pass
// near (darkgray with cyan for a black and white edge), and lightness still
// says which side of the edge a point is on.
//
// off is the offset the dither added to the cell's color (diffused error or
// an ordered threshold). Adding it to every sample keeps the ink in step
// with the colors the dither chose.
func (d *Dither) shapeGlyph(pixels []byte, x0, y0, cw, ch float64, fg, bg int32, off [3]float64) (rune, int32, int32) {
	lf, lb := paletteLum(fg), paletteLum(bg)
	if lb > lf {
		fg, bg, lf, lb = bg, fg, lb, lf
	}
	span := float64(lf - lb)
	lo := float64(lb)
	if span == 0 {
		// Two colors of equal lightness: use absolute lightness instead.
		span, lo = 8*0xfff, 0
	}
	ink := func(u, v float64) float64 {
		c := d.rgbAt(pixels, x0+u*cw, y0+v*ch)
		l := 3*(c[0]+off[0]) + 4*(c[1]+off[1]) + (c[2] + off[2])
		return math.Min(1, math.Max(0, (l-lo)/span))
	}
	return shape.Pick(ink, shape.Default), fg, bg
}

// paletteLum is the lightness of palette entry i, weighted as the gray dither
// weights it.
func paletteLum(i int32) int32 {
	return 3*rgbPalette[i*3] + 4*rgbPalette[i*3+1] + rgbPalette[i*3+2]
}

// rgbAt returns the gamma-corrected 12-bit color at (x,y) in bitmap pixels,
// interpolated bilinearly between pixel centers and clamped to the bitmap.
func (d *Dither) rgbAt(pixels []byte, x, y float64) [3]float64 {
	x, y = x-0.5, y-0.5
	x = math.Min(math.Max(x, 0), float64(d.w-1))
	y = math.Min(math.Max(y, 0), float64(d.h-1))
	ix, iy := int(x), int(y)
	fx, fy := x-float64(ix), y-float64(iy)
	ix1, iy1 := min(ix+1, d.w-1), min(iy+1, d.h-1)
	var out [3]float64
	for _, s := range [4]struct {
		x, y int
		w    float64
	}{
		{ix, iy, (1 - fx) * (1 - fy)},
		{ix1, iy, fx * (1 - fy)},
		{ix, iy1, (1 - fx) * fy},
		{ix1, iy1, fx * fy},
	} {
		if s.w == 0 {
			continue
		}
		var rgba [4]uint32
		d.getRGBA(pixels, s.x, s.y, &rgba)
		for i := 0; i < 3; i++ {
			out[i] += s.w * float64(rgba[i])
		}
	}
	return out
}
