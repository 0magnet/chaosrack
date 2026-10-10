// Package seat owns a browser page's screen and switches it between screens:
// consoles, a desktop, the page itself — what a machine's virtual terminals
// do between its text consoles and its graphical session.
//
// A page that is a whole environment needs that layer and has no place for
// it otherwise. A desktop (0magnet/desk) is a graphical session, a terminal
// (0magnet/websh) is a console, and neither is the thing that holds both:
// put the switching in the desktop and a console is a window in it; put it
// in the terminal and the terminal owns a desktop. Here it is neither, and it
// imports neither. The page that wants both brings them and names them:
//
//	s := seat.New(seat.Options{})
//	s.AddPage("instrument", nil)         // the page itself, under the seat
//	s.Add("console", termPane)           // anything with Mount and Close
//	s.Show("instrument")
//
// Ctrl+Alt+1 to Ctrl+Alt+9 switch to the screens in the order they were
// added, as Ctrl+Alt+F1… does on a console (the F keys themselves never
// reach a page on Linux: the machine takes them for its own consoles).
//
// A Screen is anything that renders into an element and can be closed —
// the same two methods as a desk pane, so a pane is a screen as it is. It is
// mounted the first time it is shown and kept running after, as a console
// is: switching away hides it, it does not stop it.
package seat

// keyIndex is the screen a key press switches to, from 0, or -1 for a key
// that is not a switch: Ctrl+Alt and a digit 1 to 9, nothing else held.
//
// The digit is read from the key's position (code) and must also be what it
// typed (key), because on a layout with AltGr — German, French, Polish, most
// of Europe — Ctrl+Alt is AltGr, and Ctrl+Alt+7 types "{". That press is a
// character for whatever has the keyboard, never a switch.
func keyIndex(ctrl, alt, shift, meta bool, code, key string) int {
	if !ctrl || !alt || shift || meta || len(code) != len("Digit1") || code[:5] != "Digit" {
		return -1
	}
	d := code[5]
	if d < '1' || d > '9' || key != string(d) {
		return -1
	}
	return int(d - '1')
}
