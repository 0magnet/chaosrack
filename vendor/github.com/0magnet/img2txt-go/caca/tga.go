package caca

// exportTGA rasterises the canvas with libcaca's built-in "Monospace 9" font
// and wraps it in an uncompressed 32-bit TGA.
func (cv *Canvas) exportTGA() ([]byte, bool) {
	f, err := LoadFont(mono9Data)
	if err != nil {
		return nil, false
	}

	w := cv.Width * f.Width()
	h := cv.Height * f.Height()

	b := make([]byte, 18+w*h*4)

	b[0] = 0 // ID length
	b[1] = 0 // color map type: none
	b[2] = 2 // image type: uncompressed truecolor
	// b[3:8] color map specification: none
	// b[8:10] X origin, b[10:12] Y origin: zero
	b[12] = byte(w & 0xff)
	b[13] = byte(w >> 8)
	b[14] = byte(h & 0xff)
	b[15] = byte(h >> 8)
	b[16] = 32 // pixel depth
	b[17] = 40 // image descriptor

	pix := b[18:]
	if err := cv.RenderCanvas(f, pix, w, h, 4*w); err != nil {
		return nil, false
	}

	// The renderer emits ARGB; TGA wants BGRA.
	for i := 0; i+4 <= len(pix); i += 4 {
		pix[i], pix[i+3] = pix[i+3], pix[i]
		pix[i+1], pix[i+2] = pix[i+2], pix[i+1]
	}

	return b, true
}
