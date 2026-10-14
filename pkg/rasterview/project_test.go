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
	v.RenderQuad(dst, Quad{BL: [3]float64{-1, -1, 0}, BR: [3]float64{1, -1, 0}, TL: [3]float64{-1, 1, 0}, Pic: pic}, 2)
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
	v.RenderQuad(dst, Quad{BL: [3]float64{-1, -1, 0}, BR: [3]float64{1, -1, 0}, TL: [3]float64{-1, 1, 0}, Pic: pic}, 2)
	if got := dst.RGBAAt(70, 30); got != blue {
		t.Errorf("from behind, the picture's top left is at the right: (70,30) is %v", got)
	}
}

// A shifted quad scrolls its picture left, wrapping round.
func TestRenderQuadShiftScrolls(t *testing.T) {
	pic := image.NewRGBA(image.Rect(0, 0, 4, 1))
	for x, v := range []uint8{0, 60, 120, 180} {
		pic.SetRGBA(x, 0, color.RGBA{v, 0, 0, 255})
	}
	dst := image.NewRGBA(image.Rect(0, 0, 100, 100))
	q := Quad{BL: [3]float64{-1, -1, 0}, BR: [3]float64{1, -1, 0}, TL: [3]float64{-1, 1, 0}, Pic: pic, Shift: 0.25}
	View{Scale: 1}.RenderQuad(dst, q, 2)
	// The quad is 25..75 across: four columns of 12.5 pixels.
	for i, want := range []uint8{60, 120, 180, 0} {
		if got := dst.RGBAAt(31+12*i, 50).R; got != want {
			t.Errorf("column %d shows picture column %d, want %d", i, got/60, want/60)
		}
	}
}

// The three-color map mixes through the page's green, not black.
func TestTheDefaultMiddleSwatchIsGreen(t *testing.T) {
	g := DefaultGradient()
	g.Colors = 3
	if c := g.ColorAt(0, 0, 0.5, [3]float32{}, [3]float32{1, 1, 1}, 0); c != [3]float32{0, 1, 0} {
		t.Errorf("halfway along a three-color map is %v", c)
	}
}
