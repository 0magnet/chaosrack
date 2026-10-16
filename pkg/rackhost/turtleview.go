package rackhost

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/attractor"
)

// The Turtle's VIEW, natively: the scene turned so the walk's axis points
// where the view says — end-on, from the back, across or up — as the page
// turns its model (turtleview_js.go). Set once when the view or the figure
// changes, so a hand can still turn it after; auto-rotate is put away while
// a view is held and given back at free.

// turtleAim is what the view was last set for.
type turtleAim struct {
	idx     int
	label   string
	set     bool
	saved   bool
	spinWas string
}

// aimTurtle turns the scene for the Turtle's VIEW, if it asks. Called with
// r.mu held, after the walk has stepped.
func (r *Rack) aimTurtle() {
	a := &r.turtleAim
	idx := int(r.number("turtle-view") + 0.5)
	to, aim := attractor.TurtleViewTarget(idx)
	if !aim {
		r.releaseTurtleAim()
		return
	}
	sm := &r.analyzers.scope
	axis, ok := sm.turtle.Axis()
	if !ok {
		return // a figure with no single direction: nothing to aim down
	}
	label := sm.turtle.Label()
	if a.set && a.idx == idx && a.label == label {
		return
	}
	if !a.saved {
		a.spinWas, a.saved = r.value("auto-rotate"), true
	}
	_ = r.set("auto-rotate", "0") //nolint:errcheck // a switch's own value
	angles := aimAngles(axis, [3]float64{float64(to[0]), float64(to[1]), float64(to[2])})
	r.poseTo = &angles
	a.idx, a.label, a.set = idx, label, true
}

// releaseTurtleAim gives auto-rotate back when the Turtle is left with a view
// held, as free does: the view put it away for the Turtle's sake, and no
// other model asked for that. Called with r.mu held.
func (r *Rack) releaseTurtleAim() {
	a := &r.turtleAim
	if a.saved {
		_ = r.set("auto-rotate", a.spinWas) //nolint:errcheck // the value it had
		a.saved = false
	}
	a.set = false
}

// aimAngles is the scene's pose — the angles rasterview turns a model by,
// Rx·Ry·Rz — that turns direction from onto direction to by the least
// rotation.
func aimAngles(from, to [3]float64) [3]float64 {
	f, t := unit(from), unit(to)
	v := cross(f, t)
	c := f[0]*t[0] + f[1]*t[1] + f[2]*t[2]
	var m [3][3]float64
	if c < -1+1e-9 {
		// Opposite: half a turn about any axis square to both.
		p := cross(f, [3]float64{1, 0, 0})
		if p == ([3]float64{}) || math.Abs(f[0]) > 0.9 {
			p = cross(f, [3]float64{0, 1, 0})
		}
		p = unit(p)
		for i := range 3 {
			for j := range 3 {
				m[i][j] = 2 * p[i] * p[j]
				if i == j {
					m[i][j]--
				}
			}
		}
	} else {
		// Rodrigues: I + [v]x + [v]x² / (1 + c).
		k := [3][3]float64{{0, -v[2], v[1]}, {v[2], 0, -v[0]}, {-v[1], v[0], 0}}
		for i := range 3 {
			for j := range 3 {
				kk := 0.0
				for n := range 3 {
					kk += k[i][n] * k[n][j]
				}
				m[i][j] = k[i][j] + kk/(1+c)
				if i == j {
					m[i][j]++
				}
			}
		}
	}
	// Rx(a)·Ry(b)·Rz(c) has sin b at (0,2), -sin a cos b at (1,2), cos a cos b
	// at (2,2), -cos b sin c at (0,1) and cos b cos c at (0,0).
	b := math.Asin(max(-1, min(1, m[0][2])))
	if math.Cos(b) < 1e-9 {
		return [3]float64{math.Atan2(m[2][1], m[1][1]), b, 0}
	}
	return [3]float64{math.Atan2(-m[1][2], m[2][2]), b, math.Atan2(-m[0][1], m[0][0])}
}

func cross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func unit(v [3]float64) [3]float64 {
	n := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
	if n == 0 {
		return v
	}
	return [3]float64{v[0] / n, v[1] / n, v[2] / n}
}
