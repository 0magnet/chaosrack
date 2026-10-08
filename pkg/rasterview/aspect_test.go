package rasterview

import (
	"image"
	"math"
	"testing"
)

// TestPixelAspect: a circle drawn with pixels half as wide as they are tall
// spans twice as many columns as rows, so it is round once those pixels are
// laid on the page.
func TestPixelAspect(t *testing.T) {
	const n = 64
	var verts []float32
	var idx []uint16
	for i := range n {
		a := 2 * math.Pi * float64(i) / n
		verts = append(verts, float32(math.Cos(a)), float32(math.Sin(a)), 0)
		idx = append(idx, uint16(i), uint16((i+1)%n))
	}
	span := func(aspect float64) (w, h int) {
		img := image.NewRGBA(image.Rect(0, 0, 200, 100))
		View{PixelAspect: aspect}.Render(img, verts, idx, DefaultGradient())
		x0, x1, y0, y1 := 200, -1, 100, -1
		for y := range 100 {
			for x := range 200 {
				if img.RGBAAt(x, y).A != 0 {
					x0, x1, y0, y1 = min(x0, x), max(x1, x), min(y0, y), max(y1, y)
				}
			}
		}
		return x1 - x0 + 1, y1 - y0 + 1
	}
	if w, h := span(0); math.Abs(float64(w)/float64(h)-1) > 0.05 {
		t.Errorf("square pixels: %dx%d, want round", w, h)
	}
	if w, h := span(0.5); math.Abs(float64(w)/float64(h)-2) > 0.1 {
		t.Errorf("half-width pixels: %dx%d, want twice as wide", w, h)
	}
}
