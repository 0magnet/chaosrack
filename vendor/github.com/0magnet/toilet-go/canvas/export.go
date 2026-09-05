package canvas

import "encoding/binary"

// Export renders the canvas in one of libcaca's formats.
//
// Every format but "caca" is the img2txt-go codec. The native one is written
// here instead, because it is the only format that records the frame cursor and
// handle, which that port has no way to carry and which the rotate filters
// change.
func (cv *Canvas) Export(format string) ([]byte, bool) {
	if len(format) == 4 &&
		(format[0]|0x20) == 'c' && (format[1]|0x20) == 'a' &&
		(format[2]|0x20) == 'c' && (format[3]|0x20) == 'a' {
		return cv.exportCaca(), true
	}
	return cv.Canvas.Export(format)
}

// exportCaca writes the native libcaca canvas format: a magic number, a canvas
// header, one frame info block and eight bytes per cell.
func (cv *Canvas) exportCaca() []byte {
	const framecount = 1

	b := make([]byte, 0, 20+32+8*cv.Width*cv.Height)
	b = append(b, "\xCA\xCA"+"CV"...)

	u32 := func(v uint32) {
		b = binary.BigEndian.AppendUint32(b, v)
	}
	u16 := func(v uint16) {
		b = binary.BigEndian.AppendUint16(b, v)
	}

	// canvas_header
	u32(16 + 32*framecount)
	u32(uint32(cv.Width) * uint32(cv.Height) * 8 * framecount)
	u16(0x0001)
	u32(framecount)
	u16(0x0000)

	// frame_info
	u32(uint32(cv.Width))
	u32(uint32(cv.Height))
	u32(0)
	u32(cv.Attr())
	u32(uint32(cv.X))
	u32(uint32(cv.Y))
	u32(uint32(cv.HandleX))
	u32(uint32(cv.HandleY))

	// canvas_data
	for n := 0; n < cv.Width*cv.Height; n++ {
		u32(uint32(cv.Chars[n]))
		u32(cv.Attrs[n])
	}

	return b
}
