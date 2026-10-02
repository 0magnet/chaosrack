//go:build js && wasm

package attractor

import (
	"strconv"
	"strings"
	"testing"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
)

// The waveform ring is built from three lists that have to agree: a wave
// with no glyph is a detent with nothing printed at it, and one with no name
// or description is a position that explains nothing of itself.
func TestEveryWaveHasAGlyphANameAndAHelp(t *testing.T) {
	if len(genWaves) != audiosrc.WaveCount || len(waveSVG) != audiosrc.WaveCount {
		t.Fatalf("%d waves named, %d drawn, %d made", len(genWaves), len(waveSVG), audiosrc.WaveCount)
	}
	for i, w := range genWaves {
		if w.name == "" || w.help == "" {
			t.Errorf("wave %d: name %q, help %q", i, w.name, w.help)
		}
	}
}

// Every generator's parts are in the markup, where the builder looks for
// them: one missing is a generator that quietly is not built.
func TestEveryGeneratorHasItsControls(t *testing.T) {
	for _, o := range genOscs {
		for _, part := range []string{"-freq", "-lvl", "-wave", "-ostack", "-module"} {
			if !strings.Contains(controlsBody, `id="`+o.id+part+`"`) {
				t.Errorf("%s%s is not in the panel markup", o.id, part)
			}
		}
	}
	for _, id := range []string{"gen-solo"} {
		if !strings.Contains(controlsBody, `id="`+id+`"`) {
			t.Errorf("%s is not in the panel markup", id)
		}
	}
}

// The Mixer's link is its pins as they differ from the rack as it starts,
// and reading it back gives the same pins — a gain, an inversion and a
// default taken away included.
func TestTheMixerLinkRoundTrips(t *testing.T) {
	defer mixReset()
	link := "rl.g2:-0.5,rl.ky,rr.cr:0" // row by row, as the Mixer writes it
	applyMixLink(link)
	if got := mixLinkValue(); got != link {
		t.Errorf("link %q comes back as %q", link, got)
	}
	b := aud.rackBus()
	if g := b.Mix[0][audiosrc.InGen+1]; g != -0.5 {
		t.Errorf("GEN 2 on RACK L is %.2f on the bus, want −0.5", g)
	}
	if g := b.Mix[1][audiosrc.InCaptureR]; g != 0 {
		t.Errorf("CAP R is still on RACK R at %.2f", g)
	}
	if !b.ReturnOn {
		t.Error("KEYS on RACK L, and the bus is not reading the return")
	}
	mixReset()
	if v := mixLinkValue(); v != "" {
		t.Errorf("reset, the Mixer still writes %q", v)
	}
}

// The capture never reaches the speakers: no pin for it there, and a link
// asking for one is refused.
func TestTheCaptureNeverReachesTheSpeakers(t *testing.T) {
	defer mixReset()
	for _, k := range []string{"cl", "cr"} {
		s := mixSrcIndex(k)
		for _, r := range []int{mixSpkL, mixSpkR} {
			if mixAllowed(r, s) {
				t.Errorf("%s can be pinned to %s", k, mixRowKeys[r])
			}
		}
	}
	applyMixLink("sl.cl,sr.cr")
	if mixOnSpeakers(mixSrcIndex("cl")) || mixOnSpeakers(mixSrcIndex("cr")) {
		t.Error("a link put the capture on the speakers")
	}
}

// Every column, row and group the Mixer and the Mod matrix draw has an entry
// in the manual, and so a tooltip: their keys are made, not written, so the
// test that finds written ones cannot see them.
func TestEveryRoutingPartIsInTheManual(t *testing.T) {
	var keys []string
	for _, s := range mixSources {
		keys = append(keys, "mix-src="+s.key, "mix-group="+s.group)
	}
	for _, r := range mixRowKeys {
		keys = append(keys, "mix-row="+r)
	}
	for _, c := range modChannels {
		keys = append(keys, "mod-src="+c.key, "mod-group="+c.group)
	}
	for in := range scopeInCount {
		keys = append(keys, "scope-in="+strconv.Itoa(in))
	}
	for _, k := range keys {
		if doc(k) == "" {
			t.Errorf("%s has no entry in the manual", k)
		}
	}
}
