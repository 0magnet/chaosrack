package rackhost

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/0magnet/chaosrack/pkg/scopegames"
)

// clock is a games clock the test moves by hand.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

// gameRack is a rack on model, untouched, its games on a hand-moved clock.
func gameRack(t *testing.T, model string) (*Rack, *clock, *fakeSpeaker) {
	t.Helper()
	r := newRack(t)
	c := &clock{t: time.Unix(1000, 0)}
	r.sound.games.now = c.now
	sp := &fakeSpeaker{r: r}
	r.SetSpeaker(sp)
	r.mu.Lock()
	err := r.set(r.selector, model)
	r.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	look(t, r)
	return r, c, sp
}

// play runs the games for secs, a look every 50 ms, filling the speakers as
// they go, and returns the left speaker.
func play(t *testing.T, r *Rack, c *clock, secs float64) []float32 {
	t.Helper()
	var out []float32
	const look50 = 50 * time.Millisecond
	for range int(secs / look50.Seconds()) {
		c.t = c.t.Add(look50)
		look(t, r)
		out = append(out, fill(r, look50.Seconds())...)
	}
	return out
}

// ledText is what the game display marked "led id" says.
func ledText(r *Rack, id string) string {
	for _, it := range r.pic.Items {
		if it != nil && it.Mark == "led "+id {
			return it.Text
		}
	}
	return "(none)"
}

// The ball plays on the clock: sixty frames a second, the trail as long as
// the Trail knob, and a re-kick counted on its display.
func TestTheBallPlaysOnTheClock(t *testing.T) {
	r, c, _ := gameRack(t, "bounceball")
	g := &r.sound.games
	if g.key != "bounceball" || len(g.ring) != int(r.number("trail-slider")) {
		t.Fatalf("playing %q with a trail of %d, want bounceball and %v", g.key, len(g.ring), r.number("trail-slider"))
	}
	play(t, r, c, 1)
	// Sixty frames, across at the drift: 0.7 × 0.016 a frame, whichever way
	// the walls send it.
	dx, ring := 0.0, g.ring[len(g.ring)-61:]
	for i := 1; i < len(ring); i++ {
		dx += math.Abs(ring[i][0] - ring[i-1][0])
	}
	if math.Abs(dx-60*0.7*scopegames.BallFrameDT) > 0.05 {
		t.Errorf("the ball went %.3f across in a second, want %.3f", dx, 60*0.7*scopegames.BallFrameDT)
	}
	g.ball.Kicks = 3
	play(t, r, c, 0.1)
	if got := ledText(r, "bounce-kicks"); got != "3" {
		t.Errorf("the kicks display says %q, want 3", got)
	}
}

// Untouched, the ball is silent, as the page's is until a gesture lets its
// audio start; a hand anywhere on the rack lets its blips through, on FX.
func TestTheBallBlipsOnceTheRackIsTouched(t *testing.T) {
	r, c, sp := gameRack(t, "bounceball")
	if s := play(t, r, c, 2); peakOf(s) > 0 || sp.playing {
		t.Fatalf("an untouched ball sounded (peak %.3f, speaker %v)", peakOf(s), sp.playing)
	}
	set(t, r, "bounce-grav", "12") // a hand on the rack, changing nothing
	s := play(t, r, c, 3)
	if !sp.playing {
		t.Fatal("the speaker did not start once the rack was touched")
	}
	// A blip starts at its level, 0.1, and falls; a floor hit and the re-kick
	// it sets off blip together, as the page's do.
	if p := peakOf(s); p < 0.05 || p > 0.2 {
		t.Errorf("the ball's blips peak at %.3f, want 0.1, or two of them at most", p)
	}
}

// Pong is silent until a hand takes a paddle, as the page's waits for a key;
// then the game blips, and the pot follows its paddle when the machine
// plays it again.
func TestPongWaitsForAPaddle(t *testing.T) {
	r, c, _ := gameRack(t, "pong")
	set(t, r, "pong-speed", "3")
	if s := play(t, r, c, 3); peakOf(s) > 0 {
		t.Fatalf("pong sounded before a paddle was played, peak %.3f", peakOf(s))
	}
	set(t, r, "pong-pad-l", "0.5")
	g := &r.sound.games
	if g.pong.PadL != 0.5 || g.pong.HumanL != scopegames.PongHuman {
		t.Fatalf("the pot left the paddle at %.2f, human for %d frames", g.pong.PadL, g.pong.HumanL)
	}
	s := play(t, r, c, 4)
	if p := peakOf(s); p < 0.05 {
		t.Errorf("a played pong peaks at %.3f; its blips start at 0.08", p)
	}
	want := strconv.FormatFloat(g.pong.PadR/scopegames.PongH, 'f', 2, 64)
	if got := r.value("pong-pad-r"); got != want {
		t.Errorf("the right pot reads %s, its paddle %s", got, want)
	}
}

// A point scored is on the score display.
func TestPongKeepsScore(t *testing.T) {
	r, c, _ := gameRack(t, "pong")
	r.sound.games.pong.ScoreL, r.sound.games.pong.ScoreR = 4, 2
	play(t, r, c, 0.1)
	if l, rt := ledText(r, "pong-score-l"), ledText(r, "pong-score-r"); l != "4" || rt != "2" {
		t.Errorf("the score reads %s:%s, want 4:2", l, rt)
	}
}

// The head's first switch drops the ball and restarts Pong, and springs back.
func TestTheHeadSwitchDropsAndRestarts(t *testing.T) {
	r, c, _ := gameRack(t, "bounceball")
	play(t, r, c, 1)
	set(t, r, "model-sw-0", "1")
	if b := r.sound.games.ball; b.X != scopegames.BallStartX || b.VY != 0 {
		t.Errorf("Drop left the ball at %.2f moving %.2f up", b.X, b.VY)
	}
	if r.value("model-sw-0") != "0" {
		t.Error("the switch stayed thrown")
	}
	r, c, _ = gameRack(t, "pong")
	r.sound.games.pong.ScoreL = 5
	play(t, r, c, 0.1)
	set(t, r, "model-sw-0", "1")
	if r.sound.games.pong.ScoreL != 0 {
		t.Error("Restart kept the score")
	}
}

// Both games draw in the scene, and a game's readouts are its own: the ball's
// kicks are not on Pong's panel.
func TestTheGamesDrawAndShowTheirOwnReadouts(t *testing.T) {
	for _, model := range []string{"bounceball", "pong"} {
		r, c, _ := gameRack(t, model)
		play(t, r, c, 0.5)
		img, err := r.Scene(160, 100, 1)
		if err != nil {
			t.Fatalf("%s: %v", model, err)
		}
		lit := 0
		for i := 0; i+2 < len(img.Px); i += 3 {
			if int(img.Px[i])+int(img.Px[i+1])+int(img.Px[i+2]) > 120 {
				lit++
			}
		}
		if lit < 50 {
			t.Errorf("%s draws %d lit pixels", model, lit)
		}
	}
	r, _, _ := gameRack(t, "pong")
	if ledText(r, "bounce-kicks") != "(none)" || ledText(r, "pong-score-l") == "(none)" {
		t.Error("Pong's panel shows the ball's kicks, or not its own score")
	}
}

func peakOf(s []float32) float64 {
	p := 0.0
	for _, v := range s {
		p = math.Max(p, math.Abs(float64(v)))
	}
	return p
}

// W held raises Pong's left paddle, and the side is a hand's; another model
// takes no keys.
func TestPongsPaddlesTakeKeys(t *testing.T) {
	r, c := idleRack(t, "pong")
	looks(t, r, c, 0.1)
	r.sound.mu.Lock()
	from := r.sound.games.pong.PadL
	r.sound.mu.Unlock()
	if !r.GameKey("w", true) {
		t.Fatal("Pong did not take W")
	}
	looks(t, r, c, 0.3)
	r.GameKey("w", false)
	r.sound.mu.Lock()
	to, human, armed := r.sound.games.pong.PadL, r.sound.games.pong.HumanL, r.sound.games.pongArmed
	r.sound.mu.Unlock()
	if to <= from || human == 0 || !armed {
		t.Errorf("W held moved the left paddle from %.3f to %.3f, human %d, armed %v", from, to, human, armed)
	}
	set(t, r, r.selector, "globe")
	if r.GameKey("up", true) {
		t.Error("a globe took ↑")
	}
}
