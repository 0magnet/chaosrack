package subcmds

import "math"

// --tumble: successive quarter turns on alternating screen axes — right, then
// down, then right — so a loop sees a figure from several sides rather than
// swinging about one axis. Each turn eases in and out (it settles at every
// quarter), and a --walk with as many steps as turns changes the colors
// while it is settled.
//
// SIX turns make a loop. A quarter turn right then a quarter turn down
// compose to a third of a turn about a diagonal, so two pairs end 240° round
// and the loop would jump back; three pairs end where they began.

// mat3 is a rotation, rows applied to a column vector.
type mat3 [3][3]float64

func (a mat3) mul(b mat3) mat3 {
	var c mat3
	for i := range 3 {
		for j := range 3 {
			for k := range 3 {
				c[i][j] += a[i][k] * b[k][j]
			}
		}
	}
	return c
}

func rotX(t float64) mat3 {
	s, c := math.Sin(t), math.Cos(t)
	return mat3{{1, 0, 0}, {0, c, -s}, {0, s, c}}
}

func rotY(t float64) mat3 {
	s, c := math.Sin(t), math.Cos(t)
	return mat3{{c, 0, s}, {0, 1, 0}, {-s, 0, c}}
}

func rotZ(t float64) mat3 {
	s, c := math.Sin(t), math.Cos(t)
	return mat3{{c, -s, 0}, {s, c, 0}, {0, 0, 1}}
}

// poseMatrix is the rasterview pose of angles a: Rx·Ry·Rz (project.go).
func poseMatrix(a [3]float64) mat3 { return rotX(a[0]).mul(rotY(a[1])).mul(rotZ(a[2])) }

// poseAngles is poseMatrix's inverse: the x, y, z angles of rotation m.
func poseAngles(m mat3) [3]float64 {
	sy := math.Max(-1, math.Min(1, m[0][2]))
	ay := math.Asin(sy)
	if math.Abs(math.Cos(ay)) < 1e-9 {
		// Gimbal lock: the z turn folds into x.
		return [3]float64{math.Atan2(m[2][1], m[1][1]), ay, 0}
	}
	return [3]float64{math.Atan2(-m[1][2], m[2][2]), ay, math.Atan2(-m[0][1], m[0][0])}
}

// tumbleViews are n frames of turns quarter turns from the pose base. A turn
// right is about the screen's vertical (y), one down about its horizontal
// (x), both applied after the pose, so they are the viewer's directions
// whatever the figure's.
func tumbleViews(base [3]float64, turns, n int) [][3]float64 {
	out := make([][3]float64, n)
	m0 := poseMatrix(base)
	for i := range out {
		f := float64(i) / float64(n) * float64(turns)
		seg := int(f)
		u := f - float64(seg)
		u = u * u * u * (u*(u*6-15) + 10) // smootherstep: still at each end
		m := m0
		for k := range seg {
			m = quarter(k, math.Pi/2).mul(m)
		}
		m = quarter(seg, u*math.Pi/2).mul(m)
		out[i] = poseAngles(m)
	}
	return out
}

// quarter is turn k of a tumble, t of the way: right on even turns, down on
// odd ones.
func quarter(k int, t float64) mat3 {
	if k%2 == 0 {
		return rotY(t)
	}
	return rotX(t)
}
