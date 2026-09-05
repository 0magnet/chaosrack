package canvas

// This is libcaca's transform.c: the flip, flop and rotate operations that back
// toilet's flip, flop, 180, left and right filters. The lookup tables live in
// tables.go.

// flipChar returns the horizontal mirror image of a character.
func flipChar(ch rune) rune { return lookupPair(ch, flipNoFlip, flipPairs) }

// flopChar returns the vertical mirror image of a character.
func flopChar(ch rune) rune { return lookupPair(ch, flopNoFlop, flopPairs) }

// rotateChar returns the 180-degree rotation of a character.
func rotateChar(ch rune) rune { return lookupPair(ch, rotateNoRotate, rotatePairs) }

// lookupPair implements the search flipchar(), flopchar() and rotatechar()
// share: characters in the first table are left alone, characters in the second
// are swapped with their neighbor, and anything else is returned unchanged.
func lookupPair(ch rune, fixed, pairs []rune) rune {
	for i := 0; fixed[i] != 0; i++ {
		if ch == fixed[i] {
			return ch
		}
	}
	for i := 0; pairs[i] != 0; i++ {
		if ch == pairs[i] {
			return pairs[i^1]
		}
	}
	return ch
}

// leftPair rotates a horizontal pair of cells 90 degrees counterclockwise.
func leftPair(pair *[2]rune) { rotatePair(pair, 2) }

// rightPair rotates a horizontal pair of cells 90 degrees clockwise.
func rightPair(pair *[2]rune) { rotatePair(pair, -2) }

// rotatePair looks a cell pair up in the two-by-two and two-by-four tables and
// replaces it with the entry `step` positions round the cycle.
func rotatePair(pair *[2]rune, step int) {
	for i := 0; leftright2x2[i] != 0; i += 2 {
		if pair[0] == leftright2x2[i] && pair[1] == leftright2x2[i+1] {
			j := (i &^ 3) | ((i + step) & 3)
			pair[0], pair[1] = leftright2x2[j], leftright2x2[j+1]
			return
		}
	}
	for i := 0; leftright2x4[i] != 0; i += 2 {
		if pair[0] == leftright2x4[i] && pair[1] == leftright2x4[i+1] {
			j := (i &^ 7) | ((i + step) & 7)
			pair[0], pair[1] = leftright2x4[j], leftright2x4[j+1]
			return
		}
	}
}

// Flip mirrors the canvas horizontally, substituting mirrored characters where
// one exists. The operation is involutive.
func (cv *Canvas) Flip() {
	for y := 0; y < cv.Height; y++ {
		row := y * cv.Width
		left, right := 0, cv.Width-1

		for left < right {
			cv.Attrs[row+left], cv.Attrs[row+right] = cv.Attrs[row+right], cv.Attrs[row+left]
			ch := cv.Chars[row+right]
			cv.Chars[row+right] = flipChar(cv.Chars[row+left])
			cv.Chars[row+left] = flipChar(ch)
			left++
			right--
		}
		if left == right {
			cv.Chars[row+left] = flipChar(cv.Chars[row+left])
		}

		// Put the halves of every fullwidth glyph back in order.
		for x := 0; x < cv.Width-1; x++ {
			if cv.Chars[row+x] == magicFullwidth {
				cv.Chars[row+x] = cv.Chars[row+x+1]
				cv.Chars[row+x+1] = magicFullwidth
				x++
			}
		}
	}
}

// Flop mirrors the canvas vertically, substituting mirrored characters where
// one exists. The operation is involutive.
func (cv *Canvas) Flop() {
	for x := 0; x < cv.Width; x++ {
		top, bottom := x, x+cv.Width*(cv.Height-1)

		for top < bottom {
			cv.Attrs[top], cv.Attrs[bottom] = cv.Attrs[bottom], cv.Attrs[top]
			ch := cv.Chars[bottom]
			cv.Chars[bottom] = flopChar(cv.Chars[top])
			cv.Chars[top] = flopChar(ch)
			top += cv.Width
			bottom -= cv.Width
		}
		if top == bottom {
			cv.Chars[top] = flopChar(cv.Chars[top])
		}
	}
}

// Rotate180 turns the canvas upside down, substituting rotated characters where
// one exists. The operation is involutive.
func (cv *Canvas) Rotate180() {
	if len(cv.Chars) == 0 {
		return
	}

	begin, end := 0, cv.Width*cv.Height-1
	for begin < end {
		cv.Attrs[begin], cv.Attrs[end] = cv.Attrs[end], cv.Attrs[begin]
		ch := cv.Chars[end]
		cv.Chars[end] = rotateChar(cv.Chars[begin])
		cv.Chars[begin] = rotateChar(ch)
		begin++
		end--
	}
	if begin == end {
		cv.Chars[begin] = rotateChar(cv.Chars[begin])
	}

	for y := 0; y < cv.Height; y++ {
		row := y * cv.Width
		for x := 0; x < cv.Width-1; x++ {
			if cv.Chars[row+x] == magicFullwidth {
				cv.Chars[row+x] = cv.Chars[row+x+1]
				cv.Chars[row+x+1] = magicFullwidth
				x++
			}
		}
	}
}

// RotateLeft turns the canvas 90 degrees counterclockwise. Cells are rotated
// two by two, so the width halves and becomes the height while the height
// doubles and becomes the width. Fullwidth characters at odd columns are lost.
func (cv *Canvas) RotateLeft() {
	cv.rotate90(func(pair *[2]rune) { leftPair(pair) },
		func(w2, h2, x, y int) int { return (h2*(w2-1-x) + y) * 2 },
		func(x, y, w, _ int) (int, int) { return y * 2, (w - 1 - x) / 2 })
}

// RotateRight turns the canvas 90 degrees clockwise, under the same rules as
// RotateLeft.
func (cv *Canvas) RotateRight() {
	cv.rotate90(func(pair *[2]rune) { rightPair(pair) },
		func(_, h2, x, y int) int { return (h2*x + h2 - 1 - y) * 2 },
		func(x, y, _, h int) (int, int) { return (h - 1 - y) * 2, x / 2 })
}

// rotate90 is the body caca_rotate_left() and caca_rotate_right() share; they
// differ only in the pair transform, in where each pair lands and in how the
// cursor and handle coordinates are carried over.
func (cv *Canvas) rotate90(pairFn func(*[2]rune), index func(w2, h2, x, y int) int,
	move func(x, y, w, h int) (int, int)) {
	w2 := (cv.Width + 1) / 2
	h2 := cv.Height

	// libcaca allocates the new cell arrays before it does anything else, and
	// its allocator refuses a zero dimension. An empty canvas therefore comes
	// back from a rotation untouched, cursor and handle included.
	if w2 == 0 || h2 == 0 {
		return
	}

	newChars := make([]rune, w2*h2*2)
	newAttrs := make([]uint32, w2*h2*2)

	for y := 0; y < h2; y++ {
		for x := 0; x < w2; x++ {
			var pair [2]rune
			pair[0] = cv.Chars[cv.Width*y+x*2]
			attr1 := cv.Attrs[cv.Width*y+x*2]

			var attr2 uint32
			if cv.Width&1 != 0 && x == w2-1 {
				// Odd final column: pretend the missing cell is a space.
				pair[1] = ' '
				attr2 = attr1
			} else {
				pair[1] = cv.Chars[cv.Width*y+x*2+1]
				attr2 = cv.Attrs[cv.Width*y+x*2+1]
			}

			// A space contributes no color of its own, or the rotated pair
			// would take on an attribute nothing in it was drawn with.
			if pair[0] == ' ' {
				attr1 = attr2
			} else if pair[1] == ' ' {
				attr2 = attr1
			}

			pairFn(&pair)

			i := index(w2, h2, x, y)
			newChars[i], newAttrs[i] = pair[0], attr1
			newChars[i+1], newAttrs[i+1] = pair[1], attr2
		}
	}

	// The cursor and the handle turn with the canvas, and both are computed
	// from the old dimensions.
	cv.X, cv.Y = move(cv.X, cv.Y, cv.Width, cv.Height)
	cv.HandleX, cv.HandleY = move(cv.HandleX, cv.HandleY, cv.Width, cv.Height)

	cv.Chars, cv.Attrs = newChars, newAttrs
	cv.Width, cv.Height = cv.Height*2, w2
}
