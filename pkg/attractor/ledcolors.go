package attractor

// LEDColor is one of the LED readouts' colors, as CSS hex: the lit segments,
// their glow, the display's ground and its border.
type LEDColor struct{ Name, Col, Glow, BG, BD string }

// LEDColors are the Style module's LED colors, in its knob's order: the one
// table the page's readouts and a native rack's are colored from. The first
// is the default.
var LEDColors = []LEDColor{
	{"red", "#ff3b30", "#ff2a20", "#170000", "#3a0000"},
	{"amber", "#ffb000", "#d08000", "#171000", "#3a2a00"},
	{"green", "#35e06a", "#1c9a44", "#001709", "#0a3a1a"},
	{"blue", "#3bb0ff", "#1c7ad0", "#001017", "#0a2a3a"},
	{"cyan", "#2ce0e0", "#159a9a", "#001515", "#0a3838"}, // the old audio-mod readout hue
	{"violet", "#b06cff", "#8040d0", "#12001f", "#2a0a3a"},
}
