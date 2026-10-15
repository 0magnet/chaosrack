package attractor

// Jam, the attract mode (Motion > Jam): the app performing itself, shared by
// the page (jam_js.go, which plays a hop through its controls) and a native
// rack. Every 12–20 seconds it hops to a random flow, sometimes turns
// Persist on, and always sets a fresh gentle spin.

// JamInterval is how long until the next hop, in milliseconds.
func JamInterval(rand func() float64) float64 { return 12000 + rand()*8000 }

// JamHop is a hop from model current: the flow to play (the attractors, the
// Sprott catalog and hyperchaos; not Custom, whose panel is an editor, not a
// show), whether Persist's paint look goes on (about a third of the time),
// and the spin rates — always some about Y, sometimes a touch about X — at
// the rate knobs' tenths. ok is false with nowhere to hop to.
func JamHop(rand func() float64, current string) (next string, persist bool, y, x float64, ok bool) {
	var pool []string
	for _, k := range ModeKeys(ClassFlow3D, ClassFlow4D) {
		if k != "custom" && k != current {
			pool = append(pool, k)
		}
	}
	if len(pool) == 0 {
		return "", false, 0, 0, false
	}
	next = pool[int(rand()*float64(len(pool)))%len(pool)]
	persist = rand() < 0.35
	tenths := func(v float64) float64 { return float64(int(v*10)) / 10 }
	y = 0.1 + rand()*0.2
	if rand() < 0.5 {
		y = -y
	}
	if rand() < 0.4 {
		x = (rand() - 0.5) * 0.2
	}
	return next, persist, tenths(y), tenths(x), true
}
