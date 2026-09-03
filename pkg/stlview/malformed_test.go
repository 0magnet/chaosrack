package stlview

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// A file picked by a person can be anything at all: truncated, mislabelled,
// hostile, or simply not an STL. None of it may take the page down, because a
// panic in wasm is a blank tab rather than a rejected file.
func TestNewSTLSurvivesRubbish(t *testing.T) {
	bin := func(count uint32, body int) []byte {
		b := make([]byte, 80)
		copy(b, "not-solid binary header")
		c := make([]byte, 4)
		binary.LittleEndian.PutUint32(c, count)
		return append(append(b, c...), bytes.Repeat([]byte{0x7f}, body)...)
	}
	nan := func() []byte {
		b := bin(1, 0)
		tri := make([]byte, 50)
		for i := 0; i < 12; i++ {
			binary.LittleEndian.PutUint32(tri[i*4:], math.Float32bits(float32(math.NaN())))
		}
		return append(b, tri...)
	}

	cases := map[string][]byte{
		"empty":                       {},
		"one byte":                    {0x00},
		"header only":                 make([]byte, 80),
		"header plus count":           bin(0, 0),
		"count 1 no body":             bin(1, 0),
		"count max":                   bin(math.MaxUint32, 0),
		"count max with body":         bin(math.MaxUint32, 500),
		"count 1 short body":          bin(1, 49),
		"ascii empty solid":           []byte("solid\nendsolid\n"),
		"ascii truncated":             []byte("solid x\n facet normal 0 0 1\n  outer loop\n"),
		"ascii junk numbers":          []byte("solid x\n facet normal a b c\n  outer loop\n   vertex q w e\n  endloop\n endfacet\nendsolid\n"),
		"ascii says solid but binary": append([]byte("solid "), bytes.Repeat([]byte{0xff}, 200)...),
		"nan vertices":                nan(),
		"all zero 1k":                 make([]byte, 1024),
		"text file":                   []byte("this is a shopping list\nmilk\neggs\n"),
	}

	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked on %q: %v", name, r)
				}
			}()
			s, err := NewSTL(in)
			if err != nil {
				return // a refusal is the right answer for most of these
			}
			// Parsed: the model it hands back has to be self-consistent, since
			// the caller uploads these straight into GL buffers.
			v, n, idx := s.GetModel()
			if len(v)%3 != 0 || len(n)%3 != 0 {
				t.Errorf("%q: ragged vertex/normal arrays: %d verts, %d normals", name, len(v), len(n))
			}
			// Every index has to address a real vertex. An index past the end
			// is not a bad picture, it is a read past the end of a GL buffer.
			for _, i := range idx {
				if int(i)*3+2 >= len(v) {
					t.Fatalf("%q: index %d needs vertex floats up to %d, but there are %d",
						name, i, int(i)*3+2, len(v))
				}
			}
		})
	}
}
