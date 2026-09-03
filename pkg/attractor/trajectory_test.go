package attractor

import "testing"

// Every registered flow must trace a bounded, non-degenerate figure — the
// export turns these into meshes, and a mode that returns three points or a
// straight line would silently ship as an empty model.
func TestEveryFlowTraces(t *testing.T) {
	keys := FlowKeys()
	if len(keys) == 0 {
		t.Fatal("no flows registered")
	}
	for _, k := range keys {
		path := Trajectory(k, DefaultTrajectory())
		if len(path) < 1000 {
			t.Errorf("%s: %d points, want a full trace", k, len(path))
			continue
		}
		var min, max [3]float64
		min, max = path[0], path[0]
		for _, p := range path {
			for i := 0; i < 3; i++ {
				if p[i] < min[i] {
					min[i] = p[i]
				}
				if p[i] > max[i] {
					max[i] = p[i]
				}
			}
		}
		for i := 0; i < 3; i++ {
			if max[i]-min[i] <= 0 {
				t.Errorf("%s: axis %d has no extent — the figure is flat or a point", k, i)
			}
		}
	}
}

func TestTrajectoryOfANonFlowIsNil(t *testing.T) {
	if Trajectory("globe", DefaultTrajectory()) != nil {
		t.Error("geometry modes have no vector field and should trace nothing")
	}
	if HasFlow("torus") {
		t.Error("torus is geometry, not a flow")
	}
}

// MaxPoints has to be a maximum. Flooring the stride returned up to twice it.
func TestTrajectoryRespectsMaxPoints(t *testing.T) {
	for _, k := range FlowKeys() {
		for _, max := range []int{100, 999, 4000} {
			o := DefaultTrajectory()
			o.MaxPoints = max
			if got := len(Trajectory(k, o)); got > max {
				t.Errorf("%s: %d points for a cap of %d", k, got, max)
			}
		}
	}
}
