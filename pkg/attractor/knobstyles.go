package attractor

// KnobStyle is how a knob looks in one of the Style module's KNOB settings,
// as a rack without the page draws it: the page paints a knob's body with a
// gradient (panel.css, "Knob appearance styles"), and a flat picture takes
// its middle stop; the border and the pointer are the page's own.
type KnobStyle struct {
	Body, Border, Pointer int // 0xRRGGBB
}

// KnobStyles are the KNOB settings, by the selector's value.
var KnobStyles = map[string]KnobStyle{
	"std":    {0x3a3a3a, 0x111111, 0xf0f0f0},
	"flat":   {0x1d1d1d, 0x4c4c4c, 0x7fe0a0},
	"vint":   {0x7d6c4d, 0x251c10, 0x332614},
	"chrome": {0x9fa3a8, 0x3a3a3a, 0x1a1a1a},
	"gold":   {0xcaa23c, 0x5a4410, 0x3a2c08},
	"carbon": {0x232323, 0x0a0a0a, 0xe0e0e0},
}
