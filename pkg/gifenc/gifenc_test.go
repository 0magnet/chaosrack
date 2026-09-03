package gifenc

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"testing"
)

// solid builds a w*h frame of one color.
func solid(w, h int, c color.RGBA) *image.RGBA {
	f := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i+3 < len(f.Pix); i += 4 {
		f.Pix[i], f.Pix[i+1], f.Pix[i+2], f.Pix[i+3] = c.R, c.G, c.B, 255
	}
	return f
}

// gradient builds a frame with many distinct colors, to push the palette
// against its 256-entry ceiling.
func gradient(w, h int) *image.RGBA {
	f := image.NewRGBA(image.Rect(0, 0, w, h))
	n := 0
	for i := 0; i+3 < len(f.Pix); i += 4 {
		f.Pix[i] = uint8(n % 251)
		f.Pix[i+1] = uint8((n * 7) % 253)
		f.Pix[i+2] = uint8((n * 13) % 249)
		f.Pix[i+3] = 255
		n++
	}
	return f
}

// A GIF palette cannot hold more than 256 colors. Going over is not a
// degraded picture, it is a file no decoder will open.
func TestAdaptivePaletteNeverExceeds256(t *testing.T) {
	for _, frames := range [][]*image.RGBA{
		{gradient(64, 64)},
		{gradient(64, 64), gradient(32, 32), solid(8, 8, color.RGBA{255, 0, 0, 255})},
	} {
		if got := len(AdaptivePalette(frames)); got > 256 {
			t.Errorf("palette has %d colors, the format allows 256", got)
		}
	}
}

// Black is the background and most of every frame. A palette that had to earn
// it could lose it on a bright clip, and then the background is not black.
func TestAdaptivePaletteAlwaysHoldsBlackFirst(t *testing.T) {
	bright := []*image.RGBA{solid(16, 16, color.RGBA{250, 250, 250, 255})}
	pal := AdaptivePalette(bright)
	if len(pal) == 0 {
		t.Fatal("empty palette")
	}
	r, g, b, _ := pal[0].RGBA()
	if r != 0 || g != 0 || b != 0 {
		t.Errorf("palette[0] = %v, want black even on a clip with none in it", pal[0])
	}
}

// Ties are broken on the key so that encoding the same frames twice gives the
// same file. Without it a re-run produces a different GIF for no reason.
func TestAdaptivePaletteIsDeterministic(t *testing.T) {
	frames := []*image.RGBA{gradient(48, 48), solid(48, 48, color.RGBA{10, 20, 30, 255})}
	first := AdaptivePalette(frames)
	for i := 0; i < 5; i++ {
		again := AdaptivePalette(frames)
		if len(again) != len(first) {
			t.Fatalf("run %d gave %d colors, the first gave %d", i, len(again), len(first))
		}
		for j := range first {
			if first[j] != again[j] {
				t.Fatalf("run %d differs at entry %d: %v vs %v", i, j, again[j], first[j])
			}
		}
	}
}

func TestAdaptivePaletteOnNoFrames(t *testing.T) {
	pal := AdaptivePalette(nil)
	if len(pal) != 1 {
		t.Errorf("an empty clip gave a %d-color palette, want just black", len(pal))
	}
}

// Every index Palettize writes has to be a real entry, or the decoder reads
// past the palette.
func TestPalettizeStaysInsideThePalette(t *testing.T) {
	src := gradient(40, 40)
	pal := AdaptivePalette([]*image.RGBA{src})
	out := Palettize(src, pal, map[uint32]uint8{})
	for i, idx := range out.Pix {
		if int(idx) >= len(pal) {
			t.Fatalf("pixel %d has index %d, the palette has %d entries", i, idx, len(pal))
		}
	}
}

func TestPalettizeKeepsTheFrameSize(t *testing.T) {
	src := gradient(23, 17) // deliberately not a round number
	out := Palettize(src, AdaptivePalette([]*image.RGBA{src}), map[uint32]uint8{})
	if got, want := out.Bounds(), src.Bounds(); got != want {
		t.Errorf("Palettize returned bounds %v, want %v", got, want)
	}
}

// The memo is shared across the frames of a clip. If it ever returned a
// different index for the same color, frames would not agree with each other.
func TestPalettizeMemoAgreesWithAFreshOne(t *testing.T) {
	a, b := gradient(32, 32), solid(32, 32, color.RGBA{7, 9, 11, 255})
	pal := AdaptivePalette([]*image.RGBA{a, b})

	shared := map[uint32]uint8{}
	Palettize(a, pal, shared)
	withMemo := Palettize(b, pal, shared)
	fresh := Palettize(b, pal, map[uint32]uint8{})

	if !bytes.Equal(withMemo.Pix, fresh.Pix) {
		t.Error("a warm memo produced different indices than a cold one")
	}
}

func TestPalettizeMapsAColorToItsNearestEntry(t *testing.T) {
	red := color.RGBA{255, 0, 0, 255}
	src := solid(8, 8, red)
	pal := AdaptivePalette([]*image.RGBA{src})
	out := Palettize(src, pal, map[uint32]uint8{})
	got := pal[out.Pix[0]]
	if got != pal.Convert(red) {
		t.Errorf("a solid red frame mapped to %v, nearest is %v", got, pal.Convert(red))
	}
}

// The whole point is a file a decoder will open. Reading it back is the only
// assertion that actually establishes that.
func TestEncodeRGBAProducesADecodableGIF(t *testing.T) {
	frames := []*image.RGBA{
		solid(16, 16, color.RGBA{0, 0, 0, 255}),
		gradient(16, 16),
		solid(16, 16, color.RGBA{40, 80, 120, 255}),
	}
	var buf bytes.Buffer
	if err := EncodeRGBA(&buf, frames, 0); err != nil {
		t.Fatalf("EncodeRGBA: %v", err)
	}
	g, err := gif.DecodeAll(&buf)
	if err != nil {
		t.Fatalf("the GIF this package wrote does not decode: %v", err)
	}
	if len(g.Image) != len(frames) {
		t.Errorf("wrote %d frames, read back %d", len(frames), len(g.Image))
	}
	if g.LoopCount != 0 {
		t.Errorf("LoopCount = %d, want 0 (loop forever)", g.LoopCount)
	}
	for i, d := range g.Delay {
		if d != DefaultDelay {
			t.Errorf("frame %d delay = %d, want the default %d", i, d, DefaultDelay)
		}
	}
}

func TestEncodeHonorsAnExplicitDelay(t *testing.T) {
	var buf bytes.Buffer
	if err := EncodeRGBA(&buf, []*image.RGBA{gradient(8, 8)}, 25); err != nil {
		t.Fatalf("EncodeRGBA: %v", err)
	}
	g, err := gif.DecodeAll(&buf)
	if err != nil {
		t.Fatalf("DecodeAll: %v", err)
	}
	if g.Delay[0] != 25 {
		t.Errorf("delay = %d, want the 25 that was asked for", g.Delay[0])
	}
}

// A zero or negative delay means "use the default" rather than "play as fast
// as the decoder can", which is what a literal zero would mean in the format.
func TestEncodeTreatsANonPositiveDelayAsTheDefault(t *testing.T) {
	for _, delay := range []int{0, -1, -100} {
		var buf bytes.Buffer
		if err := EncodeRGBA(&buf, []*image.RGBA{gradient(8, 8)}, delay); err != nil {
			t.Fatalf("EncodeRGBA(delay=%d): %v", delay, err)
		}
		g, err := gif.DecodeAll(&buf)
		if err != nil {
			t.Fatalf("DecodeAll: %v", err)
		}
		if g.Delay[0] != DefaultDelay {
			t.Errorf("delay %d encoded as %d, want the default %d", delay, g.Delay[0], DefaultDelay)
		}
	}
}

// EncodeRGBA on no frames writes nothing and reports no error: there was
// nothing to record, which is not a failure. Encode is the lower-level call
// and does not paper over it.
func TestEncodeRGBAOnNoFrames(t *testing.T) {
	var buf bytes.Buffer
	if err := EncodeRGBA(&buf, nil, 0); err != nil {
		t.Errorf("EncodeRGBA on an empty clip: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("an empty clip wrote %d bytes", buf.Len())
	}
}

func TestEncodeRejectsNoFrames(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, nil, 0); err == nil {
		t.Error("Encode wrote a GIF with no frames in it")
	}
}

// Frames of different sizes are what a window resize during a recording
// produces. Whatever the encoder does with them, it must not be to write a
// file that fails to decode.
func TestEncodeRGBAWithFramesOfDifferentSizes(t *testing.T) {
	frames := []*image.RGBA{gradient(16, 16), gradient(24, 12)}
	var buf bytes.Buffer
	err := EncodeRGBA(&buf, frames, 0)
	if err != nil {
		t.Logf("EncodeRGBA reported: %v", err) // a refusal is a fine answer
		return
	}
	if _, err := gif.DecodeAll(&buf); err != nil {
		t.Errorf("mixed frame sizes wrote a GIF that does not decode: %v", err)
	}
}

func BenchmarkEncodeRGBA(b *testing.B) {
	frames := []*image.RGBA{gradient(128, 128), gradient(128, 128), gradient(128, 128)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		if err := EncodeRGBA(&buf, frames, 0); err != nil {
			b.Fatal(err)
		}
	}
}
