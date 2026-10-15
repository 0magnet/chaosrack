package attractor

import (
	"testing"

	"github.com/0magnet/chaosrack/pkg/led"
	"github.com/0magnet/chaosrack/pkg/meters"
)

func readoutOf(rs []Readout, id string) string {
	for _, r := range rs {
		if r.ID == id {
			return r.Text
		}
	}
	return "(none)"
}

// A meter with no reading says so on every display, and one with a reading
// prints it in the display's own width.
func TestMeterReadoutsFormatAsTheDisplaysDo(t *testing.T) {
	if got := readoutOf(THDReadouts(meters.DistortionResult{}), "thd-fund-led"); got != "-----.-" {
		t.Errorf("no reading: fundamental %q", got)
	}
	if got := readoutOf(THDReadouts(meters.DistortionResult{OK: true, Fundamental: 196, SINAD: 950}), "thd-sinad-led"); got != "  >99.9" {
		t.Errorf("SINAD past the display: %q", got)
	}
	floor := meters.LoudnessFloor
	quiet := meters.LoudnessResult{Momentary: floor, ShortTerm: floor, Integrated: floor, TruePeak: floor}
	if got := readoutOf(LoudnessReadouts(quiet, -23), "lufs-delta-led"); got != "  --.-" {
		t.Errorf("no integration: distance to target %q", got)
	}
	loud := meters.LoudnessResult{OK: true, Momentary: -20, ShortTerm: -21, Integrated: -20, TruePeak: -3}
	if got := readoutOf(LoudnessReadouts(loud, -23), "lufs-delta-led"); got != led.Blank+"003.0" {
		t.Errorf("3 LU over a −23 target: %q", got)
	}
	if got := CounterReadout(440).Text; got != "00440.0" {
		t.Errorf("440 Hz on the counter: %q", got)
	}
}
