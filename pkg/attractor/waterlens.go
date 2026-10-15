package attractor

// The water lens's settings, shared by the page (water_js.go, which says
// what the lens is) and a native rack. The page shows its knobs in a Water
// module while BEHIND is on water; a native rack runs it at these.

// WaterSettings are the lens's knobs: how hard it bends (texels at unit
// slope), wave speed, per-mille kept a step, per-cent viscous smoothing,
// per-cent of a wave the walls return, audio drive, speakers in the tank,
// and raindrops a second.
type WaterSettings struct {
	Amount, Speed, Damp, Spread, Edge, Drive, Srcs, Rain float32
}

// WaterDefaults are the lens's knobs where they start.
var WaterDefaults = WaterSettings{Amount: 25, Speed: 0.5, Damp: 985, Spread: 12, Edge: 100, Drive: 0, Srcs: 1, Rain: 0}

// WaterGrid is the tank's size, in cells, each way: not the picture's — the
// wavelengths that matter are many cells wide at this, and a step costs
// nothing beside a frame.
const WaterGrid = 192

// WaterKnob is one of the lens's knobs: its control, label, default and
// travel.
type WaterKnob struct {
	ID, Label           string
	Def, Min, Max, Step float32
}

// WaterKnobs are the Water module's knobs, in its order: what the page
// builds them from, and what a native rack offers.
var WaterKnobs = []WaterKnob{
	{"water-amount", "bend", WaterDefaults.Amount, 0, 200, 1},
	{"water-speed", "speed", WaterDefaults.Speed, 0.05, 0.7, 0.01},
	{"water-damp", "damp", WaterDefaults.Damp, 900, 1000, 1},
	{"water-spread", "visc", WaterDefaults.Spread, 0, 60, 1},
	{"water-edge", "walls", WaterDefaults.Edge, 0, 100, 1},
	{"water-drive", "drive", WaterDefaults.Drive, 0, 100, 1},
	{"water-srcs", "spkrs", WaterDefaults.Srcs, 1, 4, 1},
	{"water-rain", "rain", WaterDefaults.Rain, 0, 40, 1},
}

// WaterFrom is the lens's settings from its knobs' values, knob reading
// each by its control.
func WaterFrom(knob func(id string) float32) WaterSettings {
	return WaterSettings{
		Amount: knob("water-amount"), Speed: knob("water-speed"), Damp: knob("water-damp"),
		Spread: knob("water-spread"), Edge: knob("water-edge"), Drive: knob("water-drive"),
		Srcs: knob("water-srcs"), Rain: knob("water-rain"),
	}
}
