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
