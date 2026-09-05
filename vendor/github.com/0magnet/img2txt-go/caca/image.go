package caca

import (
	"image"
	"image/draw"
	_ "image/gif"  // register GIF decoder
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"io"
	"os"

	_ "golang.org/x/image/bmp"  // register BMP decoder
	_ "golang.org/x/image/tiff" // register TIFF decoder
	_ "golang.org/x/image/webp" // register WebP decoder
)

// Image is a decoded bitmap together with its dither.
type Image struct {
	Pixels []byte
	W, H   int
	Dither *Dither
}

// LoadImage decodes an image file into the 32-bit ARGB layout libcaca expects.
func LoadImage(name string) (*Image, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return DecodeImage(f)
}

// DecodeImage decodes an image from r into the 32-bit ARGB layout libcaca
// expects: each pixel is 0xAARRGGBB stored little-endian, matching the buffer
// Imlib2 hands img2txt.
func DecodeImage(r io.Reader) (*Image, error) {
	src, _, err := image.Decode(r)
	if err != nil {
		return nil, err
	}

	b := src.Bounds()
	w, h := b.Dx(), b.Dy()

	// Convert to non-premultiplied 8-bit RGBA.
	rgba := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(rgba, rgba.Bounds(), src, b.Min, draw.Src)

	pixels := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := rgba.PixOffset(x, y)
			o := (y*w + x) * 4
			// Little-endian 0xAARRGGBB is B, G, R, A in memory.
			pixels[o+0] = rgba.Pix[i+2] // blue
			pixels[o+1] = rgba.Pix[i+1] // green
			pixels[o+2] = rgba.Pix[i+0] // red
			pixels[o+3] = rgba.Pix[i+3] // alpha
		}
	}

	im := &Image{Pixels: pixels, W: w, H: h}
	im.Dither = NewDither(32, w, h, 4*w, 0x00ff0000, 0x0000ff00, 0x000000ff, 0xff000000)
	if im.Dither == nil {
		return nil, errUnsupported
	}
	return im, nil
}

// errUnsupported reports an image geometry libcaca cannot dither.
var errUnsupported = errorString("unsupported image geometry")

type errorString string

func (e errorString) Error() string { return string(e) }
