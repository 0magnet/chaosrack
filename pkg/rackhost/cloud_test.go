package rackhost

import (
	"testing"

	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// At a render's twenty frames a second the swarm still spreads as the page's
// does at sixty: its steps follow the time a frame stands for. A parameter
// named dt was shadowed by the model's own once, and the ball stayed a dot.
func TestSwarmSpreadsAtTwentyFramesASecond(t *testing.T) {
	var c cloudState
	for range 160 { // eight seconds
		if _, ok := c.swarmFigure("lorenz", true, false, 2000, 0, 1.0/20); !ok {
			t.Fatal("lorenz has no swarm")
		}
	}
	if s := c.swarm.Spread(); s < 0.1 {
		t.Errorf("after eight seconds the swarm is still a clump, spread %.4f", s)
	}
	if _, ok := c.swarmFigure("clifford", true, false, 2000, 0, 1.0/20); ok {
		t.Error("a map has no swarm; it has the density view")
	}
}

// The density view on a map: lit cells, colored, framed to its box.
func TestDensityFigureFillsTheMapsBox(t *testing.T) {
	var c cloudState
	fig, fit, ok := c.densityFigure("clifford", true, false, 5000, rasterview.DefaultGradient())
	if !ok || len(fig.Points) < 1000 || len(fig.Colors) != len(fig.Points) || len(fit) != 4 {
		t.Fatalf("ok %v, %d points, %d colors, %d corners", ok, len(fig.Points), len(fig.Colors), len(fit))
	}
	for _, p := range fig.Points {
		if p[0] < fit[0][0] || p[0] > fit[3][0] || p[1] < fit[0][1] || p[1] > fit[3][1] {
			t.Fatalf("cell %v is outside the box %v..%v", p, fit[0], fit[3])
		}
	}
	if _, _, ok := c.densityFigure("lorenz", true, false, 5000, rasterview.DefaultGradient()); ok {
		t.Error("a flow has no density view")
	}
}
