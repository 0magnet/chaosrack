package attractor

import "testing"

// TestRowsAloneRemakeTheTerminals: the rows are the terminals' size, not an
// argument of their programs, and a change of ROWS alone kept the terminals
// of the rows before. Drawn at the new count, a taller lattice read past the
// end of every line, and the page's program died with an index out of range.
func TestRowsAloneRemakeTheTerminals(t *testing.T) {
	l := NewLatticeStack()
	l.nF = 12
	if _, started := l.Ensure(nil, 0); !started {
		t.Fatal("the first Ensure started nothing")
	}
	l.nF = 13
	n, started := l.Ensure(nil, 0)
	if !started {
		t.Fatal("ROWS 12 → 13 kept the terminals of 12")
	}
	if len(l.sheets) == 0 {
		t.Fatal("no sheets")
	}
	for i, s := range l.sheets {
		if c := s.term.Cols(); c != 2*n {
			t.Fatalf("sheet %d is %d columns, want %d", i, c, 2*n)
		}
	}
}
