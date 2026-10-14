package rasterview

import (
	"image"
	"image/color"
	"testing"
)

// Face-on, a quad covers the box Render puts its corners at, its picture
// the right way up: the picture's top row at the top of the frame.
func TestRenderQuadFillsTheBoxOfItsCorners(t *testing.T) {
	pic := image.NewRGBA(image.Rect(0, 0, 2, 2))
	red, blue := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}
	pic.SetRGBA(0, 0, red) // top left
	pic.SetRGBA(1, 0, red)
	pic.SetRGBA(0, 1, blue)
	pic.SetRGBA(1, 1, blue)
	dst := image.NewRGBA(image.Rect(0, 0, 100, 100))
	v := View{Scale: 1}
	v.RenderQuad(dst, [3]float64{-1, -1, 0}, [3]float64{1, -1, 0}, [3]float64{-1, 1, 0}, 2, pic)
	// Reach 2 fills half the half-frame: the quad is 25..75.
	for _, c := range []struct {
		x, y int
		want color.RGBA
	}{{30, 30, red}, {70, 30, red}, {30, 70, blue}, {70, 70, blue}, {20, 50, color.RGBA{}}, {50, 80, color.RGBA{}}} {
		if got := dst.RGBAAt(c.x, c.y); got != c.want {
			t.Errorf("(%d,%d) is %v, want %v", c.x, c.y, got, c.want)
		}
	}
	// Turned half round about Y it is seen from behind, mirrored.
	dst = image.NewRGBA(image.Rect(0, 0, 100, 100))
	pic.SetRGBA(0, 0, blue)
	v.AngleY = 3.141592653589793
	v.RenderQuad(dst, [3]float64{-1, -1, 0}, [3]float64{1, -1, 0}, [3]float64{-1, 1, 0}, 2, pic)
	if got := dst.RGBAAt(70, 30); got != blue {
		t.Errorf("from behind, the picture's top left is at the right: (70,30) is %v", got)
	}
}
