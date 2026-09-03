package attractor

// catmullRom evaluates the Catmull-Rom spline through p1..p2 at t∈[0,1).
func catmullRom(p0, p1, p2, p3, t float32) float32 {
	return 0.5 * ((2 * p1) +
		(-p0+p2)*t +
		(2*p0-5*p1+4*p2-p3)*t*t +
		(-p0+3*p1-3*p2+p3)*t*t*t)
}
