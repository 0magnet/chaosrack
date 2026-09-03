package cdp

import (
	"image"
	"image/color"
	"testing"
)

// filled builds a w*h image of one color.
func filled(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

var (
	black = color.RGBA{0, 0, 0, 255}
	white = color.RGBA{255, 255, 255, 255}
	// Just under the brightness threshold the helpers use, which is where a
	// nearly-black background sits.
	nearBlack = color.RGBA{20, 20, 20, 255}
)

func TestBrightFracOnAUniformImage(t *testing.T) {
	all := image.Rect(0, 0, 40, 40)
	if got := BrightFrac(filled(40, 40, black), all); got != 0 {
		t.Errorf("an all-black image is %v bright, want 0", got)
	}
	if got := BrightFrac(filled(40, 40, white), all); got != 1 {
		t.Errorf("an all-white image is %v bright, want 1", got)
	}
}

// The threshold is what separates the background from the drawing, so a very
// dark grey has to count as unlit.
func TestBrightFracTreatsNearBlackAsUnlit(t *testing.T) {
	if got := BrightFrac(filled(20, 20, nearBlack), image.Rect(0, 0, 20, 20)); got != 0 {
		t.Errorf("a near-black image is %v bright, want 0", got)
	}
}

func TestBrightFracOnAHalfLitImage(t *testing.T) {
	img := filled(40, 40, black)
	for y := 0; y < 40; y++ {
		for x := 0; x < 20; x++ {
			img.SetRGBA(x, y, white)
		}
	}
	got := BrightFrac(img, img.Bounds())
	if got < 0.45 || got > 0.55 {
		t.Errorf("a half-lit image is %v bright, want about 0.5", got)
	}
}

// The rectangle is clipped to the image, so a region that runs off the edge
// measures the part that exists rather than reading out of bounds.
func TestBrightFracClipsToTheImage(t *testing.T) {
	img := filled(20, 20, white)
	if got := BrightFrac(img, image.Rect(-100, -100, 200, 200)); got != 1 {
		t.Errorf("an oversized region is %v bright, want 1", got)
	}
	if got := BrightFrac(img, image.Rect(100, 100, 200, 200)); got != 0 {
		t.Errorf("a region entirely outside is %v bright, want 0", got)
	}
	if got := BrightFrac(img, image.Rectangle{}); got != 0 {
		t.Errorf("an empty region is %v bright, want 0", got)
	}
}

// The box is what catches a model collapsed to a dot or blown up off-screen,
// so it has to actually track where the lit pixels are.
func TestBrightBBoxFindsTheDrawnRegion(t *testing.T) {
	img := filled(60, 60, black)
	for y := 20; y < 40; y++ {
		for x := 10; x < 30; x++ {
			img.SetRGBA(x, y, white)
		}
	}
	got := BrightBBox(img, img.Bounds())
	if got.Empty() {
		t.Fatal("nothing was found in an image with a lit block in it")
	}
	// Sampling is every other pixel, so the box can be a pixel short of the
	// block; it must still sit inside it and cover most of it.
	if got.Min.X < 10 || got.Min.Y < 20 || got.Max.X > 30 || got.Max.Y > 40 {
		t.Errorf("box %v is outside the lit block 10,20-30,40", got)
	}
	if got.Dx() < 16 || got.Dy() < 16 {
		t.Errorf("box %v is much smaller than the 20x20 block", got)
	}
}

// Nothing drawn is an empty box rather than the whole frame, which is what
// tells a check that the model never appeared.
func TestBrightBBoxOnAnEmptyFrameIsEmpty(t *testing.T) {
	if got := BrightBBox(filled(40, 40, black), image.Rect(0, 0, 40, 40)); !got.Empty() {
		t.Errorf("an unlit image gave the box %v, want empty", got)
	}
}

func TestBrightBBoxOnAFullyLitFrameHugsTheEdges(t *testing.T) {
	img := filled(40, 40, white)
	got := BrightBBox(img, img.Bounds())
	if got.Min.X > 1 || got.Min.Y > 1 || got.Max.X < 36 || got.Max.Y < 36 {
		t.Errorf("a fully lit image gave the box %v, want it hugging the edges", got)
	}
}

// ── DiffFrac ─────────────────────────────────────────────────────────────────

func TestDiffFracOnIdenticalImages(t *testing.T) {
	a := filled(30, 30, color.RGBA{10, 20, 30, 255})
	b := filled(30, 30, color.RGBA{10, 20, 30, 255})
	if got := DiffFrac(a, b, 0); got != 0 {
		t.Errorf("identical images differ by %v, want 0", got)
	}
}

func TestDiffFracOnOppositeImages(t *testing.T) {
	if got := DiffFrac(filled(30, 30, black), filled(30, 30, white), 0); got != 1 {
		t.Errorf("black against white differs by %v, want 1", got)
	}
}

// The tolerance is what stops compression noise from reading as a change, so a
// difference inside it must not count.
func TestDiffFracRespectsTheTolerance(t *testing.T) {
	a := filled(20, 20, color.RGBA{100, 100, 100, 255})
	b := filled(20, 20, color.RGBA{105, 105, 105, 255})

	if got := DiffFrac(a, b, 10); got != 0 {
		t.Errorf("a difference of 5 with a tolerance of 10 counted as %v, want 0", got)
	}
	if got := DiffFrac(a, b, 2); got != 1 {
		t.Errorf("a difference of 5 with a tolerance of 2 counted as %v, want 1", got)
	}
	// The tolerance is exclusive: exactly the tolerance is not a difference.
	if got := DiffFrac(a, b, 5); got != 0 {
		t.Errorf("a difference of exactly the tolerance counted as %v, want 0", got)
	}
}

// Images of different sizes cannot be compared pixel for pixel. Reporting them
// as completely different is what makes a size change fail a check rather than
// pass one.
func TestDiffFracOnMismatchedSizesIsCompletelyDifferent(t *testing.T) {
	if got := DiffFrac(filled(30, 30, white), filled(20, 30, white), 0); got != 1 {
		t.Errorf("images of different widths differ by %v, want 1", got)
	}
	if got := DiffFrac(filled(30, 30, white), filled(30, 20, white), 0); got != 1 {
		t.Errorf("images of different heights differ by %v, want 1", got)
	}
}

// A single channel changing is still a change; comparing only luminance would
// miss a color shift that keeps the same brightness.
func TestDiffFracNoticesASingleChannel(t *testing.T) {
	a := filled(20, 20, color.RGBA{200, 0, 0, 255})
	b := filled(20, 20, color.RGBA{0, 200, 0, 255})
	if got := DiffFrac(a, b, 10); got != 1 {
		t.Errorf("red against green differs by %v, want 1", got)
	}
}

func TestDiffFracOnEmptyImages(t *testing.T) {
	empty := image.NewRGBA(image.Rectangle{})
	if got := DiffFrac(empty, empty, 0); got != 0 {
		t.Errorf("two empty images differ by %v, want 0", got)
	}
}

func TestAbsuIsSymmetric(t *testing.T) {
	for _, tc := range [][2]uint32{{0, 0}, {5, 3}, {3, 5}, {0, 255}, {255, 0}} {
		a, b := tc[0], tc[1]
		if absu(a, b) != absu(b, a) {
			t.Errorf("absu(%d,%d)=%d but absu(%d,%d)=%d", a, b, absu(a, b), b, a, absu(b, a))
		}
	}
	if got := absu(5, 3); got != 2 {
		t.Errorf("absu(5,3) = %d, want 2", got)
	}
}
