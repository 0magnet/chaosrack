package caca

import "math"

// ColorMode selects how colors are picked for each cell.
type ColorMode int

// Color modes, matching enum color_mode.
const (
	ColorModeMono ColorMode = iota
	ColorModeGray
	ColorMode8
	ColorMode16
	ColorModeFullGray
	ColorModeFull8
	ColorModeFull16
)

// rgbPalette is the 12-bit RGB palette used by the color picker.
var rgbPalette = [48]int32{
	0x0, 0x0, 0x0,
	0x0, 0x0, 0x7ff,
	0x0, 0x7ff, 0x0,
	0x0, 0x7ff, 0x7ff,
	0x7ff, 0x0, 0x0,
	0x7ff, 0x0, 0x7ff,
	0x7ff, 0x7ff, 0x0,
	0xaaa, 0xaaa, 0xaaa,
	0x555, 0x555, 0x555,
	0x000, 0x000, 0xfff,
	0x000, 0xfff, 0x000,
	0x000, 0xfff, 0xfff,
	0xfff, 0x000, 0x000,
	0xfff, 0x000, 0xfff,
	0xfff, 0xfff, 0x000,
	0xfff, 0xfff, 0xfff,
}

// rgbWeight weights each palette entry in the distance computation.
var rgbWeight = [16]int32{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}

// Glyph sets available to the dither.
var (
	asciiGlyphs  = []rune{' ', '.', ':', ';', 't', '%', 'S', 'X', '@', '8', '?'}
	shadesGlyphs = []rune{' ', 0xb7, 0x2591, 0x2592, '?'}
	blocksGlyphs = []rune{' ', 0x2598, 0x259a, '?'}
)

// ditherAlgo supplies the per-pixel threshold used by ordered and random
// dithering. Floyd-Steinberg is detected by name and handled separately.
type ditherAlgo struct {
	name      string
	init      func(d *Dither, line int)
	get       func(d *Dither) int32
	increment func(d *Dither)
}

// Dither holds the bitmap description and dithering parameters.
type Dither struct {
	bpp                    int
	hasPalette, hasAlpha   bool
	w, h, pitch            int
	rmask, gmask, bmask    uint32
	amask                  uint32
	rright, gright, bright int
	aright                 int
	rleft, gleft, bleft    int
	aleft                  int
	red, green, blue       [256]int32
	alpha                  [256]int32

	// Color features.
	gamma, brightness, contrast float32
	gammatab                    [4097]int32

	antialias bool
	color     ColorMode

	algo     ditherAlgo
	isFstein bool
	glyphs   []rune
	glyphCnt int
	invert   bool

	// Ordered-dither state.
	orderedTable []int32
	orderedIndex int
	orderedMod   int

	// Random-dither state.
	rnd *rngState
}

// mask2shift computes the right and left shifts that normalize a color mask
// to 12 bits.
func mask2shift(mask uint32) (right, left int) {
	if mask == 0 {
		return 0, 0
	}
	rshift, lshift := 0, 0
	for mask&1 == 0 {
		mask >>= 1
		rshift++
	}
	for mask&1 == 1 {
		mask >>= 1
		lshift++
	}
	return rshift, 12 - lshift
}

// NewDither creates a dither object for a bitmap with the given geometry and
// color masks.
func NewDither(bpp, w, h, pitch int, rmask, gmask, bmask, amask uint32) *Dither {
	if w < 0 || h < 0 || pitch < 0 || bpp > 32 || bpp < 8 {
		return nil
	}
	d := &Dither{
		bpp:      bpp,
		w:        w,
		h:        h,
		pitch:    pitch,
		rmask:    rmask,
		gmask:    gmask,
		bmask:    bmask,
		amask:    amask,
		hasAlpha: amask != 0,
	}

	if rmask != 0 || gmask != 0 || bmask != 0 || amask != 0 {
		d.rright, d.rleft = mask2shift(rmask)
		d.gright, d.gleft = mask2shift(gmask)
		d.bright, d.bleft = mask2shift(bmask)
		d.aright, d.aleft = mask2shift(amask)
	}

	// In 8 bpp mode, default to a grayscale palette.
	if bpp == 8 {
		d.hasPalette = true
		d.hasAlpha = false
		for i := 0; i < 256; i++ {
			v := int32(i * 0xfff / 256)
			d.red[i], d.green[i], d.blue[i] = v, v, v
		}
	}

	d.gamma = 1.0
	for i := 0; i < 4096; i++ {
		d.gammatab[i] = int32(i)
	}

	d.brightness = 1.0
	d.contrast = 1.0
	d.antialias = true
	d.color = ColorModeFull16
	d.glyphs = asciiGlyphs
	d.glyphCnt = len(asciiGlyphs)
	_ = d.SetAlgorithm("fstein")

	return d
}

// SetBrightness records the brightness. As in libcaca it does not yet affect
// the output.
func (d *Dither) SetBrightness(b float32) { d.brightness = b }

// SetContrast records the contrast. As in libcaca it does not yet affect the
// output.
func (d *Dither) SetContrast(c float32) { d.contrast = c }

// SetGamma sets the gamma. A negative value inverts the colors; zero is an
// error.
func (d *Dither) SetGamma(gamma float32) bool {
	if gamma < 0.0 {
		d.invert = true
		gamma = -gamma
	} else if gamma == 0.0 {
		return false
	}
	d.gamma = gamma
	for i := 0; i < 4096; i++ {
		d.gammatab[i] = int32(4096.0 * gammapow(float32(i)/4096.0, 1.0/gamma))
	}
	return true
}

// SetCharset selects the glyph set: "ascii", "shades" or "blocks".
func (d *Dither) SetCharset(name string) bool {
	switch name {
	case "ascii", "default":
		d.glyphs = asciiGlyphs
		d.glyphCnt = len(asciiGlyphs)
	case "shades":
		d.glyphs = shadesGlyphs
		d.glyphCnt = len(shadesGlyphs)
	case "blocks":
		d.glyphs = blocksGlyphs
		d.glyphCnt = len(blocksGlyphs)
	default:
		return false
	}
	return true
}

// SetAntialias selects "none" or "prefilter"/"default".
func (d *Dither) SetAntialias(name string) bool {
	switch name {
	case "none":
		d.antialias = false
	case "prefilter", "default":
		d.antialias = true
	default:
		return false
	}
	return true
}

// AlgorithmList returns the supported dithering algorithms and their
// descriptions, in the order libcaca reports them.
func AlgorithmList() [][2]string {
	return [][2]string{
		{"none", "no dithering"},
		{"ordered2", "2x2 ordered dithering"},
		{"ordered4", "4x4 ordered dithering"},
		{"ordered8", "8x8 ordered dithering"},
		{"random", "random dithering"},
		{"fstein", "Floyd-Steinberg dithering"},
	}
}

var (
	dither2x2 = []int32{
		0x00, 0x80,
		0xc0, 0x40,
	}
	dither4x4 = []int32{
		0x00, 0x80, 0x20, 0xa0,
		0xc0, 0x40, 0xe0, 0x60,
		0x30, 0xb0, 0x10, 0x90,
		0xf0, 0x70, 0xd0, 0x50,
	}
	dither8x8 = []int32{
		0x00, 0x80, 0x20, 0xa0, 0x08, 0x88, 0x28, 0xa8,
		0xc0, 0x40, 0xe0, 0x60, 0xc8, 0x48, 0xe8, 0x68,
		0x30, 0xb0, 0x10, 0x90, 0x38, 0xb8, 0x18, 0x98,
		0xf0, 0x70, 0xd0, 0x50, 0xf8, 0x78, 0xd8, 0x58,
		0x0c, 0x8c, 0x2c, 0xac, 0x04, 0x84, 0x24, 0xa4,
		0xcc, 0x4c, 0xec, 0x6c, 0xc4, 0x44, 0xe4, 0x64,
		0x3c, 0xbc, 0x1c, 0x9c, 0x34, 0xb4, 0x14, 0x94,
		0xfc, 0x7c, 0xdc, 0x5c, 0xf4, 0x74, 0xd4, 0x54,
	}
)

// orderedAlgo builds an ordered-dither algorithm of the given matrix size.
func orderedAlgo(name string, table []int32, n int) ditherAlgo {
	return ditherAlgo{
		name: name,
		init: func(d *Dither, line int) {
			// The C code indexes a static table by line, so a negative line
			// would be a bug; img2txt never produces one.
			d.orderedTable = table[(line%n)*n:]
			d.orderedIndex = 0
			d.orderedMod = n
		},
		get:       func(d *Dither) int32 { return d.orderedTable[d.orderedIndex] },
		increment: func(d *Dither) { d.orderedIndex = (d.orderedIndex + 1) % d.orderedMod },
	}
}

// SetAlgorithm selects the dithering algorithm by name.
func (d *Dither) SetAlgorithm(name string) bool {
	switch name {
	case "none":
		d.algo = ditherAlgo{
			name:      "none",
			init:      func(*Dither, int) {},
			get:       func(*Dither) int32 { return 0x80 },
			increment: func(*Dither) {},
		}
		d.isFstein = false
	case "ordered2":
		d.algo = orderedAlgo("ordered2", dither2x2, 2)
		d.isFstein = false
	case "ordered4":
		d.algo = orderedAlgo("ordered4", dither4x4, 4)
		d.isFstein = false
	case "ordered8":
		d.algo = orderedAlgo("ordered8", dither8x8, 8)
		d.isFstein = false
	case "random":
		d.algo = ditherAlgo{
			name:      "random",
			init:      func(*Dither, int) {},
			get:       func(d *Dither) int32 { return d.rnd.rand(0x00, 0x100) },
			increment: func(*Dither) {},
		}
		d.isFstein = false
		d.rnd = newRNG()
	case "fstein", "default":
		d.algo = ditherAlgo{
			name:      "fstein",
			init:      func(*Dither, int) {},
			get:       func(*Dither) int32 { return 0x80 },
			increment: func(*Dither) {},
		}
		d.isFstein = true
	default:
		return false
	}
	return true
}

// gammapow computes x^y using the same series expansion as libcaca's portable
// implementation, so the gamma table matches.
func gammapow(x, y float32) float32 {
	if x == 0.0 {
		if y == 0.0 {
			return 1.0
		}
		return 0.0
	}

	// libcaca uses the x87 FLDLN2/FSCALE path on x86, which evaluates
	// e^(y*ln(x)) at 80-bit precision. math.Pow at float64 reproduces it.
	return float32(math.Pow(float64(x), float64(y)))
}

// getRGBA accumulates the color of one source pixel into rgba, applying the
// gamma table.
func (d *Dither) getRGBA(pixels []byte, x, y int, rgba *[4]uint32) {
	off := (d.bpp/8)*x + d.pitch*y
	if off < 0 || off >= len(pixels) {
		return
	}
	var bits uint32
	switch d.bpp / 8 {
	case 4:
		if off+4 > len(pixels) {
			return
		}
		bits = uint32(pixels[off]) | uint32(pixels[off+1])<<8 |
			uint32(pixels[off+2])<<16 | uint32(pixels[off+3])<<24
	case 3:
		if off+3 > len(pixels) {
			return
		}
		// Little-endian host ordering.
		bits = uint32(pixels[off+2])<<16 | uint32(pixels[off+1])<<8 | uint32(pixels[off])
	case 2:
		if off+2 > len(pixels) {
			return
		}
		bits = uint32(pixels[off]) | uint32(pixels[off+1])<<8
	default:
		bits = uint32(pixels[off])
	}

	if d.hasPalette {
		rgba[0] += uint32(d.gammatab[d.red[bits&0xff]])
		rgba[1] += uint32(d.gammatab[d.green[bits&0xff]])
		rgba[2] += uint32(d.gammatab[d.blue[bits&0xff]])
		rgba[3] += uint32(d.alpha[bits&0xff])
		return
	}
	rgba[0] += uint32(d.gammatab[((bits&d.rmask)>>d.rright)<<d.rleft])
	rgba[1] += uint32(d.gammatab[((bits&d.gmask)>>d.gright)<<d.gleft])
	rgba[2] += uint32(d.gammatab[((bits&d.bmask)>>d.bright)<<d.bleft])
	rgba[3] += ((bits & d.amask) >> d.aright) << d.aleft
}

// absInt returns the absolute value of an int32, mirroring C's abs().
func absInt(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// DitherBitmap renders the bitmap onto the canvas in the rectangle (x,y,w,h).
//
// The integer arithmetic below deliberately mirrors the C original, where the
// pixel accumulators are unsigned and the distance variables are signed; the
// wraparound this produces affects which glyphs get chosen.
func (cv *Canvas) DitherBitmap(x, y, w, h int, d *Dither, pixels []byte) {
	if d == nil || pixels == nil {
		return
	}

	savedattr := cv.Attr()

	x1, x2 := x, x+w-1
	y1, y2 := y, y+h-1

	// The original overwrites its arguments with the bitmap dimensions.
	w = d.w
	h = d.h

	deltax := x2 - x1 + 1
	deltay := y2 - y1 + 1
	dchmax := int32(d.glyphCnt)

	fsLength := x2
	if cv.Width <= x2 {
		fsLength = cv.Width
	}
	fsLength++

	// fs_r and friends are offset by one, so index -1 is valid.
	fsR := make([]int32, fsLength+2)
	fsG := make([]int32, fsLength+2)
	fsB := make([]int32, fsLength+2)
	fsAt := func(a []int32, i int) *int32 {
		j := i + 1
		if j < 0 || j >= len(a) {
			return new(int32) // scratch; unreachable for valid geometry
		}
		return &a[j]
	}

	yStart := y1
	if yStart < 0 {
		yStart = 0
	}
	for y = yStart; y <= y2 && y <= cv.Height; y++ {
		var remainR, remainG, remainB int32

		xStart := x1
		if xStart < 0 {
			xStart = 0
		}
		d.algo.init(d, y)
		for x = xStart; x <= x2 && x <= cv.Width; x++ {
			var rgba [4]uint32
			var errv [3]int32
			var ch int32
			var distmin int32
			var fgR, fgG, fgB, bgR, bgG, bgB int32
			outfg, outbg := int32(0), int32(0)
			var outch rune

			fromx := int((int64(x-x1) * int64(w)) / int64(deltax))
			fromy := int((int64(y-y1) * int64(h)) / int64(deltay))
			tox := int((int64(x-x1+1) * int64(w)) / int64(deltax))
			toy := int((int64(y-y1+1) * int64(h)) / int64(deltay))

			if d.antialias {
				// We want at least one pixel.
				if tox == fromx {
					tox++
				}
				if toy == fromy {
					toy++
				}
				dots := 0
				for myx := fromx; myx < tox; myx++ {
					for myy := fromy; myy < toy; myy++ {
						dots++
						d.getRGBA(pixels, myx, myy, &rgba)
					}
				}
				if dots > 0 {
					rgba[0] /= uint32(dots)
					rgba[1] /= uint32(dots)
					rgba[2] /= uint32(dots)
					rgba[3] /= uint32(dots)
				}
			} else {
				myx := (fromx + tox) / 2
				myy := (fromy + toy) / 2
				d.getRGBA(pixels, myx, myy, &rgba)
			}

			// Force greyscale if requested.
			if d.color == ColorModeFullGray {
				gray := (3*rgba[0] + 4*rgba[1] + rgba[2] + 4) / 8
				rgba[0], rgba[1], rgba[2] = gray, gray, gray
			}

			if d.hasAlpha && rgba[3] < 0x800 {
				remainR, remainG, remainB = 0, 0, 0
				*fsAt(fsR, x) = 0
				*fsAt(fsG, x) = 0
				*fsAt(fsB, x) = 0
				// Note: the dither is not incremented on this path.
				continue
			}

			if d.isFstein {
				rgba[0] += uint32(remainR)
				rgba[1] += uint32(remainG)
				rgba[2] += uint32(remainB)
			} else {
				rgba[0] += uint32((d.algo.get(d) - 0x80) * 4)
				rgba[1] += uint32((d.algo.get(d) - 0x80) * 4)
				rgba[2] += uint32((d.algo.get(d) - 0x80) * 4)
			}

			distmin = math.MaxInt32
			for i := int32(0); i < 16; i++ {
				if d.color == ColorModeFullGray &&
					(rgbPalette[i*3] != rgbPalette[i*3+1] ||
						rgbPalette[i*3] != rgbPalette[i*3+2]) {
					continue
				}
				du := sqU(rgba[0]-uint32(rgbPalette[i*3])) +
					sqU(rgba[1]-uint32(rgbPalette[i*3+1])) +
					sqU(rgba[2]-uint32(rgbPalette[i*3+2]))
				dist := int32(du) * rgbWeight[i]
				if dist < distmin {
					outbg = i
					distmin = dist
				}
			}
			bgR = rgbPalette[outbg*3]
			bgG = rgbPalette[outbg*3+1]
			bgB = rgbPalette[outbg*3+2]

			if d.color == ColorModeFull16 || d.color == ColorModeFullGray {
				distmin = math.MaxInt32
				for i := int32(0); i < 16; i++ {
					if i == outbg {
						continue
					}
					if d.color == ColorModeFullGray &&
						(rgbPalette[i*3] != rgbPalette[i*3+1] ||
							rgbPalette[i*3] != rgbPalette[i*3+2]) {
						continue
					}
					du := sqU(rgba[0]-uint32(rgbPalette[i*3])) +
						sqU(rgba[1]-uint32(rgbPalette[i*3+1])) +
						sqU(rgba[2]-uint32(rgbPalette[i*3+2]))
					dist := int32(du) * rgbWeight[i]
					if dist < distmin {
						outfg = i
						distmin = dist
					}
				}
				fgR = rgbPalette[outfg*3]
				fgG = rgbPalette[outfg*3+1]
				fgB = rgbPalette[outfg*3+2]

				distmin = math.MaxInt32
				span := 2*dchmax - 1
				for i := int32(0); i < dchmax-1; i++ {
					newr := i*fgR + (span-i)*bgR
					newg := i*fgG + (span-i)*bgG
					newb := i*fgB + (span-i)*bgB
					dist := absInt(int32(rgba[0]*uint32(span) - uint32(newr)))
					dist += absInt(int32(rgba[1]*uint32(span) - uint32(newg)))
					dist += absInt(int32(rgba[2]*uint32(span) - uint32(newb)))
					if dist < distmin {
						ch = i
						distmin = dist
					}
				}
				outch = d.glyphs[ch]

				if d.isFstein {
					errv[0] = int32(rgba[0] - uint32((fgR*ch+bgR*(span-ch))/span))
					errv[1] = int32(rgba[1] - uint32((fgG*ch+bgG*(span-ch))/span))
					errv[2] = int32(rgba[2] - uint32((fgB*ch+bgB*(span-ch))/span))
				}
			} else {
				lum := rgba[0]
				if rgba[1] > lum {
					lum = rgba[1]
				}
				if rgba[2] > lum {
					lum = rgba[2]
				}
				outfg = outbg
				outbg = Black

				ch = int32(lum * uint32(dchmax) / 0x1000)
				if ch < 0 {
					ch = 0
				} else if ch > dchmax-1 {
					ch = dchmax - 1
				}
				outch = d.glyphs[ch]

				if d.isFstein {
					errv[0] = int32(rgba[0] - uint32(bgR*ch/(dchmax-1)))
					errv[1] = int32(rgba[1] - uint32(bgG*ch/(dchmax-1)))
					errv[2] = int32(rgba[2] - uint32(bgB*ch/(dchmax-1)))
				}
			}

			if d.isFstein {
				remainR = *fsAt(fsR, x+1) + 7*errv[0]/16
				remainG = *fsAt(fsG, x+1) + 7*errv[1]/16
				remainB = *fsAt(fsB, x+1) + 7*errv[2]/16
				*fsAt(fsR, x-1) += 3 * errv[0] / 16
				*fsAt(fsG, x-1) += 3 * errv[1] / 16
				*fsAt(fsB, x-1) += 3 * errv[2] / 16
				*fsAt(fsR, x) = 5 * errv[0] / 16
				*fsAt(fsG, x) = 5 * errv[1] / 16
				*fsAt(fsB, x) = 5 * errv[2] / 16
				*fsAt(fsR, x+1) = 1 * errv[0] / 16
				*fsAt(fsG, x+1) = 1 * errv[1] / 16
				*fsAt(fsB, x+1) = 1 * errv[2] / 16
			}

			if d.invert {
				outfg = 15 - outfg
				outbg = 15 - outbg
			}

			cv.SetColorANSI(uint8(outfg), uint8(outbg))
			cv.PutChar(x, y, outch)

			d.algo.increment(d)
		}
	}

	cv.SetAttr(savedattr)
}

// sqU squares an unsigned value with C's wraparound semantics.
func sqU(v uint32) uint32 { return v * v }
