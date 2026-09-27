package dotmatrix

import (
	"strings"
	"testing"
)

// Every legend a bank knob carries today, and so every character the font
// has to have rather than draw as '?'.
const bankLegends = "01ABCDHKLRSVabcdefghiklmnopqrstuvwxyαβγεμρστ"

func TestTheFontDrawsEveryBankLegend(t *testing.T) {
	for _, r := range bankLegends {
		if !Has(r) {
			t.Errorf("no glyph for %q", r)
		}
	}
}

// A glyph wider than its cell would light a dot in the gap, which is the
// neighbor's column, and two characters would run together.
func TestNoGlyphIsWiderThanItsCell(t *testing.T) {
	for r, g := range font {
		for _, row := range g {
			if row >= 1<<Cols {
				t.Errorf("%q has a row %05b wider than %d dots", r, row, Cols)
			}
		}
	}
}

// Every dot of the display is drawn, lit or dark: a character module has no
// holes in it, and a short legend leaves the rest of the display dark.
func TestEveryDotIsDrawnOnce(t *testing.T) {
	for _, s := range []string{"", "σ", "offset", "much too long for it"} {
		lit, dark := Paths(s, 6)
		n := strings.Count(lit, "M") + strings.Count(dark, "M")
		if want := 6 * Cols * Rows; n != want {
			t.Errorf("%q: %d dots, want %d", s, n, want)
		}
	}
	if lit, _ := Paths("", 6); lit != "" {
		t.Errorf("an empty display lit %q", lit)
	}
}

func TestWidth(t *testing.T) {
	if got := Width(6); got != 35 {
		t.Errorf("Width(6) = %d, want 35", got)
	}
}

// A vertical display has the same dots as a horizontal one, stacked: every
// dot drawn once, and the first character at the top.
func TestAVerticalDisplayStacksTheSameDots(t *testing.T) {
	lit, dark := PathsVertical("ab", 3)
	if n := strings.Count(lit, "M") + strings.Count(dark, "M"); n != 3*Cols*Rows {
		t.Errorf("%d dots, want %d", n, 3*Cols*Rows)
	}
	h, _ := Paths("ab", 3)
	if strings.Count(lit, "M") != strings.Count(h, "M") {
		t.Errorf("vertical lights %d dots, horizontal %d", strings.Count(lit, "M"), strings.Count(h, "M"))
	}
	if got := Height(3); got != 23 {
		t.Errorf("Height(3) = %d, want 23", got)
	}
}
