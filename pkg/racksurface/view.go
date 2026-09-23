package racksurface

// A window onto the surface.
//
// The surface does not resize, so a viewer that is too small shows less of it
// rather than a different arrangement of it — which is what happens when a
// rack is put in a window too small for it, and what the page already does.
// Growing the terminal, or shrinking its cell, reveals more of the SAME
// picture. Nothing moves.
//
// All of this is arithmetic on two ints, and it is here rather than in the
// renderer because the clamping is the part that is easy to get subtly wrong:
// an offset that may go one cell past the end lets the user scroll the rack
// off the screen and then wonder where it went.

// View is the visible rectangle: an offset into the surface, and a size.
type View struct {
	X, Y int // top-left cell of the surface that is visible
	W, H int // the viewer's size in cells
}

// Clamp holds the offset inside the surface.
//
// The rule at the small end is the one that matters: when the viewer is WIDER
// than the surface there is nothing to scroll, and the offset must go to zero
// rather than to some negative slack, or the rack drifts right as the window
// grows.
func (v View) Clamp(s Surface) View {
	if maxX := s.Cols - v.W; v.X > maxX {
		v.X = maxX
	}
	if maxY := s.Rows - v.H; v.Y > maxY {
		v.Y = maxY
	}
	if v.X < 0 {
		v.X = 0
	}
	if v.Y < 0 {
		v.Y = 0
	}
	return v
}

// Pan moves the window by whole cells and clamps.
func (v View) Pan(dx, dy int, s Surface) View {
	v.X += dx
	v.Y += dy
	return v.Clamp(s)
}

// Reveal moves the window as little as it can to bring a rectangle into it.
//
// As little as it can, because this is what runs when the cursor moves from
// one control to the next: a view that recentered on every step would make
// the whole rack lurch for a keypress that moved one row.
func (v View) Reveal(x, y, w, h int, s Surface) View {
	if x < v.X {
		v.X = x
	} else if r := x + w - v.W; r > v.X {
		v.X = r
	}
	if y < v.Y {
		v.Y = y
	} else if b := y + h - v.H; b > v.Y {
		v.Y = b
	}
	return v.Clamp(s)
}

// Sees reports whether any part of a rectangle is in the window. The renderer
// asks this per panel, so a rack far bigger than the terminal costs only the
// panels actually on screen.
func (v View) Sees(x, y, w, h int) bool {
	return x < v.X+v.W && x+w > v.X && y < v.Y+v.H && y+h > v.Y
}

// Bar is the position and length of a scrollbar thumb in a track of the given
// length: where the window is over the surface, drawn.
//
// It returns a zero length when everything fits, which is the caller's signal
// to draw no bar at all rather than a full-length one — a scrollbar that is
// always there and always full is furniture, not information.
func Bar(offset, window, total, track int) (pos, length int) {
	if window >= total || total <= 0 || track <= 0 {
		return 0, 0
	}
	length = window * track / total
	if length < 1 {
		length = 1
	}
	// The thumb's travel is the track minus the thumb, so the far end of the
	// surface puts the thumb against the far end of the track. Dividing by
	// total instead leaves a gap there that reads as "there is more", and
	// there is not.
	if span := total - window; span > 0 {
		pos = offset * (track - length) / span
	}
	if pos+length > track {
		pos = track - length
	}
	if pos < 0 {
		pos = 0
	}
	return pos, length
}
