package attractor

// How fast the model turns, for the page and a rack without it alike. The
// page turns it by a rate knob's value over twenty each frame at sixty frames
// a second (render.go, frameScale), and Auto-rot adds its own share to the
// Y rate whatever the knobs say.

// SpinPerUnit is a rate knob's unit as a turn, in radians a second.
const SpinPerUnit = 60.0 / 20

// AutoRotY is what Auto-rot adds to the Y rate, in the knob's units.
const AutoRotY = 0.1
