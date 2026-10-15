package attractor

// Phosphor is a CRT phosphor: its fresh trace's color, and how much of each
// channel it keeps from one frame to the next (higher, longer persistence;
// unequal, a trail that changes color as it fades, as P7's blue flash dies
// under its green afterglow).
type Phosphor struct {
	Name       string
	TR, TG, TB float64
	KR, KG, KB float64
}

// Phosphors are the Style module's phosphors, in its knob's order: the one
// table the page's CRT look and a native rack's are drawn from. The first is
// none: no CRT look.
var Phosphors = []Phosphor{
	{"— none —", 0, 0, 0, 1, 1, 1},
	{"P31 green", 0.35, 1.00, 0.45, 0.62, 0.62, 0.62},
	{"P1 green", 0.45, 1.00, 0.28, 0.80, 0.80, 0.80},
	{"P2 yel-green", 0.75, 1.00, 0.25, 0.90, 0.90, 0.90},
	{"P3 amber", 1.00, 0.75, 0.15, 0.85, 0.85, 0.85},
	{"P4 white", 0.95, 0.97, 1.00, 0.58, 0.58, 0.58},
	{"P11 blue", 0.30, 0.45, 1.00, 0.60, 0.60, 0.60},
	{"P7 blue→green", 0.45, 0.80, 1.00, 0.58, 0.95, 0.55},
	{"P39 green", 0.55, 1.00, 0.40, 0.94, 0.94, 0.94},
	{"P33 amber", 1.00, 0.50, 0.10, 0.985, 0.985, 0.985},
}
