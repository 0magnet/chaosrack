package rhythm

import "testing"

// The rows climb the pentatonic from the root on the bottom row, three
// octaves to the top.
func TestNoteOf(t *testing.T) {
	root := RootMidi(3)
	if root != 48 {
		t.Fatalf("root 3 is midi %d, want C3 (48)", root)
	}
	for _, c := range []struct{ row, want int }{
		{15, 48}, {14, 50}, {13, 52}, {12, 55}, {11, 57}, {10, 60}, {0, 84},
	} {
		if got := NoteOf(root, c.row); got != c.want {
			t.Errorf("row %d is %d, want %d", c.row, got, c.want)
		}
	}
}

// The loop takes a whole number of the preset's bars.
func TestLoopFor(t *testing.T) {
	for _, c := range []struct{ steps, beats, want int }{
		{16, 4, 16}, {16, 3, 12}, {64, 3, 48}, {32, 3, 24}, {12, 3, 12}, {16, 0, 16},
	} {
		if got := LoopFor(c.steps, c.beats); got != c.want {
			t.Errorf("LoopFor(%d, %d) = %d, want %d", c.steps, c.beats, got, c.want)
		}
	}
}

// A pad rings two columns, held between a fifth and a half of a second.
func TestPingSeconds(t *testing.T) {
	for _, c := range []struct{ step, want float64 }{
		{0.125, 0.25}, {0.05, 0.2}, {0.4, 0.5},
	} {
		if got := PingSeconds(c.step); got != c.want {
			t.Errorf("PingSeconds(%v) = %v, want %v", c.step, got, c.want)
		}
	}
}
