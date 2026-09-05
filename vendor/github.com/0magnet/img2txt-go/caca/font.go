package caca

import (
	_ "embed"
	"encoding/binary"
	"errors"
)

// mono9Data is libcaca's built-in "Monospace 9" font, 4 bpp with 7x15 glyphs.
// It is the font caca_render_canvas uses for TGA export.
//
//go:embed mono9.caca
var mono9Data []byte

// fontHeader mirrors libcaca's struct font_header. All fields are big-endian
// in the file.
type fontHeader struct {
	controlSize, dataSize uint32
	version, blocks       uint16
	glyphs                uint32
	bpp, width, height    uint16
	maxwidth, maxheight   uint16
	flags                 uint16
}

// blockInfo maps a contiguous Unicode range onto glyph indices.
type blockInfo struct{ start, stop, index uint32 }

// glyphInfo describes one glyph's dimensions and data offset.
type glyphInfo struct {
	width, height uint16
	dataOffset    uint32
}

// Font is a loaded bitmap font.
type Font struct {
	header    fontHeader
	blockList []blockInfo
	glyphList []glyphInfo
	fontData  []byte
}

// ErrBadFont reports malformed font data.
var ErrBadFont = errors.New("caca: invalid font data")

// FontList returns the names of the built-in fonts.
func FontList() []string { return []string{"Monospace 9", "Monospace Bold 12"} }

// LoadFont parses a font from memory.
func LoadFont(data []byte) (*Font, error) {
	const headerSize = 28
	if len(data) < 4+headerSize {
		return nil, ErrBadFont
	}

	be := binary.BigEndian
	h := data[4:]
	f := &Font{}
	f.header = fontHeader{
		controlSize: be.Uint32(h[0:]),
		dataSize:    be.Uint32(h[4:]),
		version:     be.Uint16(h[8:]),
		blocks:      be.Uint16(h[10:]),
		glyphs:      be.Uint32(h[12:]),
		bpp:         be.Uint16(h[16:]),
		width:       be.Uint16(h[18:]),
		height:      be.Uint16(h[20:]),
		maxwidth:    be.Uint16(h[22:]),
		maxheight:   be.Uint16(h[24:]),
		flags:       be.Uint16(h[26:]),
	}

	if uint32(len(data)) != 4+f.header.controlSize+f.header.dataSize ||
		(f.header.bpp != 8 && f.header.bpp != 4 && f.header.bpp != 2 && f.header.bpp != 1) ||
		f.header.flags&1 == 0 {
		return nil, ErrBadFont
	}

	off := 4 + headerSize
	f.blockList = make([]blockInfo, f.header.blocks)
	for i := range f.blockList {
		if off+12 > len(data) {
			return nil, ErrBadFont
		}
		f.blockList[i] = blockInfo{
			start: be.Uint32(data[off:]),
			stop:  be.Uint32(data[off+4:]),
			index: be.Uint32(data[off+8:]),
		}
		b := f.blockList[i]
		if b.start > b.stop || (i > 0 && b.start < f.blockList[i-1].stop) ||
			b.index >= f.header.glyphs {
			return nil, ErrBadFont
		}
		off += 12
	}

	f.glyphList = make([]glyphInfo, f.header.glyphs)
	for i := range f.glyphList {
		if off+8 > len(data) {
			return nil, ErrBadFont
		}
		f.glyphList[i] = glyphInfo{
			width:      be.Uint16(data[off:]),
			height:     be.Uint16(data[off+2:]),
			dataOffset: be.Uint32(data[off+4:]),
		}
		g := f.glyphList[i]
		if g.dataOffset >= f.header.dataSize ||
			g.dataOffset+(uint32(g.width)*uint32(g.height)*uint32(f.header.bpp)+7)/8 > f.header.dataSize ||
			g.width > f.header.maxwidth || g.height > f.header.maxheight {
			return nil, ErrBadFont
		}
		off += 8
	}

	start := 4 + int(f.header.controlSize)
	if start > len(data) {
		return nil, ErrBadFont
	}
	f.fontData = data[start:]
	return f, nil
}

// Width returns the font's standard glyph width.
func (f *Font) Width() int { return int(f.header.width) }

// Height returns the font's standard glyph height.
func (f *Font) Height() int { return int(f.header.height) }

// unpackGlyph expands packed glyph data to one byte per pixel.
func unpackGlyph(dst, packed []byte, n int, bpp int) {
	perByte := 8 / bpp
	scale := 0xff / ((1 << bpp) - 1)
	for i := 0; i < n; i++ {
		idx := i / perByte
		if idx >= len(packed) {
			return
		}
		pixel := packed[idx]
		pixel >>= bpp * (perByte - 1 - (i % perByte))
		pixel %= 1 << bpp
		dst[i] = pixel * byte(scale)
	}
}

// RenderCanvas rasterises the canvas into a 32-bit ARGB buffer using f.
func (cv *Canvas) RenderCanvas(f *Font, buf []byte, width, height, pitch int) error {
	if width < 0 || height < 0 || pitch < 0 {
		return errors.New("caca: invalid render geometry")
	}

	var glyph []byte
	if f.header.bpp != 8 {
		glyph = make([]byte, int(f.header.width)*int(f.header.height)*2)
	}

	xmax := cv.Width
	if width < cv.Width*int(f.header.width) {
		xmax = width / int(f.header.width)
	}
	ymax := cv.Height
	if height < cv.Height*int(f.header.height) {
		ymax = height / int(f.header.height)
	}

	for y := 0; y < ymax; y++ {
		for x := 0; x < xmax; x++ {
			starty := y * int(f.header.height)
			startx := x * int(f.header.width)
			ch := uint32(cv.Chars[y*cv.Width+x])
			attr := cv.Attrs[y*cv.Width+x]

			// Find the Unicode block containing this glyph.
			b := 0
			for ; b < int(f.header.blocks); b++ {
				if ch < f.blockList[b].start {
					b = int(f.header.blocks)
					break
				}
				if ch < f.blockList[b].stop {
					break
				}
			}
			if b == int(f.header.blocks) {
				continue // glyph not in font
			}

			gi := f.blockList[b].index + ch - f.blockList[b].start
			if gi >= uint32(len(f.glyphList)) {
				continue
			}
			g := f.glyphList[gi]

			argb := attrToARGB64(attr)

			// Step 1: unpack the glyph.
			var src []byte
			if f.header.bpp == 8 {
				src = f.fontData[g.dataOffset:]
			} else {
				unpackGlyph(glyph, f.fontData[g.dataOffset:],
					int(g.width)*int(g.height), int(f.header.bpp))
				src = glyph
			}

			// Step 2: blend the glyph with the cell colors.
			for j := 0; j < int(g.height); j++ {
				lineOff := (starty+j)*pitch + 4*startx
				for i := 0; i < int(g.width); i++ {
					off := lineOff + 4*i
					if off < 0 || off+4 > len(buf) {
						continue
					}
					p := uint32(src[j*int(g.width)+i])
					q := uint32(0xff) - p
					for t := 0; t < 4; t++ {
						buf[off+t] = byte((q*uint32(argb[t]) + p*uint32(argb[4+t])) / 0xf)
					}
				}
			}
		}
	}
	return nil
}
