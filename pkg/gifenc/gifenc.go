// Package gifenc turns a sequence of RGBA frames into an animated GIF.
//
// Untagged, so the same code runs in two places that must agree: `uitool gifs`
// makes the README gallery on a machine, and the in-app recorder makes clips in
// the browser. A GIF recorded from the app should look like the ones in the
// README, and the only way to be sure of that is for it to be the same encoder
// rather than a second one written to the same description.
//
// GIF is 256 colors with no alpha, which is a poor fit for smooth gradients —
// so the palette is built from the frames themselves rather than taken from a
// fixed table. See AdaptivePalette for why, and Palettize for why there is no
// dithering.
package gifenc

import (
	"image"
	"image/color"
	"image/gif"
	"io"
	"sort"
)

// DefaultDelay is the frame delay in hundredths of a second — about 8 frames a
// second, which is where a GIF of a moving figure stops looking like a
// slideshow without the file size running away.
const DefaultDelay = 12

// AdaptivePalette builds a 256-color palette from the frames themselves, by
// popularity over a 4-bit-per-channel histogram.
//
// A fixed palette is the obvious alternative and a bad one here: the content is
// smooth gradients over black, and the web-safe palette bands them into
// visible steps. Taking the colors from the frames means the palette is spent
// on the colors that are actually present.
//
// Every other pixel is sampled. The histogram is for ranking, not for accuracy,
// and halving the work halves the time a recording takes to encode.
func AdaptivePalette(frames []*image.RGBA) color.Palette {
	type bucket struct {
		n       uint64
		r, g, b uint64
	}
	hist := map[uint32]*bucket{}
	for _, f := range frames {
		p := f.Pix
		for i := 0; i+2 < len(p); i += 8 { // every other pixel
			r, g, b := uint32(p[i]), uint32(p[i+1]), uint32(p[i+2])
			key := (r>>4)<<8 | (g>>4)<<4 | (b >> 4)
			bk := hist[key]
			if bk == nil {
				bk = &bucket{}
				hist[key] = bk
			}
			bk.n++
			bk.r += uint64(r)
			bk.g += uint64(g)
			bk.b += uint64(b)
		}
	}
	type kb struct {
		k uint32
		b *bucket
	}
	all := make([]kb, 0, len(hist))
	for k, b := range hist {
		all = append(all, kb{k, b})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].b.n != all[j].b.n {
			return all[i].b.n > all[j].b.n
		}
		return all[i].k < all[j].k // stable for equal counts, so runs reproduce
	})
	// Black first and always: it is the background, it is most of the frame,
	// and a palette that had to earn it could lose it on a bright clip.
	pal := color.Palette{color.RGBA{0, 0, 0, 255}}
	for _, e := range all {
		if len(pal) >= 256 {
			break
		}
		b := e.b
		pal = append(pal, color.RGBA{uint8(b.r / b.n), uint8(b.g / b.n), uint8(b.b / b.n), 255}) //nolint:gosec // a channel average and a palette index; both are 0..255 by construction
	}
	return pal
}

// Palettize converts a frame with nearest-color matching and no dithering.
//
// Floyd–Steinberg is the usual advice and it is wrong for this content: the
// background is a large flat black field, and dithering speckles it with
// isolated lit pixels that read as noise and cost a great deal of GIF size,
// since the format compresses runs.
//
// The memo is what makes it fast. Palette.Index is a linear scan over 256
// colors, and these frames hold few distinct ones, so caching the answer per
// source color turns a per-pixel scan into a map lookup. Pass the same memo
// across every frame of a clip — the palette is the same for all of them.
func Palettize(src *image.RGBA, pal color.Palette, memo map[uint32]uint8) *image.Paletted {
	b := src.Bounds()
	out := image.NewPaletted(b, pal)
	for y := 0; y < b.Dy(); y++ {
		si := src.PixOffset(b.Min.X, b.Min.Y+y)
		oi := out.PixOffset(b.Min.X, b.Min.Y+y)
		for x := 0; x < b.Dx(); x++ {
			r, g, bb := src.Pix[si], src.Pix[si+1], src.Pix[si+2]
			key := uint32(r)<<16 | uint32(g)<<8 | uint32(bb)
			idx, ok := memo[key]
			if !ok {
				idx = uint8(pal.Index(color.RGBA{r, g, bb, 255})) //nolint:gosec // a channel average and a palette index; both are 0..255 by construction
				memo[key] = idx
			}
			out.Pix[oi] = idx
			si += 4
			oi++
		}
	}
	return out
}

// Encode writes the frames as one looping GIF at the given delay (hundredths
// of a second; zero uses DefaultDelay).
func Encode(w io.Writer, frames []*image.Paletted, delay int) error {
	if delay <= 0 {
		delay = DefaultDelay
	}
	g := &gif.GIF{LoopCount: 0}
	for _, f := range frames {
		g.Image = append(g.Image, f)
		g.Delay = append(g.Delay, delay)
	}
	return gif.EncodeAll(w, g)
}

// EncodeRGBA is the whole job for callers that have raw frames: build the
// palette from them, convert each, and write the GIF.
func EncodeRGBA(w io.Writer, frames []*image.RGBA, delay int) error {
	if len(frames) == 0 {
		return nil
	}
	pal := AdaptivePalette(frames)
	memo := make(map[uint32]uint8, 4096)
	out := make([]*image.Paletted, 0, len(frames))
	for _, f := range frames {
		out = append(out, Palettize(f, pal, memo))
	}
	return Encode(w, out, delay)
}
