package attractor

import (
	"strings"

	"github.com/0magnet/chaosrack/pkg/dotmatrix"
	"github.com/0magnet/chaosrack/pkg/led"
)

// Where a bank's positions go, as arithmetic, so it can be tested without a
// page (rackbank_js.go builds the cells).
//
// A cell several models share — a family's dial, a parameter declared by more
// than one model in the row — is placed once and shown for each of them, so
// it has to be at the SAME position for all of them. Prepending the shared
// cells a model declares to its list, which is what the bank first did, gave
// one position to each of two shared cells whenever their models differed:
// the embeddings' tau is takens' and polar's, their smooth is stereo's too,
// and on stereo, with no tau, smooth moved up to where tau stood for the
// others. Placed once, both stood in the first position at once.

// bankSlot is what is at one of a model's positions: shared cell Shared, or
// its own cell Own, or nothing (both -1) where a shared cell it is not in has
// kept the position for the models that are.
type bankSlot struct{ Shared, Own int }

// bankSharedSlots gives each shared cell its position: the lowest one that
// none of the models it serves has already given to another shared cell.
// shared[i] is the models the i-th shared cell is for, in declaration order.
func bankSharedSlots(shared [][]string) []int {
	taken := map[string]map[int]bool{}
	out := make([]int, len(shared))
	for i, models := range shared {
		p := 0
		for ; ; p++ {
			free := true
			for _, m := range models {
				if taken[m][p] {
					free = false
					break
				}
			}
			if free {
				break
			}
		}
		out[i] = p
		for _, m := range models {
			if taken[m] == nil {
				taken[m] = map[int]bool{}
			}
			taken[m][p] = true
		}
	}
	return out
}

// bankModelSlots lays out one model's positions: the shared cells it is in
// at their positions, and its own n cells, in order, in the positions around
// them. A position no longer needed after its last cell is not returned.
func bankModelSlots(model string, shared [][]string, pos []int, n int) []bankSlot {
	at := map[int]int{} // position → shared cell, for the ones this model is in
	last := -1
	for i, models := range shared {
		for _, m := range models {
			if m == model {
				at[pos[i]] = i
				last = max(last, pos[i])
			}
		}
	}
	var out []bankSlot
	own := 0
	for p := 0; own < n || p <= last; p++ {
		if i, ok := at[p]; ok {
			out = append(out, bankSlot{Shared: i, Own: -1})
			continue
		}
		if own < n {
			out = append(out, bankSlot{Shared: -1, Own: own})
			own++
			continue
		}
		out = append(out, bankSlot{Shared: -1, Own: -1})
	}
	return out
}

// The rack's character displays come in two sizes, full and half, the widths
// of panel.css's --disp-full and --disp-half, which a seven-segment readout
// has too: seven digits or three. A display for a number or a name is one or
// the other, never sized to what it happens to say, or every readout in the
// rack is a part of its own.
const (
	dispFullChars = 8
	dispHalfChars = 4
)

// bankValChars is how many characters a bank position's readout holds, a
// setting's name or a number alike. The legends down the cells' edges are
// sized by bankChars instead.
const bankValChars = dispFullChars

// readoutText is a seven-segment readout's text as a character display
// writes it: without the leading zeros that kept seven segments from
// changing width ("043.0" is "43.0"), and right-aligned, as a number is.
func readoutText(s string) string {
	// A character display has a plus, where seven segments leave the slot
	// blank (led.Blank).
	s = strings.TrimSpace(strings.Replace(s, led.Blank, "+", 1))
	sign := ""
	if s != "" && (s[0] == '-' || s[0] == '+') {
		sign, s = s[:1], s[1:]
	}
	for len(s) > 1 && s[0] == '0' && s[1] != '.' {
		s = s[1:]
	}
	s = sign + s
	if n := len([]rune(s)); n < bankValChars {
		s = strings.Repeat(" ", bankValChars-n) + s
	}
	return s
}

// displayText is s as a character display can write it: without the
// characters its font has no dots for ("— none —" is "none"), and with the
// spaces that leaves run together.
func displayText(s string) string {
	var b strings.Builder
	for _, r := range s {
		if dotmatrix.Has(r) {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
