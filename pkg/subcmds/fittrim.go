package subcmds

import (
	"image"
	"slices"
)

// --fit-trim: frame an animation to where the model spends its time, not to
// the farthest it ever reaches.
//
// A run's bounding box is set by its rarest excursions. The hyper-Rössler
// spends nearly all its time in a disc and now and then throws a spike several
// times as tall, and the Wang–Sun's wing tips reach out the same way; fitted
// to the box, every frame drew the disc as a speck to leave room for a spike
// in a few of them. So the camera is fitted to the points inside each axis's
// central range, and the crop to where most of the lit pixels are: a spike
// runs out of the frame for the moment it lasts, as one would out of a
// camera's.

// renderFitTrim is the share of the points, and of the lit pixels, left out
// at each end of each axis. 0 frames everything, as before.
var renderFitTrim float64

// fitPoints are the points an animation's camera is fitted to: pts thinned to
// n, less the trimmed tails of each axis.
func fitPoints(pts [][3]float64, n int) [][3]float64 {
	fit := thinned(pts, n)
	if renderFitTrim <= 0 || len(fit) < 100 {
		return fit
	}
	var lo, hi [3]float64
	vals := make([]float64, len(fit))
	for k := range 3 {
		for i, p := range fit {
			vals[i] = p[k]
		}
		slices.Sort(vals)
		cut := int(renderFitTrim * float64(len(vals)))
		lo[k], hi[k] = vals[cut], vals[len(vals)-1-cut]
	}
	out := fit[:0:0]
	for _, p := range fit {
		if p[0] >= lo[0] && p[0] <= hi[0] && p[1] >= lo[1] && p[1] <= hi[1] && p[2] >= lo[2] && p[2] <= hi[2] {
			out = append(out, p)
		}
	}
	return out
}

// trimmedLit is the rectangle holding all but the trimmed share of the lit
// pixels at each side, from how many are lit in each column and each row
// across every frame.
func trimmedLit(cols, rows []int, b image.Rectangle) image.Rectangle {
	span := func(h []int) (int, int) {
		total := 0
		for _, c := range h {
			total += c
		}
		cut := int(renderFitTrim * float64(total))
		lo, hi, acc := 0, len(h)-1, 0
		for ; lo < len(h); lo++ {
			if acc += h[lo]; acc > cut {
				break
			}
		}
		acc = 0
		for ; hi >= 0; hi-- {
			if acc += h[hi]; acc > cut {
				break
			}
		}
		return lo, hi
	}
	x0, x1 := span(cols)
	y0, y1 := span(rows)
	if x1 < x0 || y1 < y0 {
		return image.Rectangle{}
	}
	return image.Rect(b.Min.X+x0, b.Min.Y+y0, b.Min.X+x1+1, b.Min.Y+y1+1)
}
