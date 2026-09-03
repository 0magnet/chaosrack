package stlview

import (
	"encoding/base64"
	"encoding/binary"
	"math"
	"strings"
	"testing"
)

// ParseBase64 takes what a file input hands the page: a data URL whose payload
// starts after the "base64," marker.
func TestParseBase64ReadsADataURL(t *testing.T) {
	want := []byte("solid nothing")
	url := "data:model/stl;base64," + base64.StdEncoding.EncodeToString(want)
	got, err := ParseBase64(url)
	if err != nil {
		t.Fatalf("ParseBase64: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The marker can be anywhere; what follows it is the payload.
func TestParseBase64TakesEverythingAfterTheMarker(t *testing.T) {
	got, err := ParseBase64("junk;base64," + base64.StdEncoding.EncodeToString([]byte("hi")))
	if err != nil {
		t.Fatalf("ParseBase64: %v", err)
	}
	if string(got) != "hi" {
		t.Errorf("got %q, want %q", got, "hi")
	}
}

func TestParseBase64RejectsInputWithNoPayload(t *testing.T) {
	for _, in := range []string{"", "data:model/stl", "not a data url", "base64"} {
		if _, err := ParseBase64(in); err == nil {
			t.Errorf("%q was accepted", in)
		}
	}
}

func TestParseBase64RejectsPayloadThatIsNotBase64(t *testing.T) {
	if _, err := ParseBase64("data:;base64,!!!not base64!!!"); err == nil {
		t.Error("a payload that is not base64 was accepted")
	}
}

func TestParseBase64OnAnEmptyPayload(t *testing.T) {
	got, err := ParseBase64("data:;base64,")
	if err != nil {
		t.Fatalf("an empty payload: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d bytes from an empty payload", len(got))
	}
}

func TestNewSTLRejectsWhatIsNotAnSTL(t *testing.T) {
	for _, in := range [][]byte{nil, []byte("hello"), []byte(strings.Repeat("x", 200))} {
		if _, err := NewSTL(in); err == nil {
			t.Errorf("%q was accepted as an STL", in)
		}
	}
}

// binarySTL builds a valid binary STL with n identical triangles.
func binarySTL(n uint32) []byte {
	b := make([]byte, stlHeaderBytes)
	copy(b, "a binary header")
	b = binary.LittleEndian.AppendUint32(b, n)
	for i := uint32(0); i < n; i++ {
		tri := make([]byte, 0, stlTriangleBytes)
		for j := 0; j < 12; j++ { // normal + three vertices, three floats each
			tri = binary.LittleEndian.AppendUint32(tri, math.Float32bits(float32(j)))
		}
		tri = append(tri, 0, 0) // attribute byte count
		b = append(b, tri...)
	}
	return b
}

func TestNewSTLReadsABinaryFile(t *testing.T) {
	s, err := NewSTL(binarySTL(3))
	if err != nil {
		t.Fatalf("a valid binary STL was refused: %v", err)
	}
	v, c, idx := s.GetModel()
	if len(idx) != 9 { // three vertices per triangle
		t.Errorf("got %d indices for 3 triangles, want 9", len(idx))
	}
	if len(v) != 27 { // three floats per vertex
		t.Errorf("got %d vertex floats, want 27", len(v))
	}
	if len(c) != 27 {
		t.Errorf("got %d color floats, want 27", len(c))
	}
}

// The triangle count sizes an allocation, so a file claiming more triangles
// than it could hold is a request for that much memory. Two hundred bytes of
// junk declares about two billion triangles — a hundred gigabytes — and used
// to take the process out with an out-of-memory kill rather than an error.
func TestNewSTLRefusesAnImpossibleTriangleCount(t *testing.T) {
	b := binarySTL(1)
	binary.LittleEndian.PutUint32(b[stlHeaderBytes:], 0xFFFFFFFF)

	_, err := NewSTL(b)
	if err == nil {
		t.Fatal("a file declaring four billion triangles was accepted")
	}
	if !strings.Contains(err.Error(), "triangles") {
		t.Errorf("the error does not explain the refusal: %v", err)
	}
}

// A file one triangle short of what it promises is truncated, not malicious,
// and is refused the same way rather than read past the end.
func TestNewSTLRefusesATruncatedFile(t *testing.T) {
	b := binarySTL(4)
	if _, err := NewSTL(b[:len(b)-stlTriangleBytes]); err == nil {
		t.Error("a truncated binary STL was accepted")
	}
}

// An ASCII file has no binary triangle count, so the size check must not read
// its text as one and refuse it.
func TestNewSTLStillReadsASCII(t *testing.T) {
	const ascii = `solid test
facet normal 0 0 1
  outer loop
    vertex 0 0 0
    vertex 1 0 0
    vertex 0 1 0
  endloop
endfacet
endsolid test
`
	if !looksASCII([]byte(ascii)) {
		t.Fatal("an ASCII STL was not recognized as one")
	}
	if _, err := NewSTL([]byte(ascii)); err != nil {
		t.Errorf("an ASCII STL was refused: %v", err)
	}
}

// A binary header often begins with the word solid, which is why the keyword
// alone cannot decide the format.
func TestLooksASCIINeedsMoreThanTheKeyword(t *testing.T) {
	b := binarySTL(1)
	copy(b, "solid a binary file whose header happens to say solid")
	if looksASCII(b) {
		t.Error("a binary file with solid in its header was taken for ASCII")
	}
}
