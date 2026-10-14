package scopegames

import (
	"math"
	"testing"
)

func half() float64 { return 0.5 }

// A floor hit blips at a pitch set by how hard the ball came down, and a
// warmup run blips nothing and counts no kicks.
func TestTheBallBlipsAtItsImpact(t *testing.T) {
	b := NewBall(half)
	var hz []float64
	b.Beep = func(f float64, _ int) { hz = append(hz, f) }
	b.Y, b.VY = BallFloor+0.001, -2
	b.Step(BallFrameDT)
	if len(hz) != 1 || math.Abs(hz[0]-(180+90*(2+12*BallFrameDT))) > 1e-9 {
		t.Fatalf("a hit at 2.19 blipped %v", hz)
	}
	b.Warm, hz = true, nil
	b.Y, b.VY = BallFloor+0.001, -0.1 // dead: the machine re-kicks it
	b.Step(BallFrameDT)
	if len(hz) != 0 || b.Kicks != 0 {
		t.Errorf("a warmup blipped %v and counted %d kicks", hz, b.Kicks)
	}
}

// A ball past a paddle scores for the other side and is served back; ten
// resets the match.
func TestPongScoresAndResets(t *testing.T) {
	p := NewPong(half)
	p.Serve, p.BX, p.VX, p.PadR = 0, PongW+0.24, 1, -0.9
	p.HumanR = PongHuman // a hand holding the paddle away
	p.Step(1)
	if p.ScoreL != 1 || p.BX != 0 || p.VX >= 0 {
		t.Fatalf("score %d, ball at %.2f moving %.2f; want 1, served left", p.ScoreL, p.BX, p.VX)
	}
	p.ScoreL, p.Serve, p.BX, p.VX = 9, 0, PongW+0.24, 1
	p.Step(1)
	if p.ScoreL != 0 || p.ScoreR != 0 {
		t.Errorf("a tenth point left the score %d:%d", p.ScoreL, p.ScoreR)
	}
}
