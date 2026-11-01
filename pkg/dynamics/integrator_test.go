package dynamics

import "testing"

func TestIntegratorDefaultsToRK4(t *testing.T) {
	if CurrentIntegrator() != RK4Integrator || UsingEuler() || IntegratorKnob != 0 {
		t.Fatalf("integrator in force is %v, want rk4", CurrentIntegrator())
	}
}

func TestParseIntegrator(t *testing.T) {
	for _, c := range []struct {
		in   string
		want Integrator
	}{{"rk4", RK4Integrator}, {"RK4", RK4Integrator}, {"", RK4Integrator}, {"euler", EulerIntegrator}, {" Euler ", EulerIntegrator}} {
		in, want := c.in, c.want
		got, err := ParseIntegrator(in)
		if err != nil || got != want {
			t.Errorf("ParseIntegrator(%q) = %v, %v; want %v", in, got, err, want)
		}
		if err == nil && got.String() != map[Integrator]string{RK4Integrator: "rk4", EulerIntegrator: "euler"}[want] {
			t.Errorf("%v prints as %q", want, got.String())
		}
	}
	if _, err := ParseIntegrator("verlet"); err == nil {
		t.Error("ParseIntegrator accepted verlet")
	}
}

// withIntegrator runs f with i in force and puts the default back.
func withIntegrator(t *testing.T, i Integrator) {
	t.Helper()
	old := CurrentIntegrator()
	SetIntegrator(i)
	t.Cleanup(func() { SetIntegrator(old) })
}

// Trajectory, and every consumer of FlowFor4, step by the integrator in
// force: the classic Lorenz is RK4 by default and the page's own float32
// Euler when that is chosen, and the two part within a few hundred steps.
func TestTrajectoryFollowsTheIntegrator(t *testing.T) {
	o := TrajectoryOptions{Transient: 0.001, Duration: 5, MaxPoints: 1 << 20}
	rk := Trajectory("lorenz", o)

	dt, f, _ := FlowFor("lorenz")
	ic := InitCondFor("lorenz")
	x, y, z := float64(ic[0]), float64(ic[1]), float64(ic[2])
	for t := 0.0; t < o.Transient; t += dt {
		x, y, z = RK4(f, dt, x, y, z)
	}
	x, y, z = RK4(f, dt, x, y, z)
	if p := rk[0]; p != [3]float64{x, y, z} {
		t.Errorf("default trajectory starts at %v, RK4 by hand at %v", p, [3]float64{x, y, z})
	}
	if s, _ := FlowFor4("lorenz"); s.Euler {
		t.Error("FlowFor4 says Euler under rk4")
	}

	withIntegrator(t, EulerIntegrator)
	eu := Trajectory("lorenz", o)
	if s, _ := FlowFor4("lorenz"); !s.Euler {
		t.Error("FlowFor4 says RK4 under euler")
	}
	if s, _ := FlowFor4("hyperrossler"); !s.Euler {
		t.Error("a 4-D system's Euler mark does not follow the integrator")
	}
	if len(eu) != len(rk) || eu[len(eu)-1] == rk[len(rk)-1] {
		t.Errorf("euler and rk4 trajectories end together (%d and %d points)", len(eu), len(rk))
	}
}
