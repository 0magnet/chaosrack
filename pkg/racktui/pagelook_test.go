package racktui

import (
	"testing"

	"github.com/0magnet/chaosrack/pkg/rackpic"
	"github.com/0magnet/chaosrack/pkg/racksurface"
)

// cellBuf is a Painter that keeps what was painted.
type cellBuf map[[2]int]cellAt

func (b cellBuf) Set(x, y int, c cellAt) { b[[2]int{x, y}] = c }

func ptr[T any](v T) *T { return &v }

// A box is painted at the fixed scale: a box of 24 x 10 page pixels is 10
// cells across and 2 down, whatever the terminal.
func TestABoxIsItsSizeAtTheScale(t *testing.T) {
	pic := &rackpic.Picture{W: 100, H: 50, Items: []*rackpic.Item{
		{X: 24, Y: 10, W: 24, H: 10, Fill: ptr(0xff0000)},
	}}
	b := cellBuf{}
	drawPicture(b, pic, racksurface.View{W: 40, H: 10}, "", nil)
	red := 0
	for _, c := range b {
		if c.Top == [3]uint8{255, 0, 0} {
			red++
		}
	}
	if red != 20 {
		t.Errorf("%d cells are red, want 10 x 2", red)
	}
	if c := b[[2]int{10, 2}]; c.Top != [3]uint8{255, 0, 0} || c.Bottom != [3]uint8{255, 0, 0} {
		t.Errorf("the box's corner cell is %+v", c)
	}
}

// Words are characters, in their ink, on what is under them; and a box
// painted later over them hides them, as on the page.
func TestWordsAreCharactersAndABoxOverThemHidesThem(t *testing.T) {
	pic := &rackpic.Picture{W: 100, H: 50, Items: []*rackpic.Item{
		{X: 0, Y: 0, W: 24, H: 10, Text: "hi", Ink: ptr(0x00ff00)},
		{X: 24, Y: 20, W: 24, H: 10, Text: "no", Ink: ptr(0x00ff00)},
		{X: 20, Y: 15, W: 40, H: 20, Fill: ptr(0x101010)},
	}}
	b := cellBuf{}
	drawPicture(b, pic, racksurface.View{W: 40, H: 10}, "", nil)
	if c := b[[2]int{0, 1}]; c.Ch != 'h' || c.Top != [3]uint8{0, 255, 0} {
		t.Errorf("the word's first cell is %+v", c)
	}
	if c := b[[2]int{10, 5}]; c.Ch == 'n' {
		t.Error("a word under a box shows through it")
	}
}

// The view is a window: the same picture panned shows the box elsewhere.
func TestTheViewPansThePicture(t *testing.T) {
	pic := &rackpic.Picture{W: 100, H: 50, Items: []*rackpic.Item{
		{X: 24, Y: 10, W: 2.4, H: 5, Fill: ptr(0xff0000)},
	}}
	b := cellBuf{}
	drawPicture(b, pic, racksurface.View{X: 5, Y: 1, W: 20, H: 5}, "", nil)
	if c := b[[2]int{5, 1}]; c.Top != [3]uint8{255, 0, 0} {
		t.Errorf("the box at 10,2 seen from 5,1 is %+v", c)
	}
}

// The control the keys are on is outlined; the mouse finds a control by its
// part, the smallest when parts nest.
func TestTheCursorIsOutlinedAndTheMouseFindsTheControl(t *testing.T) {
	pic := &rackpic.Picture{W: 200, H: 100, Ctls: map[string][4]float64{
		"cell": {24, 10, 72, 50},
		"btn":  {72, 20, 12, 10},
	}}
	b := cellBuf{}
	drawPicture(b, pic, racksurface.View{W: 80, H: 20}, "cell", nil)
	ink := [3]uint8{cursorInk.R, cursorInk.G, cursorInk.B}
	if c := b[[2]int{10, 5}]; c.Top != ink && c.Bottom != ink {
		t.Errorf("the cursor's left edge is %+v", c)
	}
	if id, _ := ctlAtPage(pic, 75, 25); id != "btn" {
		t.Errorf("inside both, the mouse found %q", id)
	}
	if id, _ := ctlAtPage(pic, 30, 40); id != "cell" {
		t.Errorf("inside the cell only, the mouse found %q", id)
	}
	if _, ok := ctlAtPage(pic, 150, 90); ok {
		t.Error("the mouse found a control where there is none")
	}
}

// A canvas is drawn from the pixels sampled for it, scaled to its box.
func TestACanvasIsDrawnFromItsPixels(t *testing.T) {
	pic := &rackpic.Picture{W: 100, H: 50, Items: []*rackpic.Item{
		{X: 24, Y: 10, W: 24, H: 10, Canvas: 1},
	}}
	img := &rackpic.Image{W: 2, H: 1, Px: []byte{255, 0, 0, 0, 0, 255}}
	b := cellBuf{}
	drawPicture(b, pic, racksurface.View{W: 40, H: 10}, "", map[int]*rackpic.Image{0: img})
	if c := b[[2]int{10, 2}]; c.Top != [3]uint8{255, 0, 0} {
		t.Errorf("the canvas's left is %+v, want the image's left pixel", c)
	}
	if c := b[[2]int{19, 2}]; c.Top != [3]uint8{0, 0, 255} {
		t.Errorf("the canvas's right is %+v, want the image's right pixel", c)
	}
	if want := canvasWants(pic, racksurface.View{W: 40, H: 10}); len(want) != 1 || want[0].W != 10 || want[0].H != 4 {
		t.Errorf("the canvas is asked for as %+v, want 10 x 4 pixels", want)
	}
}
