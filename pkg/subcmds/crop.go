package subcmds

import (
	"image"
	"image/color"

	"image/draw"
)

// --crop: frame the picture to what is drawn rather than to the figure's
// extent. The renderer fits a model's bounding box, and a few sparse loops at
// the edge of an attractor make the dense part a small figure in a corner;
// the page's portraits were cropped to the lit area, and this does the same.
// An animation is cropped to the lit area of all its frames together, so the
// camera holds still across the loop.

// cropOver is how much larger the picture is drawn when it is to be cropped,
// so the crop is scaled down into the output rather than up.
const cropOver = 2

// drawSize is the size to draw at: the output's, or cropOver times it.
func drawSize() (w, h int) {
	if renderCrop {
		return renderW * cropOver, renderH * cropOver
	}
	return renderW, renderH
}

// cropLit crops every image to one square around what is drawn in any of
// them — whatever is not the background — with a margin, and scales each to
// the output size. Without --crop it returns them as they are.
func cropLit(imgs []*image.RGBA) []*image.RGBA {
	if !renderCrop || len(imgs) == 0 {
		return imgs
	}
	bg := background()
	b := imgs[0].Bounds()
	lit := image.Rectangle{}
	for _, m := range imgs {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if !nearColor(m.RGBAAt(x, y), bg) {
					lit = lit.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
	}
	if lit.Empty() {
		lit = b
	}
	// The output's shape about the lit area's middle, a twentieth wider each
	// side. Where it runs past the drawing, the part past it is background,
	// placed where it falls rather than squeezed in.
	w, h := float64(lit.Dx())*1.1, float64(lit.Dy())*1.1
	if aspect := float64(renderW) / float64(renderH); w/h < aspect {
		w = h * aspect
	} else {
		h = w / aspect
	}
	cx, cy := float64(lit.Min.X+lit.Max.X)/2, float64(lit.Min.Y+lit.Max.Y)/2
	r := image.Rect(int(cx-w/2), int(cy-h/2), int(cx+w/2), int(cy+h/2))
	out := make([]*image.RGBA, len(imgs))
	for i, m := range imgs {
		region := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
		draw.Draw(region, region.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
		draw.Draw(region, r.Intersect(b).Sub(r.Min), m, r.Intersect(b).Min, draw.Src)
		out[i] = boxScale(region, renderW, renderH)
	}
	return out
}

// nearColor reports whether a pixel is the background, give or take the
// JPEG-free rounding a renderer leaves.
func nearColor(a, b color.RGBA) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R)+d(a.G, b.G)+d(a.B, b.B) < 24
}

// cropOne is cropLit for a still.
func cropOne(m *image.RGBA) *image.RGBA { return cropLit([]*image.RGBA{m})[0] }

// boxScale shrinks src to w×h by averaging each output pixel's share of it.
// A box filter, not a resampler: thin lines drawn at cropOver times the size
// come out as thin lines rather than as dotted ones.
func boxScale(src *image.RGBA, w, h int) *image.RGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		y0, y1 := y*sh/h, max((y+1)*sh/h, y*sh/h+1)
		for x := range w {
			x0, x1 := x*sw/w, max((x+1)*sw/w, x*sw/w+1)
			var r, g, b, n int
			for yy := y0; yy < y1 && yy < sh; yy++ {
				row := src.Pix[yy*src.Stride:]
				for xx := x0; xx < x1 && xx < sw; xx++ {
					r += int(row[xx*4])
					g += int(row[xx*4+1])
					b += int(row[xx*4+2])
					n++
				}
			}
			if n > 0 {
				out.SetRGBA(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(b / n), 255}) //nolint:gosec // averages of bytes
			}
		}
	}
	return out
}
