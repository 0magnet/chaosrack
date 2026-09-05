package figlet

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
)

// lineReader reads a font file line by line the way libcaca's caca_file_gets()
// does: a line is everything up to and including the next newline, but never
// more than one byte short of the caller's buffer.
type lineReader struct {
	data []byte
	pos  int
}

// gets returns the next line, or false at end of file.
func (r *lineReader) gets(size int) ([]byte, bool) {
	if r.pos >= len(r.data) {
		return nil, false
	}

	limit := r.pos + size - 1
	if limit > len(r.data) {
		limit = len(r.data)
	}

	end := r.pos
	for end < limit && r.data[end] != '\n' {
		end++
	}
	if end < limit && r.data[end] == '\n' {
		end++
	}

	line := r.data[r.pos:end]
	r.pos = end
	return line, true
}

// eof reports whether the reader has consumed all its input.
func (r *lineReader) eof() bool { return r.pos >= len(r.data) }

// maxFontSize caps the size of a decompressed font, so that a malformed or
// hostile archive cannot exhaust memory.
const maxFontSize = 64 << 20

// decompress returns the font data, transparently unpacking the two container
// formats libcaca's caca_file_open() understands. A ZIP is handled the way
// libcaca handles it: skip the fixed 30-byte local file header plus the
// variable filename and extra fields, then inflate a raw deflate stream. The
// central directory is not consulted, so only the first member is readable.
func decompress(data []byte) ([]byte, error) {
	switch {
	case len(data) >= 4 && bytes.Equal(data[:4], []byte("PK\x03\x04")):
		if len(data) < 30 {
			return nil, fmt.Errorf("%w: truncated zip header", ErrBadFont)
		}
		nameLen := int(data[26]) | int(data[27])<<8
		extraLen := int(data[28]) | int(data[29])<<8
		off := 30 + nameLen + extraLen
		if off > len(data) {
			return nil, fmt.Errorf("%w: truncated zip header", ErrBadFont)
		}

		zr := flate.NewReader(bytes.NewReader(data[off:]))
		defer func() { _ = zr.Close() }()
		out, err := io.ReadAll(io.LimitReader(zr, maxFontSize))
		if err != nil && len(out) == 0 {
			return nil, fmt.Errorf("%w: %v", ErrBadFont, err)
		}
		return out, nil

	case len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b:
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBadFont, err)
		}
		defer func() { _ = gz.Close() }()
		out, err := io.ReadAll(io.LimitReader(gz, maxFontSize))
		if err != nil && len(out) == 0 {
			return nil, fmt.Errorf("%w: %v", ErrBadFont, err)
		}
		return out, nil
	}

	return data, nil
}

// scanner reproduces just enough of C's sscanf to read a FIGfont header the way
// libcaca reads it.
type scanner struct {
	b []byte
	i int
}

// skipSpace advances past ASCII whitespace, as every sscanf conversion but
// "%[" does.
func (s *scanner) skipSpace() {
	for s.i < len(s.b) {
		switch s.b[s.i] {
		case ' ', '\t', '\n', '\v', '\f', '\r':
			s.i++
		default:
			return
		}
	}
}

// runOf consumes one or more bytes drawn from set, as "%[set]" does. It fails
// if the first byte is not in the set.
func (s *scanner) runOf(set string) bool {
	start := s.i
	for s.i < len(s.b) && bytes.IndexByte([]byte(set), s.b[s.i]) >= 0 {
		s.i++
	}
	return s.i > start
}

// literal matches a fixed string in the format.
func (s *scanner) literal(lit string) bool {
	if s.i+len(lit) > len(s.b) || string(s.b[s.i:s.i+len(lit)]) != lit {
		return false
	}
	s.i += len(lit)
	return true
}

// word consumes up to max non-whitespace bytes, as "%<max>s" does.
func (s *scanner) word(max int) ([]byte, bool) {
	s.skipSpace()
	start := s.i
	for s.i < len(s.b) && s.i-start < max {
		switch s.b[s.i] {
		case ' ', '\t', '\n', '\v', '\f', '\r':
			return s.b[start:s.i], true
		}
		s.i++
	}
	if s.i == start {
		return nil, false
	}
	return s.b[start:s.i], true
}

// number reads an integer. With prefixed set it behaves like "%i", honoring a
// 0x or leading-zero base prefix; otherwise it is plain decimal, which is what
// "%u" accepts here.
func (s *scanner) number(prefixed bool) (int, bool) {
	s.skipSpace()

	neg := false
	if s.i < len(s.b) && (s.b[s.i] == '-' || s.b[s.i] == '+') {
		neg = s.b[s.i] == '-'
		s.i++
	}

	base := 10
	if prefixed && s.i < len(s.b) && s.b[s.i] == '0' {
		if s.i+1 < len(s.b) && (s.b[s.i+1] == 'x' || s.b[s.i+1] == 'X') {
			base = 16
			s.i += 2
		} else {
			base = 8
		}
	}

	v, ok := s.digits(base)
	if !ok {
		return 0, false
	}
	if neg {
		return -v, true
	}
	return v, true
}

// digits reads a run of digits in the given base.
func (s *scanner) digits(base int) (int, bool) {
	start := s.i
	v := 0
	for s.i < len(s.b) {
		d := digitValue(s.b[s.i])
		if d < 0 || d >= base {
			break
		}
		v = v*base + d
		s.i++
		if v > 0x7fffffff {
			return 0, false
		}
	}
	return v, s.i > start
}

// digitValue returns the numeric value of a digit byte, or -1.
func digitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}
