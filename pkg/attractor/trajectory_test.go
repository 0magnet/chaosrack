package attractor

import (
	"testing"

	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// Every registered flow must trace a bounded, non-degenerate figure — the
// export turns these into meshes, and a mode that returns three points or a
// straight line would silently ship as an empty model.
func TestEveryFlowTraces(t *testing.T) {
	keys := dynamics.Keys()
	if len(keys) == 0 {
		t.Fatal("no flows registered")
	}
	for _, k := range keys {
		path := dynamics.Trajectory(k, dynamics.DefaultTrajectory())
		if len(path) < 1000 {
			t.Errorf("%s: %d points, want a full trace", k, len(path))
			continue
		}
		var lo, hi [3]float64
		lo, hi = path[0], path[0]
		for _, p := range path {
			for i := range 3 {
				if p[i] < lo[i] {
					lo[i] = p[i]
				}
				if p[i] > hi[i] {
					hi[i] = p[i]
				}
			}
		}
		for i := range 3 {
			if hi[i]-lo[i] <= 0 {
				t.Errorf("%s: axis %d has no extent — the figure is flat or a point", k, i)
			}
		}
	}
}

func TestTrajectoryOfANonFlowIsNil(t *testing.T) {
	if dynamics.Trajectory("globe", dynamics.DefaultTrajectory()) != nil {
		t.Error("geometry modes have no vector field and should trace nothing")
	}
	if dynamics.HasFlow("torus") {
		t.Error("torus is geometry, not a flow")
	}
}

// MaxPoints has to be a maximum. Flooring the stride returned up to twice it.
func TestTrajectoryRespectsMaxPoints(t *testing.T) {
	for _, k := range dynamics.Keys() {
		for _, max := range []int{100, 999, 4000} {
			o := dynamics.DefaultTrajectory()
			o.MaxPoints = max
			if got := len(dynamics.Trajectory(k, o)); got > max {
				t.Errorf("%s: %d points for a cap of %d", k, got, max)
			}
		}
	}
}
