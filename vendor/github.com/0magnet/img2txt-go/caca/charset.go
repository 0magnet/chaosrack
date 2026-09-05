package caca

import "math/rand"

// uch returns a character as the unsigned value libcaca compares.
//
// A canvas cell holds a rune, which is signed, but libcaca's is a uint32 and
// its decoder happily produces code points above 0x7FFFFFFF from malformed
// UTF-8. Ordered comparisons on such a cell have to be made unsigned or they
// come out backwards.
func uch(ch rune) uint32 { return uint32(ch) }

// UTF32ToCP437 converts a Unicode code point to its CP437 byte, or '?' when it
// has no CP437 representation.
func UTF32ToCP437(ch rune) byte {
	if uch(ch) < 0x00000020 {
		return '?'
	}
	if uch(ch) < 0x00000080 {
		return byte(ch)
	}
	for i, r := range cp437Lookup1 {
		if r == ch {
			return byte(0x01 + i)
		}
	}
	for i, r := range cp437Lookup2 {
		if r == ch {
			return byte(0x7f + i)
		}
	}
	return '?'
}

// CP437ToUTF32 converts a CP437 byte to Unicode, or zero for control codes
// with no representation.
func CP437ToUTF32(ch byte) rune {
	if ch > 0x7f {
		return cp437Lookup2[int(ch)-0x7f]
	}
	if ch >= 0x20 {
		return rune(ch)
	}
	if ch > 0 {
		return cp437Lookup1[int(ch)-1]
	}
	return 0
}

// UTF32ToUTF8 appends the UTF-8 encoding of ch to dst.
//
// This is caca_utf32_to_utf8(), which stops at four bytes. A code point above
// U+1FFFFF therefore does not get the historical five or six byte form: it is
// encoded as four bytes whose lead byte has overflowed, which is what the C
// library writes and what its own decoder reads back.
func UTF32ToUTF8(dst []byte, ch rune) []byte {
	u := uint32(ch)
	if u < 0x80 {
		return append(dst, byte(u))
	}

	mark := [7]byte{0x00, 0x00, 0xc0, 0xe0, 0xf0, 0xf8, 0xfc}

	n := 4
	switch {
	case u < 0x800:
		n = 2
	case u < 0x10000:
		n = 3
	}

	var buf [4]byte
	for i := n - 1; i > 0; i-- {
		buf[i] = byte(u|0x80) & 0xbf
		u >>= 6
	}
	buf[0] = byte(u) | mark[n]

	return append(dst, buf[:n]...)
}

// IsFullwidth reports whether a code point occupies two cells.
//
// These are libcaca's own ranges, which are coarser than the Unicode East Asian
// Width property: everything from U+2E80 to U+A6FF counts, Hangul Jamo below
// U+2E80 does not, and the supplementary planes are fullwidth as far as
// U+DFFFF. Anything finer would disagree with the C library.
func IsFullwidth(ch rune) bool {
	u := uch(ch)
	switch {
	case u < 0x2e80: // standard stuff
		return false
	case u < 0xa700: // Japanese, Korean, CJK, Yi...
		return true
	case u < 0xac00: // modified tone letters, Syloti Nagri
		return false
	case u < 0xd800: // Hangul syllables
		return true
	case u < 0xf900:
		return false
	case u < 0xfb00: // more CJK
		return true
	case u < 0xfe20:
		return false
	case u < 0xfe70: // more CJK
		return true
	case u < 0xff00:
		return false
	case u < 0xff61: // fullwidth forms
		return true
	case u < 0xffe0: // halfwidth forms
		return false
	case u < 0xffe8: // more fullwidth forms
		return true
	case u < 0x20000:
		return false
	case u < 0xe0000: // more CJK
		return true
	default:
		return false
	}
}

// rngState backs the "random" dithering algorithm. libcaca seeds its generator
// from the process id and a timer, so random dithering is not reproducible
// there either.
type rngState struct{ r *rand.Rand }

func newRNG() *rngState {
	return &rngState{r: rand.New(rand.NewSource(rand.Int63()))}
}

// rand returns a value in [min, max).
func (s *rngState) rand(min, max int32) int32 {
	if max <= min {
		return min
	}
	return min + int32(float64(max-min)*s.r.Float64())
}
