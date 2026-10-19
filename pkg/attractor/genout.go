package attractor

// A generator's OUT: where on the speakers and the rack's signal it goes, as
// a knob says it. The Mixer holds the pins; OUT names the pinnings a hand
// wants most — the one side, the other, or both, always the speakers and the
// rack together, so what is heard is what the scopes and meters read — and
// reads MIX for any other, which the Mixer, where every pin and gain is, set.
// The sends to modulation are not OUT's: a generator can be both heard and
// sent, and OUT moves only the first.

// The OUT positions, in order round the knob.
const (
	GenOutOff = iota
	GenOutL
	GenOutC
	GenOutR
	GenOutMix
	GenOutCount
)

// GenOutNames are the positions as the display reads them.
var GenOutNames = [GenOutCount]string{"OFF", "L", "C", "R", "MIX"}

// GenOutOf is the position a generator's four pins make: the left and right
// speakers', and the rack's left and right.
func GenOutOf(spkL, spkR, rackL, rackR float32) int {
	l, r := spkL == rackL, spkR == rackR
	switch {
	case !l || !r || spkL != 0 && spkL != 1 || spkR != 0 && spkR != 1:
		return GenOutMix
	case spkL == 0 && spkR == 0:
		return GenOutOff
	case spkR == 0:
		return GenOutL
	case spkL == 0:
		return GenOutR
	}
	return GenOutC
}

// GenOutPins is the gain of the left pins and of the right pins, speaker and
// rack alike, at position pos; ok is false for MIX, which leaves the pins as
// the Mixer has them.
func GenOutPins(pos int) (l, r float32, ok bool) {
	switch pos {
	case GenOutOff:
		return 0, 0, true
	case GenOutL:
		return 1, 0, true
	case GenOutC:
		return 1, 1, true
	case GenOutR:
		return 0, 1, true
	}
	return 0, 0, false
}
