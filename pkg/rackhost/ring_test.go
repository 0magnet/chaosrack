package rackhost

import (
	"strconv"
	"testing"

	"github.com/0magnet/chaosrack/pkg/attractor"
)

// fills is the colors of a ring's LEDs now.
func fills(r *Rack, rg ledRing) []int {
	out := make([]int, len(rg.leds))
	for i, item := range rg.leds {
		out[i] = -1
		if item >= 0 && r.pic.Items[item].Fill != nil {
			out[i] = *r.pic.Items[item].Fill
		}
	}
	return out
}

// A generator's FREQ ring lights to the note in its octave's color: C3 to
// C4 in the rack's LED color, the octave over it in the next, over the
// first's still lit.
func TestTheFreqRingLightsInItsOctavesColor(t *testing.T) {
	r := newRack(t)
	rg, ok := r.rings["gen-y-freq"]
	if !ok || len(rg.leds) != 24 {
		t.Fatalf("Gen 2's FREQ has a ring of %d LEDs", len(rg.leds))
	}
	col := func(i int) int { return hexColor(attractor.LEDColors[i].Col) }
	dark := hexColor(attractor.LEDColors[0].BG)
	set(t, r, "gen-y-freq", "30") // D#3: a quarter of the way round
	f := fills(r, rg)
	if f[0] != col(0) || f[5] != col(0) || f[6] != dark || f[23] != dark {
		t.Errorf("D#3 lights %06x %06x %06x %06x", f[0], f[5], f[6], f[23])
	}
	set(t, r, "gen-y-freq", "42") // D#4: the next octave's color over the first's
	f = fills(r, rg)
	if f[0] != col(1) || f[5] != col(1) || f[6] != col(0) || f[23] != col(0) {
		t.Errorf("D#4 lights %06x %06x %06x %06x", f[0], f[5], f[6], f[23])
	}
}

// Turning a bank position's encoder relights its ring, where it used to stay
// as the catalog drew it.
func TestABankRingFollowsItsKnob(t *testing.T) {
	r := newRack(t)
	set(t, r, r.selector, "lorenz")
	// One of Lorenz's own constants. (A bounded knob found anywhere in the
	// bank's region was this once, and the region reached the Display
	// module's TRAIL while the bay-3 modules were bank controls outside it.)
	id := "lorenz-s"
	if _, ok := r.rings[id]; !ok || !r.inBank[id] {
		t.Fatalf("%s has no ring in Lorenz's bank", id)
	}
	rg, c := r.rings[id], r.ctls[r.byID[id]]
	lit := func() (n int) {
		for _, f := range fills(r, rg) {
			if f == hexColor(attractor.LEDColors[0].Col) {
				n++
			}
		}
		return n
	}
	set(t, r, id, strconv.FormatFloat(c.Min, 'f', -1, 64))
	low := lit()
	set(t, r, id, strconv.FormatFloat(c.Max, 'f', -1, 64))
	if high := lit(); high != len(rg.leds) || high <= low {
		t.Errorf("%s lights %d LEDs at its bottom and %d at its top, of %d", id, low, high, len(rg.leds))
	}
}
