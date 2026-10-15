package rackhost

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
)

// meterSays is what a meter's display says, as a number, and whether it is
// one.
func meterSays(r *Rack, id string) (float64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, err := strconv.ParseFloat(strings.TrimSpace(ledText(r, id)), 64)
	return v, err == nil
}

// The measurement modules read the rack's signal: a generator's sine on it
// is the Distortion module's fundamental and the Counter's frequency, it is
// loud on the Loudness module, and RESET clears the integration.
func TestTheMetersReadTheRacksSignal(t *testing.T) {
	r, c := idleRack(t, "lorenz")
	set(t, r, "gen-x-wave", "0", "gen-x-lvl", "80", "mix-rl-g1", "1", "mix-rr-g1", "1")
	looks(t, r, c, 2.5)
	fund, ok := meterSays(r, "thd-fund-led")
	if !ok || fund < 20 {
		t.Fatalf("the Distortion module's fundamental reads %q", ledText(r, "thd-fund-led"))
	}
	if hz, ok := meterSays(r, "counter-led"); !ok || math.Abs(hz-fund) > 0.02*fund+1 {
		t.Errorf("the Counter reads %q, the fundamental %.1f", ledText(r, "counter-led"), fund)
	}
	if m, ok := meterSays(r, "lufs-m-led"); !ok || m > 0 || m < -30 {
		t.Errorf("a sine at 80%% reads %q LUFS momentary", ledText(r, "lufs-m-led"))
	}
	r.mu.Lock()
	r.press("btn lufs-reset")
	r.mu.Unlock()
	if _, ok := meterSays(r, "lufs-i-led"); ok {
		t.Errorf("after RESET the integrated loudness reads %q", ledText(r, "lufs-i-led"))
	}
}

// Wow and flutter is read off the panel's turn, and written when it is done.
func TestWowAndFlutterIsReadInTheBackground(t *testing.T) {
	r, c := idleRack(t, "lorenz")
	set(t, r, "gen-x-wave", "0", "gen-x-lvl", "80", "mix-rl-g1", "1", "wf-win", "2")
	looks(t, r, c, 2.6)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if v, ok := meterSays(r, "wf-carrier-led"); ok && v > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the carrier never read: %q", ledText(r, "wf-carrier-led"))
		}
		looks(t, r, c, 0.6)
		time.Sleep(20 * time.Millisecond)
	}
}
