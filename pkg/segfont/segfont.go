// Package segfont is a 16-segment stroke font: capitals, digits and a little
// punctuation, each glyph a set of straight strokes on a 2×3 cell.
//
// Strokes rather than pixels because both of its users draw lines. The scope's
// Fourier Text tours them with a beam, and the STL dimension labels sweep them
// into solid rods that survive into a printed model.
package segfont

// Segment layout on a 2×3 cell (x 0..2, y 0..3, mid at 1.5):
//
//	 a1  a2
//	f h i j b
//	 g1  g2
//	e m l k c
//	 d1  d2
const (
	sgA1 = 1 << iota
	sgA2
	sgB
	sgC
	sgD2
	sgD1
	sgE
	sgF
	sgG1
	sgG2
	sgH
	sgI
	sgJ
	sgK
	sgL
	sgM
)

// segEnds is each segment's endpoints {x1,y1,x2,y2} on the cell grid.
var segEnds = [16][4]float64{
	{0, 3, 1, 3},     // a1
	{1, 3, 2, 3},     // a2
	{2, 3, 2, 1.5},   // b
	{2, 1.5, 2, 0},   // c
	{2, 0, 1, 0},     // d2
	{1, 0, 0, 0},     // d1
	{0, 0, 0, 1.5},   // e
	{0, 1.5, 0, 3},   // f
	{0, 1.5, 1, 1.5}, // g1
	{1, 1.5, 2, 1.5}, // g2
	{0, 3, 1, 1.5},   // h
	{1, 3, 1, 1.5},   // i
	{2, 3, 1, 1.5},   // j
	{1, 1.5, 2, 0},   // k
	{1, 1.5, 1, 0},   // l
	{1, 1.5, 0, 0},   // m
}

// segFont maps each supported rune to its lit segments.
var segFont = map[rune]uint16{
	'A': sgA1 | sgA2 | sgB | sgC | sgE | sgF | sgG1 | sgG2,
	'B': sgA1 | sgA2 | sgB | sgC | sgD1 | sgD2 | sgI | sgL | sgG2,
	'C': sgA1 | sgA2 | sgF | sgE | sgD1 | sgD2,
	'D': sgA1 | sgA2 | sgB | sgC | sgD1 | sgD2 | sgI | sgL,
	'E': sgA1 | sgA2 | sgF | sgE | sgD1 | sgD2 | sgG1 | sgG2,
	'F': sgA1 | sgA2 | sgF | sgE | sgG1 | sgG2,
	'G': sgA1 | sgA2 | sgF | sgE | sgD1 | sgD2 | sgC | sgG2,
	'H': sgF | sgE | sgB | sgC | sgG1 | sgG2,
	'I': sgA1 | sgA2 | sgI | sgL | sgD1 | sgD2,
	'J': sgB | sgC | sgD1 | sgD2 | sgE,
	'K': sgF | sgE | sgG1 | sgJ | sgK,
	'L': sgF | sgE | sgD1 | sgD2,
	'M': sgF | sgE | sgH | sgJ | sgB | sgC,
	'N': sgF | sgE | sgH | sgK | sgB | sgC,
	'O': sgA1 | sgA2 | sgB | sgC | sgD1 | sgD2 | sgE | sgF,
	'P': sgA1 | sgA2 | sgB | sgF | sgE | sgG1 | sgG2,
	'Q': sgA1 | sgA2 | sgB | sgC | sgD1 | sgD2 | sgE | sgF | sgK,
	'R': sgA1 | sgA2 | sgB | sgF | sgE | sgG1 | sgG2 | sgK,
	'S': sgA1 | sgA2 | sgF | sgG1 | sgG2 | sgC | sgD1 | sgD2,
	'T': sgA1 | sgA2 | sgI | sgL,
	'U': sgF | sgE | sgD1 | sgD2 | sgC | sgB,
	'V': sgF | sgE | sgJ | sgM,
	'W': sgF | sgE | sgM | sgK | sgB | sgC,
	'X': sgH | sgJ | sgK | sgM,
	'Y': sgH | sgJ | sgL,
	'Z': sgA1 | sgA2 | sgJ | sgM | sgD1 | sgD2,
	'0': sgA1 | sgA2 | sgB | sgC | sgD1 | sgD2 | sgE | sgF | sgJ | sgM,
	'1': sgI | sgL,
	'2': sgA1 | sgA2 | sgB | sgG1 | sgG2 | sgE | sgD1 | sgD2,
	'3': sgA1 | sgA2 | sgB | sgC | sgD1 | sgD2 | sgG2,
	'4': sgF | sgG1 | sgG2 | sgB | sgC,
	'5': sgA1 | sgA2 | sgF | sgG1 | sgG2 | sgC | sgD1 | sgD2,
	'6': sgA1 | sgA2 | sgF | sgE | sgD1 | sgD2 | sgC | sgG1 | sgG2,
	'7': sgA1 | sgA2 | sgB | sgC,
	'8': sgA1 | sgA2 | sgB | sgC | sgD1 | sgD2 | sgE | sgF | sgG1 | sgG2,
	'9': sgA1 | sgA2 | sgB | sgC | sgD1 | sgD2 | sgF | sgG1 | sgG2,
	'-': sgG1 | sgG2,
	'?': sgA1 | sgA2 | sgB | sgG2 | sgL,
	'!': sgI, // dot omitted — single-stroke font
	' ': 0,
}

// Segments returns the strokes that draw r, each {x1, y1, x2, y2} on the cell
// grid (x 0..2, y 0..3, y up), in segment order. ok is false for a rune the
// font does not have. A space is in the font and has no strokes.
func Segments(r rune) (segs [][4]float64, ok bool) {
	mask, ok := segFont[r]
	if !ok {
		return nil, false
	}
	for s := range 16 {
		if mask&(1<<s) != 0 {
			segs = append(segs, segEnds[s])
		}
	}
	return segs, true
}
