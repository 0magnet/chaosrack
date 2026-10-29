package attractor

import "testing"

// At 60 Hz the clock ticks four times a second, on whichever frames cross the
// grid, and not on the frames between.
func TestThePanelClockTicksFourTimesASecond(t *testing.T) {
	var c panelClock
	ticks := 0
	for f := 1; f <= 600; f++ { // ten seconds at 60 Hz
		c.advance(float64(f) * 1000 / 60)
		if c.due {
			ticks++
		}
	}
	if ticks < 39 || ticks > 41 {
		t.Fatalf("%d ticks in ten seconds, want 40", ticks)
	}
}

// A frame slower than the period ticks every time, and the clock starts again
// from it rather than ticking twice to catch up.
func TestASlowRackTicksOnEveryFrame(t *testing.T) {
	var c panelClock
	for f := 1; f <= 5; f++ {
		c.advance(float64(f) * 1000)
		if !c.due {
			t.Fatalf("frame %d at 1 fps did not tick", f)
		}
	}
	c.advance(5100)
	if c.due {
		t.Fatal("ticked again 100 ms after a tick")
	}
}

// A timeline that starts again (a reload, a test setting the frame time back)
// ticks at once instead of waiting for the old one to come round.
func TestAClockThatGoesBackTicksAtOnce(t *testing.T) {
	var c panelClock
	c.advance(100000)
	c.advance(50)
	if !c.due {
		t.Fatal("the clock went back and did not tick")
	}
	c.advance(100)
	if c.due {
		t.Fatal("ticked twice in 50 ms after starting again")
	}
}

// What is registered runs on ticks only, in order.
func TestOnTickRunsOnTicksOnly(t *testing.T) {
	var c panelClock
	var got []int
	c.onTick(func() { got = append(got, 1) })
	c.onTick(func() { got = append(got, 2) })
	c.advance(1000) // tick
	c.advance(1016) // not
	c.advance(1250) // tick
	if len(got) != 4 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("ran %v, want [1 2 1 2]", got)
	}
}
