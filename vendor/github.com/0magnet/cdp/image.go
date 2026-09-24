package cdp

import "image"

// Image oracles: cheap "is the render blank, collapsed or changed" checks on a
// Screenshot, for tests that cannot know exactly what a frame should contain.

// BrightFrac returns the fraction of pixels in the region that are brighter
// than a near-black background (luminance > 24/255), i.e. how much is "drawn".
// A visual model render is thin bright lines on black, so it's small but > 0; a
// blank/failed render is ~0.
func BrightFrac(img image.Image, r image.Rectangle) float64 {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return 0
	}
	var bright, total int
	for y := r.Min.Y; y < r.Max.Y; y += 2 {
		for x := r.Min.X; x < r.Max.X; x += 2 {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			lum := (299*cr + 587*cg + 114*cb) / 1000 >> 8 // 0..255
			if lum > 24 {
				bright++
			}
			total++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(bright) / float64(total)
}

// BrightBBox returns the bounding box of the drawn (bright) pixels within r —
// useful to spot a model collapsed to a dot (tiny box) or blown up off-screen
// (box hugging the edges). Empty if nothing is drawn.
func BrightBBox(img image.Image, r image.Rectangle) image.Rectangle {
	r = r.Intersect(img.Bounds())
	minX, minY, maxX, maxY := r.Max.X, r.Max.Y, r.Min.X, r.Min.Y
	found := false
	for y := r.Min.Y; y < r.Max.Y; y += 2 {
		for x := r.Min.X; x < r.Max.X; x += 2 {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			if (299*cr+587*cg+114*cb)/1000>>8 > 24 {
				found = true
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if !found {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX, maxY)
}

// DiffFrac returns the fraction of sampled pixels that differ between a and b
// by more than tol per channel (0..255). Both images must be the same size.
func DiffFrac(a, b image.Image, tol int) float64 {
	ra, rb := a.Bounds(), b.Bounds()
	if ra.Dx() != rb.Dx() || ra.Dy() != rb.Dy() {
		return 1
	}
	var diff, total int
	t := uint32(tol) //nolint:gosec // a pixel dimension from the page; never negative
	for y := 0; y < ra.Dy(); y += 2 {
		for x := 0; x < ra.Dx(); x += 2 {
			ar, ag, ab, _ := a.At(ra.Min.X+x, ra.Min.Y+y).RGBA()
			br, bg, bb, _ := b.At(rb.Min.X+x, rb.Min.Y+y).RGBA()
			if absu(ar>>8, br>>8) > t || absu(ag>>8, bg>>8) > t || absu(ab>>8, bb>>8) > t {
				diff++
			}
			total++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(diff) / float64(total)
}

func absu(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}
